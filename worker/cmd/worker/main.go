package main

import (
	"context"
	"errors"
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
		// A terminal credential rejection exits with a DISTINCT code so the
		// operator/orchestrator can tell it apart from a transient fatal and
		// NOT blindly restart into an infinite crash-loop (re-provision needed).
		var rejected agent.ErrSessionRejected
		if errors.As(err, &rejected) {
			fmt.Fprintln(os.Stderr, "worker: terminal:", err)
			os.Exit(agent.ExitCodeSessionRejected)
		}
		fmt.Fprintln(os.Stderr, "worker: fatal:", err)
		os.Exit(1)
	}
}
