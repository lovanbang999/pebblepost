package runner

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

func TestRunner_WebSocketCollection(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

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
			// Respond with JSON reply
			reply := fmt.Sprintf(`{"status":"ack","received":%s}`, string(data))
			if err := conn.WriteMessage(msgType, []byte(reply)); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	tmpDir, err := os.MkdirTemp("", "pebblepost-ws-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	wsSvc := workspace.NewWorkspaceService()

	wsReq := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test WS Runner",
		Protocol:      "websocket",
		Method:        "WS",
		URL:           wsURL,
		Stream: &types.StreamDefinition{
			OutgoingMessages: []types.WebSocketMessage{
				{Payload: `{"hello":"world"}`, Type: "text"},
			},
			MaxWaitMessages: 1,
			TimeoutMs:       3000,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
pb.test("receives ack from websocket", function() {
    var hasAck = pb.stream.waitFor(function(msg) {
        return msg && msg.status === "ack";
    });
    pb.expect(hasAck).to.be.true;
});

pb.test("ack contains original payload", function() {
    var msg = pb.response.json();
    pb.expect(msg.received.hello).to.equal("world");
});
`,
		},
	}

	reqPath := filepath.Join(tmpDir, "ws_test.pebble.json")
	if err := wsSvc.SaveRequest(reqPath, wsReq); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	r := NewRunner()
	var stdout bytes.Buffer

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   tmpDir,
		ReportFormat: "terminal",
		Writer:       &stdout,
	})
	if err != nil {
		t.Fatalf("runner execution failed: %v", err)
	}

	if summary.TotalRequests != 1 {
		t.Fatalf("expected 1 request, got %d", summary.TotalRequests)
	}
	if summary.PassedRequests != 1 {
		t.Fatalf("expected 1 passed request, got %d. Output: %s", summary.PassedRequests, stdout.String())
	}
	if summary.TotalTests != 2 || summary.PassedTests != 2 {
		t.Fatalf("expected 2 passed tests, got %d/%d. Output: %s", summary.PassedTests, summary.TotalTests, stdout.String())
	}
}

func TestRunner_SSECollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		fmt.Fprintf(w, "event: init\ndata: {\"ready\":true,\"app\":\"pebblepost\"}\n\n")
		flusher.Flush()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "pebblepost-sse-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	wsSvc := workspace.NewWorkspaceService()

	sseReq := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Test SSE Runner",
		Protocol:      "sse",
		Method:        "SSE",
		URL:           server.URL,
		Stream: &types.StreamDefinition{
			MaxWaitMessages: 1,
			TimeoutMs:       3000,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
pb.test("receives ready event from sse", function() {
    var hasReady = pb.stream.waitFor(function(msg) {
        return msg && msg.ready === true;
    });
    pb.expect(hasReady).to.be.true;
});

pb.test("sse payload app is pebblepost", function() {
    var msg = pb.response.json();
    pb.expect(msg.app).to.equal("pebblepost");
});
`,
		},
	}

	reqPath := filepath.Join(tmpDir, "sse_test.pebble.json")
	if err := wsSvc.SaveRequest(reqPath, sseReq); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	r := NewRunner()
	var stdout bytes.Buffer

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   tmpDir,
		ReportFormat: "terminal",
		Writer:       &stdout,
	})
	if err != nil {
		t.Fatalf("runner execution failed: %v", err)
	}

	if summary.TotalRequests != 1 {
		t.Fatalf("expected 1 request, got %d", summary.TotalRequests)
	}
	if summary.PassedRequests != 1 {
		t.Fatalf("expected 1 passed request, got %d. Output: %s", summary.PassedRequests, stdout.String())
	}
	if summary.TotalTests != 2 || summary.PassedTests != 2 {
		t.Fatalf("expected 2 passed tests, got %d/%d. Output: %s", summary.PassedTests, summary.TotalTests, stdout.String())
	}
}
