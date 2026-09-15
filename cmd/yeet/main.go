package main

import (
	"context"
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
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := telemetry.New(build.Version())

	return commands.Execute(ctx, manager) //nolint:wrapcheck // commands report and preserve the failure
}
