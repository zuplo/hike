package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/zuplo/hike/internal/config"
	"github.com/zuplo/hike/internal/names"
	"github.com/zuplo/hike/internal/project"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:    "mcp",
	Short:  "Start MCP server (stdio transport)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := server.NewMCPServer("hike", version,
			server.WithToolCapabilities(true),
		)

		s.AddTool(mcp.NewTool("create_project",
			mcp.WithDescription("Create a new project with git worktrees for all repos in a group"),
			mcp.WithString("group", mcp.Description("Group name (uses default group if omitted)")),
			mcp.WithString("name", mcp.Description("Project name suffix (random if omitted)")),
			mcp.WithString("color", mcp.Description("VS Code title bar color: red, orange, yellow, green, teal, blue, indigo, purple, pink, rose, sky, lime, cyan, slate")),
		), handleCreate)

		s.AddTool(mcp.NewTool("delete_project",
			mcp.WithDescription("Delete a project and remove its worktrees"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Full project name (e.g. platform-bold-cedar)")),
		), handleDelete)

		s.AddTool(mcp.NewTool("list_projects",
			mcp.WithDescription("List all projects"),
		), handleList)

		s.AddTool(mcp.NewTool("sync_repos",
			mcp.WithDescription("Clone missing repos and sync .main repos to latest origin/HEAD"),
			mcp.WithString("group", mcp.Description("Group name (syncs all groups if omitted)")),
		), handleSync)

		s.AddTool(mcp.NewTool("project_status",
			mcp.WithDescription("Show git status of all repos in a project"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Full project name")),
		), handleStatus)

		s.AddTool(mcp.NewTool("pull_project",
			mcp.WithDescription("Pull latest changes in all repos of a project"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Full project name")),
		), handlePull)

		s.AddTool(mcp.NewTool("push_project",
			mcp.WithDescription("Push all repos in a project"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Full project name")),
		), handlePush)

		return server.ServeStdio(s)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func mcpResolveGroup(params map[string]any) (string, error) {
	if g, ok := params["group"].(string); ok && g != "" {
		if cfg != nil {
			resolved, found := cfg.ResolveGroup(g)
			if !found {
				return "", fmt.Errorf("group %q not found", g)
			}
			return resolved, nil
		}
		return g, nil
	}
	if cfg != nil && cfg.DefaultGroup() != "" {
		return cfg.DefaultGroup(), nil
	}
	return "", fmt.Errorf("no group specified and no default group configured")
}

func handleCreate(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	group, err := mcpResolveGroup(request.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		name = names.Generate()
	}

	projectName := config.ProjectName(group, name)
	color, _ := request.GetArguments()["color"].(string)

	if err := project.Create(rootDir, cfg, projectName, group, color); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Project %q created in group %q.", projectName, group)), nil
}

func handleDelete(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	if err := project.Delete(rootDir, cfg, name); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Project %q deleted.", name)), nil
}

func handleList(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projects, err := project.List(rootDir)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if len(projects) == 0 {
		return mcp.NewToolResultText("No projects found."), nil
	}
	return mcp.NewToolResultText(strings.Join(projects, "\n")), nil
}

func handleSync(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("Use 'hike sync' from the command line for sync operations."), nil
}

func handleStatus(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	statuses, err := project.GetStatus(rootDir, cfg, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var lines []string
	for _, s := range statuses {
		dirty := ""
		if s.Dirty {
			dirty = " [dirty]"
		}
		lines = append(lines, fmt.Sprintf("%-20s branch: %s%s", s.Repo, s.Branch, dirty))
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handlePull(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	results, err := project.Pull(rootDir, cfg, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var lines []string
	for _, r := range results {
		if r.Err != nil {
			lines = append(lines, fmt.Sprintf("%s: %v", r.Repo, r.Err))
		} else {
			lines = append(lines, fmt.Sprintf("%s: %s", r.Repo, r.Output))
		}
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handlePush(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	results, err := project.Push(rootDir, cfg, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var lines []string
	for _, r := range results {
		if r.Err != nil {
			lines = append(lines, fmt.Sprintf("%s: %v", r.Repo, r.Err))
		} else {
			lines = append(lines, fmt.Sprintf("%s: %s", r.Repo, r.Output))
		}
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}
