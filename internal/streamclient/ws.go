package streamclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"pebblepost/internal/types"
)

// WSClientConfig holds configuration for the WebSocket client.
type WSClientConfig struct {
	StreamID             string
	URL                  string
	Headers              http.Header
	Subprotocols         []string
	AutoReconnect        bool
	MaxReconnectAttempts int
	ReconnectInterval    time.Duration
	PingInterval         time.Duration
	TLSConfig            *tls.Config
	ProxyURL             string
	RingBuffer           *RingBuffer
	OnLog                LogCallback
	OnStatus             StatusCallback
}

// WSClient manages a live WebSocket session with auto-reconnect, ping/pong, and ring buffering.
type WSClient struct {
	cfg            WSClientConfig
	conn           *websocket.Conn
	mu             sync.Mutex
	writeMu        sync.Mutex
	state          string // "connecting", "connected", "disconnected", "reconnecting"
	subprotocol    string
	totalSent      atomic.Int64
	totalReceived  atomic.Int64
	reconnectCount atomic.Int64
	logSeq         atomic.Int64
	closeCode      atomic.Int64
	closeReason    string

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	isClosed  atomic.Bool
	ring      *RingBuffer
}

// NewWSClient creates a new WSClient instance.
func NewWSClient(cfg WSClientConfig) *WSClient {
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

	return &WSClient{
		cfg:   cfg,
		state: "disconnected",
		ring:  ring,
	}
}

// Connect establishes the WebSocket connection and starts read/heartbeat pumps.
func (c *WSClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state == "connected" || c.state == "connecting" {
		c.mu.Unlock()
		return errors.New("client already connected or connecting")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.isClosed.Store(false)
	c.closeOnce = sync.Once{}
	c.closeCode.Store(0)
	c.closeReason = ""
	c.mu.Unlock()

	return c.dialAndRun()
}

func (c *WSClient) dialAndRun() error {
	dialer := websocket.Dialer{
		Subprotocols:     c.cfg.Subprotocols,
		TLSClientConfig:  c.cfg.TLSConfig,
		HandshakeTimeout: 10 * time.Second,
	}

	if c.cfg.ProxyURL != "" {
		if proxyURL, err := url.Parse(c.cfg.ProxyURL); err == nil {
			dialer.Proxy = http.ProxyURL(proxyURL)
		}
	}

	c.setState("connecting")
	c.recordLog("system", "open", fmt.Sprintf("Connecting to %s...", c.cfg.URL), false, 0, "")

	conn, resp, err := dialer.DialContext(c.ctx, c.cfg.URL, c.cfg.Headers)
	if err != nil {
		c.setState("disconnected")
		c.recordLog("system", "error", fmt.Sprintf("Connection failed: %v", err), true, 0, "")
		if c.cfg.AutoReconnect && !c.isClosed.Load() {
			go c.reconnectLoop()
		}
		return err
	}

	c.mu.Lock()
	c.conn = conn
	if resp != nil {
		c.subprotocol = resp.Header.Get("Sec-WebSocket-Protocol")
	}
	c.mu.Unlock()

	c.setState("connected")
	connectedMsg := fmt.Sprintf("Connected to %s", c.cfg.URL)
	if c.subprotocol != "" {
		connectedMsg += fmt.Sprintf(" (Subprotocol: %s)", c.subprotocol)
	}
	c.recordLog("system", "open", connectedMsg, false, 0, "")

	// Set ping/pong handlers
	conn.SetPingHandler(func(appData string) error {
		c.recordLog("receive", "ping", appData, false, 0, "")
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	conn.SetPongHandler(func(appData string) error {
		c.recordLog("receive", "pong", appData, false, 0, "")
		return nil
	})

	go c.readPump()
	if c.cfg.PingInterval > 0 {
		go c.pingPump()
	}

	return nil
}

func (c *WSClient) readPump() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()

		if !c.isClosed.Load() && c.cfg.AutoReconnect {
			c.reconnectLoop()
		} else {
			c.setState("disconnected")
		}
	}()

	for {
		if c.ctx.Err() != nil || c.isClosed.Load() {
			break
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			break
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			closeCode := CloseAbnormalClosure
			closeReason := err.Error()

			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				closeCode = closeErr.Code
				closeReason = closeErr.Text
			}

			c.closeCode.Store(int64(closeCode))
			c.mu.Lock()
			c.closeReason = closeReason
			c.mu.Unlock()

			desc := CloseCodeDescription(closeCode)
			c.recordLog("system", "close", fmt.Sprintf("Connection closed: %s (Reason: %s)", desc, closeReason), false, closeCode, closeReason)
			break
		}

		c.totalReceived.Add(1)
		typeStr := "text"
		if msgType == websocket.BinaryMessage {
			typeStr = "binary"
		}
		c.recordLog("receive", typeStr, string(data), false, 0, "")
	}
}

