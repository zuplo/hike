package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:    "mcp",
	Short:  "Start MCP server (stdio transport)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := server.NewMCPServer("zproj", version,
			server.WithToolCapabilities(true),
		)

		s.AddTool(mcp.NewTool("create_project",
			mcp.WithDescription("Create a new project with git worktrees for all repos in a group"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
			mcp.WithString("group", mcp.Description("Group name (uses default group if omitted)")),
			mcp.WithString("color", mcp.Description("VS Code title bar color: red, orange, yellow, green, teal, blue, indigo, purple, pink, rose, sky, lime, cyan, slate")),
		), handleCreate)

		s.AddTool(mcp.NewTool("delete_project",
			mcp.WithDescription("Delete a project and remove its worktrees"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
			mcp.WithString("group", mcp.Description("Group name (uses default group if omitted)")),
		), handleDelete)

		s.AddTool(mcp.NewTool("list_projects",
			mcp.WithDescription("List all projects, optionally filtered by group"),
			mcp.WithString("group", mcp.Description("Group name (lists all groups if omitted)")),
		), handleList)

		s.AddTool(mcp.NewTool("sync_repos",
			mcp.WithDescription("Sync .main repos to latest origin/HEAD"),
			mcp.WithString("group", mcp.Description("Group name (syncs all groups if omitted)")),
		), handleSync)

		s.AddTool(mcp.NewTool("project_status",
			mcp.WithDescription("Show git status of all repos in a project"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
			mcp.WithString("group", mcp.Description("Group name (uses default group if omitted)")),
		), handleStatus)

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

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	group, err := mcpResolveGroup(request.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	color, _ := request.GetArguments()["color"].(string)

	if err := project.Create(rootDir, cfg, name, group, color); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Project %q created in group %q.", name, group)), nil
}

func handleDelete(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	group, err := mcpResolveGroup(request.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := project.Delete(rootDir, cfg, name, group); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Project %q deleted from group %q.", name, group)), nil
}

func handleList(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	groups := make(map[string]struct{})
	if g, ok := request.GetArguments()["group"].(string); ok && g != "" {
		resolved, found := cfg.ResolveGroup(g)
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("group %q not found", g)), nil
		}
		groups[resolved] = struct{}{}
	} else {
		for g := range cfg.Groups {
			groups[g] = struct{}{}
		}
	}

	var lines []string
	for g := range groups {
		projects, err := project.List(rootDir, g)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		for _, p := range projects {
			lines = append(lines, fmt.Sprintf("[%s] %s", g, p))
		}
	}

	if len(lines) == 0 {
		return mcp.NewToolResultText("No projects found."), nil
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleSync(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// This delegates to the same logic but we run it inline
	return mcp.NewToolResultText("Use 'zproj sync' from the command line for sync operations."), nil
}

func handleStatus(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := requireConfig(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	group, err := mcpResolveGroup(request.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	statuses, err := project.GetStatus(rootDir, cfg, name, group)
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
