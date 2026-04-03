package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ntotten/zproj/internal/update"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias [name]",
	Short: "Create a shell alias (symlink) for zproj",
	Long:  "Create a symlink so you can invoke zproj with a shorter name (e.g. 'z').",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) == 1 {
			name = args[0]
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Enter alias name (e.g. z): ")
			input, _ := reader.ReadString('\n')
			name = strings.TrimSpace(input)
		}

		if name == "" {
			return fmt.Errorf("alias name cannot be empty")
		}
		if name == "zproj" {
			return fmt.Errorf("alias is the same as the binary name")
		}

		binDir, err := update.BinDir()
		if err != nil {
			return err
		}
		target := filepath.Join(binDir, "zproj")

		if _, err := os.Stat(target); os.IsNotExist(err) {
			// Fall back to resolved executable path
			target, err = os.Executable()
			if err != nil {
				return fmt.Errorf("could not find zproj binary: %w", err)
			}
			target, _ = filepath.EvalSymlinks(target)
		}

		linkDir := "/usr/local/bin"
		linkPath := filepath.Join(linkDir, name)

		// Check if something already exists at that path
		if info, err := os.Lstat(linkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				existing, _ := os.Readlink(linkPath)
				if existing == target {
					fmt.Printf("Alias %q already exists and points to zproj.\n", name)
					return nil
				}
			}
			fmt.Printf("Warning: %s already exists.\n", linkPath)
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Overwrite? [y/N] ")
			answer, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(answer)) != "y" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		// Try without sudo first
		os.Remove(linkPath)
		if err := os.Symlink(target, linkPath); err != nil {
			// Need sudo
			fmt.Printf("Creating symlink %s -> %s (requires sudo)...\n", linkPath, target)
			rm := exec.Command("sudo", "ln", "-sf", target, linkPath)
			rm.Stdin = os.Stdin
			rm.Stdout = os.Stdout
			rm.Stderr = os.Stderr
			if err := rm.Run(); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		}

		fmt.Printf("Alias created: %s -> zproj\n", name)
		fmt.Printf("You can now use '%s' instead of 'zproj'.\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(aliasCmd)
}
