package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"slices"
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

func connectToSource(ctx context.Context, path string, predicate func(payload map[string]interface{}) bool) <-chan Message[string] {
	ch := make(chan Message[string])
	go func() {
		for v := range tailLog(ctx, path) {
			if v.Err != nil {
				ch <- Message[string]{Err: fmt.Errorf("tail: %w", v.Err)}
				continue
			}

			payload, err := parseVRCEvent(v.Body)
			if err != nil {
				ch <- Message[string]{Err: fmt.Errorf("parse event: %w", v.Err)}
				continue
			}

			if predicate(payload) {
				if raw, ok := payload["raw"].(string); ok {
					ch <- Message[string]{Body: raw}
				}
			}
		}
	}()
	return ch
}

func parseVRCEvent(body string) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return payload, nil
}

func parseInstanceOwner(instanceID string) string {
	r := regexp.MustCompile(`(hidden|friends|private|group)\(([^\)]+)\)`)
	m := r.FindStringSubmatch(instanceID)
	if m == nil {
		return ""
	}
	return m[2]
}

func buildPredicateChain(predicates ...func(payload map[string]interface{}) bool) func(payload map[string]interface{}) bool {
	return func(payload map[string]interface{}) bool {
		for _, pred := range predicates {
			if pred(payload) {
				return true
			}
		}
		return false
	}
}

func predicateBuilder(allowUserID string, allowInstanceOwnerID []string) func(payload map[string]interface{}) bool {
	return func(payload map[string]interface{}) bool {
		eventType, ok := payload["message.type"].(string)
		if !ok {
			return false
		}

		switch eventType {
		case "friend-location":
			if id, ok := payload["message.content.user.id"].(string); ok {
				if allowUserID != id {
					return false
				}
			}

			if instanceID, ok := payload["message.content.location"].(string); ok {
				instanceOwner := parseInstanceOwner(instanceID)
				if !slices.Contains(allowInstanceOwnerID, instanceOwner) {
					return false
				}
			}

			return true
		}

		return false
	}
}
