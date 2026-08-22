package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/monkescience/yeet/internal/build"
	"github.com/monkescience/yeet/internal/commands"
	"github.com/monkescience/yeet/internal/telemetry"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := telemetry.New(build.Version())

	return commands.NewRoot(manager).ExecuteContext(ctx) //nolint:wrapcheck // preserve user-facing error verbatim
}
