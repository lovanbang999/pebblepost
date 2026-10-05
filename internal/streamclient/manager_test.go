package streamclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pebblepost/internal/types"
)

func TestStreamSession_BasicMethods(t *testing.T) {
	session := newStreamSession("test-stream", "websocket")

	// Disconnected status when no client
	status := session.Status()
	if status.State != "disconnected" {
		t.Fatalf("expected disconnected, got %s", status.State)
	}

	if session.RingBuffer() != nil {
		t.Fatalf("expected nil ringbuffer")
	}

	// Close with no client
	if err := session.Close(1000, "normal"); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	// Subscribe & Broadcast
	ch, unsub := session.Subscribe()
	defer unsub()

	ev := StreamEvent{Type: "log", Log: &types.StreamLogEntry{ID: "entry-1"}}
	session.Broadcast(ev)

	select {
	case received := <-ch:
		if received.Type != "log" || received.Log.ID != "entry-1" {
			t.Fatalf("unexpected event: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast event")
	}

	unsub()
}

func TestStreamSession_SendErrorOnSSE(t *testing.T) {
	session := newStreamSession("sse-stream", "sse")
	session.SSEClient = NewSSEClient(SSEClientConfig{StreamID: "sse-stream", URL: "http://localhost"})

	err := session.Send("ping", "text")
	if err == nil {
		t.Fatal("expected error sending to SSE session")
	}
}

func TestStreamManager_LifecycleWS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Echo back
			if err := conn.WriteMessage(mt, msg); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	mgr := NewStreamManager()
	wsURL := "ws" + server.URL[len("http"):]

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logCh := make(chan types.StreamLogEntry, 10)
	statusCh := make(chan types.StreamSessionStatus, 10)

	cfg := WSClientConfig{
		StreamID: "test-ws-1",
		URL:      wsURL,
		OnLog: func(entry types.StreamLogEntry) {
			logCh <- entry
		},
		OnStatus: func(status types.StreamSessionStatus) {
			statusCh <- status
		},
	}

	session, err := mgr.StartWS(ctx, cfg)
	if err != nil {
		t.Fatalf("StartWS error: %v", err)
	}

	if session.Status().State != "connected" {
		t.Fatalf("expected connected, got %s", session.Status().State)
	}

	if session.RingBuffer() == nil {
		t.Fatal("expected non-nil ringbuffer")
	}

	// Lookup
	gotSession, ok := mgr.GetSession("test-ws-1")
	if !ok || gotSession != session {
		t.Fatalf("GetSession failed")
	}

	// Send message via manager
	err = mgr.Send("test-ws-1", "hello manager", "text")
	if err != nil {
		t.Fatalf("mgr.Send error: %v", err)
	}

	// Wait for echo
	select {
	case entry := <-logCh:
		if entry.Direction == "outgoing" {
			// Now wait for incoming
			select {
			case incoming := <-logCh:
				if incoming.Payload != "hello manager" {
					t.Fatalf("expected hello manager, got %s", incoming.Payload)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("timeout waiting for incoming echo")
			}
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for log")
	}

	// Replacing session
	session2, err := mgr.StartWS(ctx, cfg)
	if err != nil {
		t.Fatalf("StartWS replace error: %v", err)
	}
	if session2 == session {
		t.Fatal("expected new session instance on replace")
	}

	// Close session
	if err := mgr.CloseSession("test-ws-1", CloseNormalClosure, "done"); err != nil {
		t.Fatalf("CloseSession error: %v", err)
	}

	// Close already closed or missing session
	if err := mgr.CloseSession("test-ws-1", CloseNormalClosure, "done"); err == nil {
		t.Fatal("expected error on closing missing session")
	}

	// Send on missing session
	if err := mgr.Send("test-ws-1", "ping", "text"); err == nil {
		t.Fatal("expected error sending to missing session")
	}
}

func TestStreamManager_LifecycleSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		w.Write([]byte("data: sse-test-1\n\n"))
		flusher.Flush()

		// keep open briefly
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	mgr := NewStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logCh := make(chan types.StreamLogEntry, 10)
	cfg := SSEClientConfig{
		StreamID: "test-sse-1",
		URL:      server.URL,
		OnLog: func(entry types.StreamLogEntry) {
			logCh <- entry
		},
	}

	session, err := mgr.StartSSE(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSSE error: %v", err)
	}
	defer mgr.CloseSession("test-sse-1", 0, "")

	if session.Status().State != "connected" {
		t.Fatalf("expected connected, got %s", session.Status().State)
	}

	found := false
	deadline := time.After(2 * time.Second)
	for !found {
		select {
		case entry := <-logCh:
			if entry.Payload == "sse-test-1" {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for sse-test-1 message")
		}
	}

	// Replace SSE session
	_, err = mgr.StartSSE(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSSE replace error: %v", err)
	}
}
