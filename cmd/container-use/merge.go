package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <env>",
	Short: "Merges an environment into the current git branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(app *cobra.Command, args []string) error {
		env := args[0]
		// prevent accidental single quotes to mess up command
		env = strings.Trim(env, "'")
		cmd := exec.CommandContext(app.Context(), "bash", "-c", fmt.Sprintf("git stash --include-untracked && git merge -m 'Merge environment %s' -- %q && git stash pop", env, "container-use/"+env))
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		return cmd.Run()
	},
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}
