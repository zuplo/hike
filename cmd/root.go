package cmd

import (
	"fmt"
	"os"

	"github.com/zuplo/hike/internal/config"
	"github.com/zuplo/hike/internal/update"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	rootDir    string
	cfg        *config.Config
	cfgLoadErr error
	groupArg   string
	colorArg   string
	version    = "dev"
)

var nameArg string

var rootCmd = &cobra.Command{
	Use:   "hike [group] [name]",
	Short: "Git worktree project manager",
	Long:  "Manage multi-repo development workspaces using git worktrees.",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && groupArg == "" && nameArg == "" {
			return cmd.Help()
		}
		// Single positional arg without flags: only allow if it resolves to a
		// known group. Otherwise it's likely a typo or unknown subcommand.
		if len(args) == 1 && groupArg == "" && nameArg == "" {
			isGroup := false
			if cfg != nil {
				_, isGroup = cfg.ResolveGroup(args[0])
			}
			if !isGroup {
				return fmt.Errorf("unknown command %q. Run 'hike --help' for usage", args[0])
			}
		}
		group, name := resolveCreateArgs(args)
		return runCreateWithArgs(group, name)
	},
	SilenceUsage: true,
	Version:      version,
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if latest := update.CheckOutdated(version); latest != "" {
			fmt.Fprintf(os.Stderr, "\nA new version of hike is available: %s → %s\n", version, latest)
			fmt.Fprintf(os.Stderr, "Run 'hike update' to upgrade.\n")
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&groupArg, "group", "g", "", "group to operate on (default: the default group)")
	rootCmd.Flags().StringVarP(&nameArg, "name", "n", "", "project name")
	rootCmd.Flags().StringVarP(&colorArg, "color", "c", "", "title bar color (random if no color specified)")
	rootCmd.Flags().Lookup("color").NoOptDefVal = "random"
	createCmd.Flags().StringVarP(&colorArg, "color", "c", "", "title bar color (random if no color specified)")
	createCmd.Flags().Lookup("color").NoOptDefVal = "random"
}

func initConfig() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Allow commands that don't need config (like completion) to skip
	root, err := config.FindRoot(cwd)
	if err != nil {
		// Store empty root; commands that need config will check
		rootDir = ""
		return
	}
	rootDir = root

	cfgPath, _ := config.FindConfigFile(root)
	c, err := config.Load(cfgPath)
	if err != nil {
		// Don't fatal — commands that need config will check via requireConfig()
		cfgLoadErr = err
		return
	}
	cfg = c
}

func requireConfig() error {
	if cfgLoadErr != nil {
		return cfgLoadErr
	}
	if cfg == nil {
		return fmt.Errorf("no %s found. Run 'hike init' in a directory with a config file", config.ConfigFile)
	}
	return nil
}

// resolveCreateArgs figures out group and name from positional args + flags.
// Positional: `hike <group> <name>`, `hike <group>`, or `hike <name>`.
// The first arg is checked against known groups/aliases — if it matches, it's the group.
// Otherwise it's treated as the project name (using default group).
func resolveCreateArgs(args []string) (group, name string) {
	// Flags take priority
	group = groupArg
	name = nameArg

	switch len(args) {
	case 2:
		// hike <group> <name>
		if group == "" {
			group = args[0]
		}
		if name == "" {
			name = args[1]
		}
	case 1:
		// Is it a known group?
		if cfg != nil {
			if _, ok := cfg.ResolveGroup(args[0]); ok {
				if group == "" {
					group = args[0]
				}
			} else {
				// Not a group — treat as name
				if name == "" {
					name = args[0]
				}
			}
		} else if name == "" {
			name = args[0]
		}
	}

	return group, name
}

func resolveGroup() (string, error) {
	name := groupArg
	if name == "" {
		if cfg != nil && cfg.DefaultGroup() != "" {
			return cfg.DefaultGroup(), nil
		}
		return "", fmt.Errorf("no --group specified and no default group set in config\n\nSet a default group in %s:\n  groups:\n    mygroup:\n      default: true", config.ConfigFile)
	}
	if cfg != nil {
		resolved, ok := cfg.ResolveGroup(name)
		if !ok {
			return "", fmt.Errorf("group %q not found in config", name)
		}
		return resolved, nil
	}
	return name, nil
}
