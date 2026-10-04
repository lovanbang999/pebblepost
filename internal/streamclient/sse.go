package streamclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pebblepost/internal/types"
)

// SSEClientConfig holds configuration for the Server-Sent Events client.
type SSEClientConfig struct {
	StreamID             string
	URL                  string
	Headers              http.Header
	AutoReconnect        bool
	MaxReconnectAttempts int
	ReconnectInterval    time.Duration
	TLSConfig            *tls.Config
	ProxyURL             string
	RingBuffer           *RingBuffer
	OnLog                LogCallback
	OnStatus             StatusCallback
}

// SSEClient manages an EventSource stream connection.
type SSEClient struct {
	cfg            SSEClientConfig
	mu             sync.Mutex
	state          string // "connecting", "connected", "disconnected", "reconnecting"
	totalReceived  atomic.Int64
	reconnectCount atomic.Int64
	logSeq         atomic.Int64
	lastEventID    string

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	isClosed  atomic.Bool
	ring      *RingBuffer
	respBody  io.ReadCloser
}

// NewSSEClient creates a new SSEClient instance.
func NewSSEClient(cfg SSEClientConfig) *SSEClient {
	if cfg.MaxReconnectAttempts <= 0 {
		cfg.MaxReconnectAttempts = 5
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 1 * time.Second
	}
	ring := cfg.RingBuffer
	if ring == nil {
		ring = NewRingBuffer(1000, 5*1024*1024)
	}

	return &SSEClient{
		cfg:   cfg,
		state: "disconnected",
		ring:  ring,
	}
}

