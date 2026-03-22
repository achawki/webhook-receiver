package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/achawki/webhook-receiver/cmd/receiver"
)

func main() {
	server := receiver.Setup()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
