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
	"github.com/mark3labs/mcp-go/server"
)

var dag *dagger.Client

//go:embed rules/agent.md
var mcpRules string

func dumpStacks() {
	buf := make([]byte, 1<<20) // 1MB buffer
	n := runtime.Stack(buf, true)
	io.MultiWriter(logWriter, os.Stderr).Write(buf[:n])
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
		server.WithInstructions(mcpRules),
	)

	for _, t := range tools {
		s.AddTool(t.Definition, t.Handler)
	}

	slog.Info("starting server")
	if err := server.ServeStdio(s); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}
}
