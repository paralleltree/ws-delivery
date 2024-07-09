package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type Message[T any] struct {
	Body T
	Err  error
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		log.Fatalf("err: %v", err)
	}
}

func run(ctx context.Context) error {
	var inboxCh <-chan string // TODO

	serverConf := ServerConfig{
		Port:        os.Getenv("PORT"),
		AcceptToken: os.Getenv("ACCEPT_TOKEN"),
	}
	if err := serve(ctx, serverConf, inboxCh); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}
