package streamclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestRingBuffer(t *testing.T) {
	// Ring buffer with max 3 entries or 100 bytes
	rb := NewRingBuffer(3, 100)

	rb.Push(types.StreamLogEntry{ID: "1", Payload: "hello", Size: 5})
	rb.Push(types.StreamLogEntry{ID: "2", Payload: "world", Size: 5})
	rb.Push(types.StreamLogEntry{ID: "3", Payload: "foo", Size: 3})

	if rb.Count() != 3 {
		t.Fatalf("expected count 3, got %d", rb.Count())
	}
	if rb.EvictedCount() != 0 {
		t.Fatalf("expected evicted 0, got %d", rb.EvictedCount())
	}

	// 4th entry pushes out 1st
	rb.Push(types.StreamLogEntry{ID: "4", Payload: "bar", Size: 3})
	if rb.Count() != 3 {
		t.Fatalf("expected count 3, got %d", rb.Count())
	}
	if rb.EvictedCount() != 1 {
		t.Fatalf("expected evicted 1, got %d", rb.EvictedCount())
	}

	entries := rb.GetAll()
	if len(entries) != 3 || entries[0].ID != "2" || entries[2].ID != "4" {
		t.Fatalf("unexpected entries order: %+v", entries)
	}

	// Large payload exceeding maxBytes (100 bytes)
	largePayload := strings.Repeat("A", 120)
	rb.Push(types.StreamLogEntry{ID: "5", Payload: largePayload, Size: len(largePayload)})

	// Should evict until within limits (at least keeping 1 entry)
	if rb.Count() != 1 {
		t.Fatalf("expected 1 entry left, got %d", rb.Count())
	}
	if rb.GetAll()[0].ID != "5" {
		t.Fatalf("expected entry 5, got %s", rb.GetAll()[0].ID)
	}

	rb.Clear()
	if rb.Count() != 0 || rb.EvictedCount() != 0 {
		t.Fatalf("expected cleared buffer")
	}
}

