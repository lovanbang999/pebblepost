package streamclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"pebblepost/internal/types"
)

var gqlWSUpgrader = websocket.Upgrader{
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{SubprotocolGraphQLTransportWS},
}

// setupFakeGraphQLWSServer spins up an httptest server speaking the graphql-transport-ws protocol.
func setupFakeGraphQLWSServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := gqlWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var msg struct {
				ID      string          `json:"id,omitempty"`
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload,omitempty"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "connection_init":
				// Acknowledge connection
				ack, _ := json.Marshal(map[string]string{"type": "connection_ack"})
				_ = conn.WriteMessage(websocket.TextMessage, ack)

			case "subscribe":
				// Emit 3 subscription data events
				go func(subID string) {
					for i := 1; i <= 3; i++ {
						nextMsg, _ := json.Marshal(map[string]any{
							"id":   subID,
							"type": "next",
							"payload": map[string]any{
								"data": map[string]any{
									"messageAdded": map[string]any{
										"id":   i,
										"text": "Stream event",
									},
								},
							},
						})
						_ = conn.WriteMessage(websocket.TextMessage, nextMsg)
						time.Sleep(20 * time.Millisecond)
					}

					// Send complete message
					compMsg, _ := json.Marshal(map[string]string{
						"id":   subID,
						"type": "complete",
					})
					_ = conn.WriteMessage(websocket.TextMessage, compMsg)
				}(msg.ID)

			case "ping":
				pong, _ := json.Marshal(map[string]string{"type": "pong"})
				_ = conn.WriteMessage(websocket.TextMessage, pong)

			case "complete":
				// Client cancelled subscription
				return
			}
		}
	}))
}

func TestGraphQLWS_SubscriptionLifecycle(t *testing.T) {
	server := setupFakeGraphQLWSServer(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	var receivedCount atomic.Int64
	done := make(chan struct{})

	client := NewGraphQLWSClient(GraphQLWSConfig{
		StreamID:      "test_gql_sub",
		URL:           wsURL,
		Query:         "subscription OnMessage { messageAdded { id text } }",
		OperationName: "OnMessage",
		Variables:     map[string]any{"channel": "general"},
		OnLog: func(entry types.StreamLogEntry) {
			if entry.Direction == "receive" && strings.Contains(entry.Payload, "Stream event") {
				if receivedCount.Add(1) == 3 {
					close(done)
				}
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	select {
	case <-done:
		// Succeeded in receiving all 3 stream events
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for subscription events, received %d", receivedCount.Load())
	}

	if status := client.Status(); status.Protocol != "graphql-ws" {
		t.Errorf("expected protocol graphql-ws, got %s", status.Protocol)
	}

	entries := client.RingBuffer().GetAll()
	if len(entries) < 4 {
		t.Errorf("expected at least 4 log entries, got %d", len(entries))
	}
}

func TestStreamManager_StartGraphQLWS(t *testing.T) {
	server := setupFakeGraphQLWSServer(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	mgr := NewStreamManager()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session, err := mgr.StartGraphQLWS(ctx, GraphQLWSConfig{
		StreamID: "mgr_test_sub",
		URL:      wsURL,
		Query:    "subscription { test }",
	})
	if err != nil {
		t.Fatalf("StartGraphQLWS failed: %v", err)
	}
	defer session.Close(CloseNormalClosure, "test done")

	if session.Protocol != "graphql-ws" {
		t.Errorf("expected session protocol graphql-ws, got %s", session.Protocol)
	}

	retrieved, ok := mgr.GetSession("mgr_test_sub")
	if !ok || retrieved != session {
		t.Errorf("expected retrieved session to match created session")
	}
}
