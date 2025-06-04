package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"dagger.io/dagger"
	"github.com/aluzzardi/container-use/mcpserver"
	"github.com/aluzzardi/container-use/rules"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var dag *dagger.Client

func dumpStacks() {
	buf := make([]byte, 1<<20) // 1MB buffer
	n := runtime.Stack(buf, true)
	io.MultiWriter(logWriter, os.Stderr).Write(buf[:n])
}

var (
	rootCmd = &cobra.Command{
		Use:   "container-use",
		Short: "Container Use",
		Long:  `MCP server to add container superpowers to your AI agent.`,
	}

	stdioCmd = &cobra.Command{
		Use:   "stdio",
		Short: "Start stdio server",
		Long:  `Start a server that communicates via standard input/output streams using JSON-RPC messages.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			slog.Info("connecting to dagger")

			var err error
			dag, err = dagger.Connect(context.Background(), dagger.WithLogOutput(logWriter))
			if err != nil {
				slog.Error("Error starting dagger", "error", err)
				os.Exit(1)
			}
			defer dag.Close()

			s := server.NewMCPServer(
				"Dagger",
				"1.0.0",
				server.WithInstructions(rules.AgentRules),
			)

			for _, t := range mcpserver.Tools {
				s.AddTool(t.Definition, t.Handler)
			}

			slog.Info("starting server")
			return server.ServeStdio(s)
		},
	}
)

func init() {
	rootCmd.AddCommand(stdioCmd)
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1)

	go func() {
		for sig := range sigs {
			if sig == syscall.SIGUSR1 {
				dumpStacks()
			}
		}
	}()

	if err := setupLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
