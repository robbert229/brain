package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/johnrowl/brain/internal/braind"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(braind.Run(ctx, os.Args[1:], version, os.Stdout, os.Stderr))
}
