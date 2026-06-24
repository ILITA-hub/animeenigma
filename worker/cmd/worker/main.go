package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ILITA-hub/animeenigma/worker/internal/agent"
)

func main() {
	cfg := agent.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	client := agent.NewClient(cfg)
	if err := client.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, "worker: fatal:", err)
		os.Exit(1)
	}
}
