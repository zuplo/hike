package cmd

import (
	"fmt"
	"os"

	"github.com/ntotten/zproj/internal/config"
	"github.com/ntotten/zproj/internal/update"
	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	rootDir  string
	cfg      *config.Config
	groupArg string
	colorArg string
	version  = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "zproj [project-name]",
	Short: "Git worktree project manager",
	Long:  "Manage multi-repo development workspaces using git worktrees.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		// Default action: create a project
		return runCreate(args[0])
	},
	SilenceUsage: true,
	Version:      version,
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if latest := update.CheckOutdated(version); latest != "" {
			fmt.Fprintf(os.Stderr, "\nA new version of zproj is available: %s → %s\n", version, latest)
			fmt.Fprintf(os.Stderr, "Run 'zproj update' to upgrade.\n")
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
	rootCmd.PersistentFlags().StringVar(&groupArg, "group", "default", "group to operate on")
	rootCmd.Flags().StringVarP(&colorArg, "color", "c", "", "title bar color for VS Code workspace (e.g. #1e90ff)")
	createCmd.Flags().StringVarP(&colorArg, "color", "c", "", "title bar color for VS Code workspace (e.g. #1e90ff)")
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

	c, err := config.Load(root + "/" + config.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	cfg = c
}

func requireConfig() error {
	if cfg == nil {
		return fmt.Errorf("no %s found. Run 'zproj init' in a directory with a config file", config.ConfigFile)
	}
	return nil
}

func resolveGroup() string {
	return groupArg
}