func TestWebSocket_ConnectAndEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Echo back
			if err := conn.WriteMessage(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	var receivedCount atomic.Int64
	done := make(chan struct{})

	client := NewWSClient(WSClientConfig{
		StreamID: "test_ws",
		URL:      wsURL,
		OnLog: func(entry types.StreamLogEntry) {
			if entry.Direction == "receive" && entry.Payload == "hello pebblepost" {
				receivedCount.Add(1)
				close(done)
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close(CloseNormalClosure, "test finished")

	if err := client.Send("hello pebblepost", "text"); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for echo response")
	}

	if receivedCount.Load() != 1 {
		t.Fatalf("expected 1 received message, got %d", receivedCount.Load())
	}

	status := client.Status()
	if status.TotalSent < 1 || status.TotalReceived < 1 {
		t.Fatalf("unexpected status counters: %+v", status)
	}
}

func TestWebSocket_ExplicitClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := NewWSClient(WSClientConfig{
		StreamID: "test_close",
		URL:      wsURL,
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if err := client.Close(CloseNormalClosure, "closing gracefully"); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	status := client.Status()
	if status.State != "disconnected" {
		t.Fatalf("expected disconnected state, got %s", status.State)
	}
	if status.CloseCode != CloseNormalClosure {
		t.Fatalf("expected close code %d, got %d", CloseNormalClosure, status.CloseCode)
	}
}

func TestWebSocket_PingPong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := NewWSClient(WSClientConfig{
		StreamID: "test_ping",
		URL:      wsURL,
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close(CloseNormalClosure, "")

	if err := client.Send("heartbeat", "ping"); err != nil {
		t.Fatalf("failed to send ping: %v", err)
	}

	logs := client.RingBuffer().GetAll()
	foundPing := false
	for _, l := range logs {
		if l.Type == "ping" && l.Direction == "send" {
			foundPing = true
			break
		}
	}
	if !foundPing {
		t.Fatalf("expected send ping log entry")
	}
}

func TestWebSocket_AutoReconnect(t *testing.T) {
	var connectCount atomic.Int64
	var activeConn *websocket.Conn
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connectCount.Add(1)
		mu.Lock()
		activeConn = conn
		mu.Unlock()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	reconnectedCh := make(chan struct{})
	client := NewWSClient(WSClientConfig{
		StreamID:             "test_reconnect",
		URL:                  wsURL,
		AutoReconnect:        true,
		MaxReconnectAttempts: 3,
		ReconnectInterval:    50 * time.Millisecond,
		OnStatus: func(status types.StreamSessionStatus) {
			if status.ReconnectCount > 0 && status.State == "connected" {
				select {
				case <-reconnectedCh:
				default:
					close(reconnectedCh)
				}
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close(CloseNormalClosure, "")

	// Abruptly close the server side connection to trigger auto-reconnect
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if activeConn != nil {
		_ = activeConn.Close()
	}
	mu.Unlock()

	select {
	case <-reconnectedCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for auto-reconnect")
	}

	if connectCount.Load() < 2 {
		t.Fatalf("expected at least 2 connections, got %d", connectCount.Load())
	}
}

func TestWebSocket_LargeMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	largePayload := strings.Repeat("X", 256*1024) // 256 KB
	receivedCh := make(chan struct{})

	client := NewWSClient(WSClientConfig{
		StreamID: "test_large",
		URL:      wsURL,
		OnLog: func(entry types.StreamLogEntry) {
			if entry.Direction == "receive" && len(entry.Payload) == len(largePayload) {
				close(receivedCh)
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close(CloseNormalClosure, "")

	if err := client.Send(largePayload, "text"); err != nil {
		t.Fatalf("send large failed: %v", err)
	}

	select {
	case <-receivedCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for large message echo")
	}

	if client.RingBuffer().TotalBytes() < int64(len(largePayload)) {
		t.Fatalf("expected ring buffer byte tracking")
	}
}

func TestSSE_ConnectAndReceive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		fmt.Fprintf(w, "id: 1\nevent: greeting\ndata: Hello from SSE 1\n\n")
		flusher.Flush()

		time.Sleep(20 * time.Millisecond)
		fmt.Fprintf(w, "id: 2\nevent: update\ndata: {\"count\": 42}\n\n")
		flusher.Flush()

		// keep open briefly
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	var receivedCount atomic.Int64
	done := make(chan struct{})

	client := NewSSEClient(SSEClientConfig{
		StreamID: "test_sse",
		URL:      server.URL,
		OnLog: func(entry types.StreamLogEntry) {
			if entry.Direction == "receive" {
				if receivedCount.Add(1) == 2 {
					close(done)
				}
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for 2 SSE events")
	}

	entries := client.RingBuffer().GetAll()
	var foundGreeting, foundUpdate bool
	for _, e := range entries {
		if e.Type == "greeting" && strings.Contains(e.Payload, "Hello from SSE 1") {
			foundGreeting = true
		}
		if e.Type == "update" && strings.Contains(e.Payload, "42") {
			foundUpdate = true
		}
	}

	if !foundGreeting || !foundUpdate {
		t.Fatalf("missing expected SSE events: %+v", entries)
	}
}

func TestStream_AssertionsAndScripts(t *testing.T) {
	scriptEngine := scripting.NewEngine()

	resp := &types.ExecutionResult{
		StatusCode: 200,
		StatusText: "OK",
		StreamLogs: []types.StreamLogEntry{
			{Index: 1, Direction: "send", Type: "text", Payload: `{"action":"subscribe"}`},
			{Index: 2, Direction: "receive", Type: "text", Payload: `{"event":"subscribed","channel":"trades"}`},
			{Index: 3, Direction: "receive", Type: "text", Payload: `{"price":105.5,"symbol":"BTC"}`},
		},
		StreamCloseCode:   1000,
		StreamCloseReason: "Normal",
	}

	script := `
pb.test("receives subscribed event", function() {
    var hasSub = pb.stream.waitFor(function(msg) {
        return msg && msg.event === "subscribed";
    });
    pb.expect(hasSub).to.be.true;
});

pb.test("receives price trade", function() {
    var hasPrice = pb.stream.waitFor(function(msg) {
        return msg && msg.price > 100;
    });
    pb.expect(hasPrice).to.be.true;
});

pb.test("pb.response.json parses latest received payload", function() {
    var last = pb.response.json();
    pb.expect(last.symbol).to.equal("BTC");
});

pb.test("pb.stream.messages returns received array", function() {
    var msgs = pb.stream.messages();
    pb.expect(msgs.length).to.equal(2);
});

pb.test("close code is 1000", function() {
    pb.expect(pb.stream.closeCode()).to.equal(1000);
});
`

	result, err := scriptEngine.ExecutePostResponseNamed("test-stream", script, &types.RequestDefinition{}, resp, map[string]string{}, 2*time.Second)
	if err != nil {
		t.Fatalf("script execution error: %v", err)
	}

	if len(result.Tests) != 5 {
		t.Fatalf("expected 5 tests, got %d", len(result.Tests))
	}

	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("test '%s' failed: %s", test.Name, test.Message)
		}
	}
}
