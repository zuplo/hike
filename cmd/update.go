package cmd

import (
	"fmt"

	"github.com/ntotten/zproj/internal/update"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update zproj to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		latest, err := update.LatestVersion()
		if err != nil {
			return err
		}
		fmt.Printf("Current version: %s\n", version)
		fmt.Printf("Latest version:  %s\n", latest)

		if latest == version {
			fmt.Println("Already up to date.")
			return nil
		}

		fmt.Printf("Updating to %s...\n", latest)
		if err := update.SelfUpdate(version); err != nil {
			return err
		}
		fmt.Printf("Updated to %s.\n", latest)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
