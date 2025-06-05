package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long:  `List environments filtering the git remotes`,
	RunE: func(app *cobra.Command, _ []string) error {
		cmd := exec.CommandContext(app.Context(), "bash", "-c", "git branch -r | grep 'container-use/.*/' | cut -d/ -f2-")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		return cmd.Run()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
