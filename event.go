package wsdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

func ConnectToSource(ctx context.Context, path string, predicate func(payload map[string]interface{}) bool) <-chan Message[string] {
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

func ParseInstanceOwner(instanceID string) string {
	r := regexp.MustCompile(`(hidden|friends|private|group)\(([^\)]+)\)`)
	m := r.FindStringSubmatch(instanceID)
	if m == nil {
		return ""
	}
	return m[2]
}

func BuildPredicateChain(predicates ...func(payload map[string]interface{}) bool) func(payload map[string]interface{}) bool {
	return func(payload map[string]interface{}) bool {
		for _, pred := range predicates {
			if pred(payload) {
				return true
			}
		}
		return false
	}
}

func PredicateBuilder(allowUserID string, allowInstanceOwnerID []string) func(payload map[string]interface{}) bool {
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
				if instanceID == "private" {
					return true
				}

				if instanceID == "traveling" {
					if travelingToLocation, ok := payload["message.content.travelingToLocation"].(string); ok {
						instanceID = travelingToLocation
					}
				}

				instanceOwner := ParseInstanceOwner(instanceID)
				if slices.Contains(allowInstanceOwnerID, instanceOwner) {
					return true
				}
			}

			return false

		case "friend-offline":
			if id, ok := payload["message.content.userId"].(string); ok {
				return id == allowUserID
			}
		}

		return false
	}
}
