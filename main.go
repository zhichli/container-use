package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"dagger.io/dagger"
	"github.com/mark3labs/mcp-go/server"
)

var dag *dagger.Client

func main() {
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
