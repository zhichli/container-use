package main

import (
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"dagger.io/dagger"
	"github.com/aluzzardi/container-use/environment"
	"github.com/aluzzardi/container-use/mcpserver"
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
		RunE: func(app *cobra.Command, _ []string) error {
			ctx := app.Context()

			ensureDagger()

			slog.Info("connecting to dagger")

			var err error
			dag, err = dagger.Connect(ctx, dagger.WithLogOutput(logWriter))
			if err != nil {
				slog.Error("Error starting dagger", "error", err)
				os.Exit(1)
			}
			defer dag.Close()

			environment.Initialize(dag)
			return mcpserver.RunStdioServer(ctx)
		},
	}
)

func init() {
	rootCmd.AddCommand(stdioCmd)
}

func ensureDagger() {
	if _, ok := os.LookupEnv("DAGGER_SESSION_TOKEN"); !ok {
		args := make([]string, len(os.Args)+2)
		var err error
		args[0], err = exec.LookPath("dagger")
		if err != nil {
			panic("TODO: auto download dagger")
		}
		args[1] = "run"
		copy(args[2:], os.Args)
		err = syscall.Exec(args[0], args, os.Environ())
		panic(fmt.Errorf("unexpected reexec failure %v: %w", args, err))
	}
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