func (c *WSClient) pingPump() {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.isClosed.Load() || c.state != "connected" {
				return
			}
			c.writeMu.Lock()
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()
			if conn == nil {
				c.writeMu.Unlock()
				return
			}
			err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
			c.writeMu.Unlock()
			if err != nil {
				return
			}
			c.recordLog("send", "ping", "ping", false, 0, "")
		}
	}
}

func (c *WSClient) reconnectLoop() {
	if c.isClosed.Load() {
		return
	}

	attempts := int(c.reconnectCount.Load())
	if attempts >= c.cfg.MaxReconnectAttempts {
		c.setState("disconnected")
		c.recordLog("system", "error", fmt.Sprintf("Auto-reconnect failed after %d attempts", attempts), true, 0, "")
		return
	}

	c.reconnectCount.Add(1)
	c.setState("reconnecting")

	// Backoff interval: interval * 1.5^attempt
	backoff := c.cfg.ReconnectInterval
	for i := 0; i < attempts; i++ {
		backoff = time.Duration(float64(backoff) * 1.5)
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
			break
		}
	}

	c.recordLog("system", "open", fmt.Sprintf("Attempting to reconnect (%d/%d) in %v...", attempts+1, c.cfg.MaxReconnectAttempts, backoff), false, 0, "")

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

// Send transmits an outgoing message across the active WebSocket connection.
func (c *WSClient) Send(payload string, msgType string) error {
	c.mu.Lock()
	conn := c.conn
	state := c.state
	c.mu.Unlock()

	if state != "connected" || conn == nil {
		return errors.New("cannot send: client is not connected")
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var err error
	switch msgType {
	case "binary":
		err = conn.WriteMessage(websocket.BinaryMessage, []byte(payload))
	case "ping":
		err = conn.WriteControl(websocket.PingMessage, []byte(payload), time.Now().Add(5*time.Second))
	case "pong":
		err = conn.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(5*time.Second))
	default:
		msgType = "text"
		err = conn.WriteMessage(websocket.TextMessage, []byte(payload))
	}

	if err != nil {
		c.recordLog("system", "error", fmt.Sprintf("Failed to send message: %v", err), true, 0, "")
		return err
	}

	c.totalSent.Add(1)
	c.recordLog("send", msgType, payload, false, 0, "")
	return nil
}

// Close gracefully closes the WebSocket connection.
func (c *WSClient) Close(code int, reason string) error {
	var err error
	c.closeOnce.Do(func() {
		c.isClosed.Store(true)
		if code <= 0 {
			code = CloseNormalClosure
		}
		c.closeCode.Store(int64(code))

		c.mu.Lock()
		c.closeReason = reason
		conn := c.conn
		c.mu.Unlock()

		if conn != nil {
			c.writeMu.Lock()
			closeData := websocket.FormatCloseMessage(code, reason)
			_ = conn.WriteControl(websocket.CloseMessage, closeData, time.Now().Add(2*time.Second))
			c.writeMu.Unlock()

			_ = conn.Close()
		}

		if c.cancel != nil {
			c.cancel()
		}
		c.setState("disconnected")
		c.recordLog("system", "close", fmt.Sprintf("Disconnected by user: %s", CloseCodeDescription(code)), false, code, reason)
	})
	return err
}

func (c *WSClient) recordLog(direction, logType, payload string, isError bool, closeCode int, closeReason string) {
	idx := int(c.logSeq.Add(1))
	entry := types.StreamLogEntry{
		ID:          fmt.Sprintf("log_%s_%d", c.cfg.StreamID, idx),
		Index:       idx,
		Direction:   direction,
		Type:        logType,
		Timestamp:   time.Now(),
		Payload:     payload,
		Size:        len(payload),
		CloseCode:   closeCode,
		CloseReason: closeReason,
		IsError:     isError,
	}

	c.ring.Push(entry)

	if c.cfg.OnLog != nil {
		c.cfg.OnLog(entry)
	}
}

func (c *WSClient) setState(s string) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()

	if c.cfg.OnStatus != nil {
		c.cfg.OnStatus(c.Status())
	}
}

// Status returns current snapshot of the session.
func (c *WSClient) Status() types.StreamSessionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return types.StreamSessionStatus{
		StreamID:       c.cfg.StreamID,
		Protocol:       "websocket",
		State:          c.state,
		URL:            c.cfg.URL,
		Subprotocol:    c.subprotocol,
		ReconnectCount: int(c.reconnectCount.Load()),
		TotalSent:      int(c.totalSent.Load()),
		TotalReceived:  int(c.totalReceived.Load()),
		EvictedCount:   c.ring.EvictedCount(),
		CloseCode:      int(c.closeCode.Load()),
		CloseReason:    c.closeReason,
	}
}

// RingBuffer returns the underlying ring buffer of logs.
func (c *WSClient) RingBuffer() *RingBuffer {
	return c.ring
}