// Connect establishes the HTTP GET SSE connection and streams events.
func (c *SSEClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state == "connected" || c.state == "connecting" {
		c.mu.Unlock()
		return errors.New("client already connected or connecting")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.isClosed.Store(false)
	c.closeOnce = sync.Once{}
	c.mu.Unlock()

	return c.dialAndRun()
}

func (c *SSEClient) dialAndRun() error {
	transport := &http.Transport{
		TLSClientConfig: c.cfg.TLSConfig,
	}

	if c.cfg.ProxyURL != "" {
		if proxyURL, err := url.Parse(c.cfg.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(c.ctx, "GET", c.cfg.URL, nil)
	if err != nil {
		return err
	}

	// Copy headers and set SSE Accept header
	for k, vv := range c.cfg.Headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("Cache-Control", "no-cache")

	c.mu.Lock()
	if c.lastEventID != "" {
		req.Header.Set("Last-Event-ID", c.lastEventID)
	}
	c.mu.Unlock()

	c.setState("connecting")
	c.recordLog("system", "open", fmt.Sprintf("Connecting to SSE %s...", c.cfg.URL), false)

	resp, err := client.Do(req)
	if err != nil {
		c.setState("disconnected")
		c.recordLog("system", "error", fmt.Sprintf("SSE connection failed: %v", err), true)
		if c.cfg.AutoReconnect && !c.isClosed.Load() {
			go c.reconnectLoop()
		}
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		c.setState("disconnected")
		errMsg := fmt.Sprintf("SSE server responded with HTTP %d %s", resp.StatusCode, resp.Status)
		c.recordLog("system", "error", errMsg, true)
		if c.cfg.AutoReconnect && !c.isClosed.Load() {
			go c.reconnectLoop()
		}
		return errors.New(errMsg)
	}

	c.mu.Lock()
	c.respBody = resp.Body
	c.mu.Unlock()

	c.setState("connected")
	c.recordLog("system", "open", fmt.Sprintf("Connected to SSE %s", c.cfg.URL), false)

	go c.readPump(resp.Body)
	return nil
}

func (c *SSEClient) readPump(body io.ReadCloser) {
	defer func() {
		_ = body.Close()
		c.mu.Lock()
		c.respBody = nil
		c.mu.Unlock()

		if !c.isClosed.Load() && c.cfg.AutoReconnect {
			c.reconnectLoop()
		} else {
			c.setState("disconnected")
		}
	}()

	scanner := bufio.NewScanner(body)
	var currentEvent string
	var currentData strings.Builder
	var currentID string

	for scanner.Scan() {
		if c.ctx.Err() != nil || c.isClosed.Load() {
			break
		}

		line := scanner.Text()

		// An empty line signals the dispatch of the pending event
		if len(line) == 0 {
			if currentData.Len() > 0 {
				dataStr := currentData.String()
				// Remove trailing newline if present
				dataStr = strings.TrimSuffix(dataStr, "\n")
				eventType := currentEvent
				if eventType == "" {
					eventType = "message"
				}

				if currentID != "" {
					c.mu.Lock()
					c.lastEventID = currentID
					c.mu.Unlock()
				}

				c.totalReceived.Add(1)
				c.recordLog("receive", eventType, dataStr, false)
			}
			currentEvent = ""
			currentData.Reset()
			currentID = ""
			continue
		}

		// Comment line
		if strings.HasPrefix(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		field := parts[0]
		value := ""
		if len(parts) > 1 {
			value = strings.TrimPrefix(parts[1], " ")
		}

		switch field {
		case "event":
			currentEvent = value
		case "data":
			currentData.WriteString(value)
			currentData.WriteString("\n")
		case "id":
			currentID = value
		case "retry":
			// Can update reconnect interval if needed
		}
	}

	if err := scanner.Err(); err != nil && !c.isClosed.Load() && c.ctx.Err() == nil {
		c.recordLog("system", "error", fmt.Sprintf("SSE stream interrupted: %v", err), true)
	} else if !c.isClosed.Load() {
		c.recordLog("system", "close", "SSE stream ended by server", false)
	}
}

func (c *SSEClient) reconnectLoop() {
	if c.isClosed.Load() {
		return
	}

	attempts := int(c.reconnectCount.Load())
	if attempts >= c.cfg.MaxReconnectAttempts {
		c.setState("disconnected")
		c.recordLog("system", "error", fmt.Sprintf("Auto-reconnect failed after %d attempts", attempts), true)
		return
	}

	c.reconnectCount.Add(1)
	c.setState("reconnecting")

	backoff := c.cfg.ReconnectInterval
	for i := 0; i < attempts; i++ {
		backoff = time.Duration(float64(backoff) * 1.5)
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
			break
		}
	}

	c.recordLog("system", "open", fmt.Sprintf("Attempting to reconnect SSE (%d/%d) in %v...", attempts+1, c.cfg.MaxReconnectAttempts, backoff), false)

	select {
	case <-c.ctx.Done():
		return
	case <-time.After(backoff):
	}

	if c.isClosed.Load() {
		return
	}

	_ = c.dialAndRun()
}

// Close disconnects the SSE connection.
func (c *SSEClient) Close() error {
	c.closeOnce.Do(func() {
		c.isClosed.Store(true)

		c.mu.Lock()
		if c.respBody != nil {
			_ = c.respBody.Close()
		}
		c.mu.Unlock()

		if c.cancel != nil {
			c.cancel()
		}
		c.setState("disconnected")
		c.recordLog("system", "close", "Disconnected by user", false)
	})
	return nil
}

func (c *SSEClient) recordLog(direction, logType, payload string, isError bool) {
	idx := int(c.logSeq.Add(1))
	entry := types.StreamLogEntry{
		ID:        fmt.Sprintf("log_%s_%d", c.cfg.StreamID, idx),
		Index:     idx,
		Direction: direction,
		Type:      logType,
		Timestamp: time.Now(),
		Payload:   payload,
		Size:      len(payload),
		IsError:   isError,
	}

	c.ring.Push(entry)

	if c.cfg.OnLog != nil {
		c.cfg.OnLog(entry)
	}
}

func (c *SSEClient) setState(s string) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()

	if c.cfg.OnStatus != nil {
		c.cfg.OnStatus(c.Status())
	}
}

// Status returns current snapshot of the session.
func (c *SSEClient) Status() types.StreamSessionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return types.StreamSessionStatus{
		StreamID:       c.cfg.StreamID,
		Protocol:       "sse",
		State:          c.state,
		URL:            c.cfg.URL,
		ReconnectCount: int(c.reconnectCount.Load()),
		TotalSent:      0,
		TotalReceived:  int(c.totalReceived.Load()),
		EvictedCount:   c.ring.EvictedCount(),
	}
}

// RingBuffer returns the underlying ring buffer of logs.
func (c *SSEClient) RingBuffer() *RingBuffer {
	return c.ring
}
