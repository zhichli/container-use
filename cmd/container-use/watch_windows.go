//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dagger/container-use/repository"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch environment activity in real-time",
	Long: `Continuously display environment activity as agents work.
Shows new commits and environment changes updated every second.
Press Ctrl+C to stop watching.`,
	Example: `# Watch all environment activity
container-use watch

# Monitor agents while they work
container-use watch`,
	RunE: func(app *cobra.Command, _ []string) error {
		ctx := app.Context()

		// Ensure we're in a git repository
		if _, err := repository.Open(ctx, "."); err != nil {
			return err
		}

		return watchGitLogWindows(ctx)
	},
}

func watchGitLogWindows(ctx context.Context) error {
	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nWatch stopped.")
		cancel()
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Initial display
	if err := runGitLog(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Clear screen and show updated log
			if err := clearScreen(); err != nil {
				// If clear fails, just add a separator
				fmt.Println("\n" + strings.Repeat("=", 50))
			}
			if err := runGitLog(ctx); err != nil {
				return err
			}
		}
	}
}

func runGitLog(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "log", "--color=always", "--remotes=container-use", "--oneline", "--graph", "--decorate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func clearScreen() error {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func init() {
	rootCmd.AddCommand(watchCmd)
}
