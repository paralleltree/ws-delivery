package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	wsdelivery "github.com/paralleltree/ws-delivery"
)

type appConfig struct {
	SourcePath            string
	AllowUserID           string
	AllowInstanceOwnerIDs []string
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		log.Fatalf("err: %v", err)
	}
}

func run(ctx context.Context) error {
	appConf := loadAppConfig()

	predicate := wsdelivery.BuildPredicateChain(wsdelivery.PredicateBuilder(appConf.AllowUserID, appConf.AllowInstanceOwnerIDs))
	srcCh := wsdelivery.ConnectToSource(ctx, appConf.SourcePath, predicate)
	inboxCh := make(chan string)
	go func() {
		for m := range srcCh {
			if m.Err != nil {
				fmt.Fprintf(os.Stderr, "read from source: %v\n", m.Err)
				continue
			}
			fmt.Fprintf(os.Stderr, "recv: %s\n", m.Body)
			inboxCh <- m.Body
		}
	}()

	serverConf := wsdelivery.ServerConfig{
		Port:        os.Getenv("PORT"),
		AcceptToken: os.Getenv("ACCEPT_TOKEN"),
	}
	if err := wsdelivery.Serve(ctx, serverConf, inboxCh); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func loadAppConfig() appConfig {
	allowInstanceOwnerIDsString := os.Getenv("ALLOW_INSTANCE_OWNER_IDS")
	allowInstanceOwnerIDs := make([]string, 0, len(allowInstanceOwnerIDsString))
	for _, id := range strings.Split(allowInstanceOwnerIDsString, ",") {
		allowInstanceOwnerIDs = append(allowInstanceOwnerIDs, strings.TrimSpace(id))
	}
	return appConfig{
		AllowUserID:           os.Getenv("ALLOW_USER_ID"),
		AllowInstanceOwnerIDs: allowInstanceOwnerIDs,
		SourcePath:            os.Getenv("SOURCE_PATH"),
	}
}
