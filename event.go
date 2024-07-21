package wsdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

type MutateRawEvent func(map[string]any) error

func ConnectToSource(ctx context.Context, path string, predicate func(payload map[string]interface{}) (bool, MutateRawEvent)) <-chan Message[string] {
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

			result, modifyFunc := predicate(payload)
			if !result {
				continue
			}

			if modifyFunc != nil {
				if err := mutateRawEvent(payload, modifyFunc); err != nil {
					ch <- Message[string]{Err: fmt.Errorf("modify event: %w", v.Err)}
					continue
				}
			}

			if raw, ok := payload["raw"].(string); ok {
				ch <- Message[string]{Body: raw}
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

func BuildPredicateChain(predicates ...func(payload map[string]interface{}) (bool, MutateRawEvent)) func(payload map[string]interface{}) (bool, MutateRawEvent) {
	return func(payload map[string]interface{}) (bool, MutateRawEvent) {
		for _, pred := range predicates {
			if result, mutateFunc := pred(payload); result {
				return true, mutateFunc
			}
		}
		return false, nil
	}
}

// Returns boolean value indicating whether the event can be sent and a function to modify the raw content if necessary.
func PredicateBuilder(allowUserID string, allowInstanceOwnerID []string) func(payload map[string]interface{}) (bool, MutateRawEvent) {
	return func(payload map[string]interface{}) (bool, MutateRawEvent) {
		eventType, ok := payload["message.type"].(string)
		if !ok {
			return false, nil
		}

		switch eventType {
		case "friend-location":
			if id, ok := payload["message.content.user.id"].(string); ok {
				if allowUserID != id {
					return false, nil
				}
			}

			if instanceID, ok := payload["message.content.location"].(string); ok {
				if instanceID == "private" {
					return true, nil
				}

				if instanceID == "traveling" {
					if travelingToLocation, ok := payload["message.content.travelingToLocation"].(string); ok {
						instanceID = travelingToLocation
					}
				}

				instanceOwner := ParseInstanceOwner(instanceID)
				if slices.Contains(allowInstanceOwnerID, instanceOwner) {
					return true, nil
				}
			}

			return false, nil

		case "friend-offline":
			if id, ok := payload["message.content.userId"].(string); ok {
				return id == allowUserID, nil
			}
		}

		return false, nil
	}
}

// Mutates the raw content in the payload using the modifier function.
func mutateRawEvent(payload map[string]any, modifier func(map[string]any) error) error {
	rawJSON, ok := payload["raw"].(string)
	if !ok {
		return fmt.Errorf("fetch raw payload")
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}

	contentJSON, ok := raw["content"].(string)
	if !ok {
		return fmt.Errorf("fetch raw content")
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return fmt.Errorf("unmarshal content: %w", err)
	}

	if err := modifier(content); err != nil {
		return err
	}

	maskedContentJSON, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	raw["content"] = string(maskedContentJSON)

	maskedRawJSON, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	payload["raw"] = string(maskedRawJSON)
	return nil
}
