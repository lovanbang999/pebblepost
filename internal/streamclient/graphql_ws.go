package streamclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"pebblepost/internal/types"
)

// SubprotocolGraphQLTransportWS is the official subprotocol identifier for graphql-ws.
const SubprotocolGraphQLTransportWS = "graphql-transport-ws"

// GraphQLWSConfig holds configuration for the GraphQL WebSocket subscription client.
type GraphQLWSConfig struct {
	StreamID             string
	URL                  string
	Headers              http.Header
	ConnectionParams     map[string]any // sent in connection_init payload
	Query                string
	Variables            map[string]any
	OperationName        string
	AutoReconnect        bool
	MaxReconnectAttempts int
	ReconnectInterval    time.Duration
	TLSConfig            *tls.Config
	ProxyURL             string
	RingBuffer           *RingBuffer
	OnLog                LogCallback
	OnStatus             StatusCallback
}

// GraphQLWSClient manages a GraphQL subscription session over WebSocket implementing the graphql-transport-ws protocol.
type GraphQLWSClient struct {
	cfg            GraphQLWSConfig
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

// NewGraphQLWSClient creates a new GraphQLWSClient instance.
func NewGraphQLWSClient(cfg GraphQLWSConfig) *GraphQLWSClient {
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

	return &GraphQLWSClient{
		cfg:   cfg,
		state: "disconnected",
		ring:  ring,
	}
}

// Connect dials the WebSocket server and begins the graphql-transport-ws handshake.
func (c *GraphQLWSClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	return c.dialAndRun()
}

func (c *GraphQLWSClient) dialAndRun() error {
	c.setState("connecting")
	c.recordLog("system", "open", fmt.Sprintf("Connecting to GraphQL WebSocket at %s with subprotocol %s...", c.cfg.URL, SubprotocolGraphQLTransportWS), false, 0, "")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     []string{SubprotocolGraphQLTransportWS},
		TLSClientConfig:  c.cfg.TLSConfig,
	}

	if c.cfg.ProxyURL != "" {
		if pu, err := url.Parse(c.cfg.ProxyURL); err == nil {
			dialer.Proxy = http.ProxyURL(pu)
		}
	}

	wsURL := c.cfg.URL
	if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	} else if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	}

	conn, resp, err := dialer.DialContext(c.ctx, wsURL, c.cfg.Headers)
	if err != nil {
		c.setState("disconnected")
		c.recordLog("system", "error", fmt.Sprintf("GraphQL WS connection failed: %v", err), true, 0, "")
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
	c.recordLog("system", "open", fmt.Sprintf("Connected (Subprotocol: %s)", c.subprotocol), false, 0, "")

	// Send connection_init
	initMsg := map[string]any{
		"type": "connection_init",
	}
	if len(c.cfg.ConnectionParams) > 0 {
		initMsg["payload"] = c.cfg.ConnectionParams
	}
	initBytes, _ := json.Marshal(initMsg)
	if err := c.writeRaw(websocket.TextMessage, initBytes); err != nil {
		c.recordLog("system", "error", fmt.Sprintf("Failed to send connection_init: %v", err), true, 0, "")
		return err
	}
	c.recordLog("send", "text", string(initBytes), false, 0, "")
	c.totalSent.Add(1)

	go c.readPump()
	return nil
}

func (c *GraphQLWSClient) readPump() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.mu.Unlock()
	}()

	for {
		if c.ctx.Err() != nil || c.isClosed.Load() {
			return
		}

		conn := c.getConn()
		if conn == nil {
			return
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if c.isClosed.Load() {
				return
			}
			closeCode := CloseAbnormalClosure
			closeReason := err.Error()
			var wsErr *websocket.CloseError
			if errors.As(err, &wsErr) {
				closeCode = wsErr.Code
				closeReason = wsErr.Text
			}

			c.closeCode.Store(int64(closeCode))
			c.closeReason = closeReason
			c.setState("disconnected")
			c.recordLog("system", "close", fmt.Sprintf("Connection closed: %s (%s)", closeReason, CloseCodeDescription(closeCode)), false, closeCode, closeReason)

			if c.cfg.AutoReconnect && !c.isClosed.Load() {
				go c.reconnectLoop()
			}
			return
		}

		c.totalReceived.Add(1)
		payloadStr := string(data)

		// Parse graphql-transport-ws message
		var msg struct {
			ID      string          `json:"id,omitempty"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload,omitempty"`
		}

		if err := json.Unmarshal(data, &msg); err == nil {
			switch msg.Type {
			case "connection_ack":
				c.recordLog("receive", "text", payloadStr, false, 0, "")
				// Send subscription request on connection_ack
				c.sendSubscription()

			case "next":
				c.recordLog("receive", "text", payloadStr, false, 0, "")

			case "error":
				c.recordLog("receive", "error", payloadStr, true, 0, "")

			case "complete":
				c.recordLog("receive", "text", payloadStr, false, 0, "")

			case "ping":
				c.recordLog("receive", "ping", payloadStr, false, 0, "")
				// Send pong response
				pongMsg, _ := json.Marshal(map[string]string{"type": "pong"})
				_ = c.writeRaw(websocket.TextMessage, pongMsg)
				c.recordLog("send", "pong", string(pongMsg), false, 0, "")

			case "pong":
				c.recordLog("receive", "pong", payloadStr, false, 0, "")

			default:
				c.recordLog("receive", "text", payloadStr, false, 0, "")
			}
		} else {
			// Non-JSON or raw text frame
			c.recordLog("receive", "text", payloadStr, false, 0, "")
		}

		_ = msgType
	}
}

func (c *GraphQLWSClient) sendSubscription() {
	if strings.TrimSpace(c.cfg.Query) == "" {
		return
	}

	subPayload := map[string]any{
		"query": c.cfg.Query,
	}
	if len(c.cfg.Variables) > 0 {
		subPayload["variables"] = c.cfg.Variables
	}
	if opName := strings.TrimSpace(c.cfg.OperationName); opName != "" {
		subPayload["operationName"] = opName
	}

	subscribeMsg := map[string]any{
		"id":      "1",
		"type":    "subscribe",
		"payload": subPayload,
	}

	data, err := json.Marshal(subscribeMsg)
	if err != nil {
		c.recordLog("system", "error", fmt.Sprintf("Failed to marshal subscribe message: %v", err), true, 0, "")
		return
	}

	if err := c.writeRaw(websocket.TextMessage, data); err != nil {
		c.recordLog("system", "error", fmt.Sprintf("Failed to send subscribe message: %v", err), true, 0, "")
		return
	}

	c.recordLog("send", "text", string(data), false, 0, "")
	c.totalSent.Add(1)
}

func (c *GraphQLWSClient) writeRaw(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	conn := c.getConn()
	if conn == nil {
		return errors.New("cannot write: connection is nil")
	}
	return conn.WriteMessage(msgType, data)
}

func (c *GraphQLWSClient) reconnectLoop() {
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

// Send transmits an arbitrary outgoing message or payload.
func (c *GraphQLWSClient) Send(payload string) error {
	c.mu.Lock()
	conn := c.conn
	state := c.state
	c.mu.Unlock()

	if state != "connected" || conn == nil {
		return errors.New("cannot send: client is not connected")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err == nil && parsed["type"] != nil {
		// Already formatted graphql-transport-ws message
		if err := c.writeRaw(websocket.TextMessage, []byte(payload)); err != nil {
			return err
		}
	} else {
		// Treat payload as updated query / subscribe message
		subPayload := map[string]any{"query": payload}
		msg := map[string]any{"id": fmt.Sprintf("%d", time.Now().UnixNano()), "type": "subscribe", "payload": subPayload}
		data, _ := json.Marshal(msg)
		if err := c.writeRaw(websocket.TextMessage, data); err != nil {
			return err
		}
		payload = string(data)
	}

	c.recordLog("send", "text", payload, false, 0, "")
	c.totalSent.Add(1)
	return nil
}

// Close closes the subscription cleanly.
func (c *GraphQLWSClient) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.isClosed.Store(true)
		if c.cancel != nil {
			c.cancel()
		}

		conn := c.getConn()
		if conn != nil {
			// Send complete message before close
			completeMsg, _ := json.Marshal(map[string]string{"id": "1", "type": "complete"})
			_ = c.writeRaw(websocket.TextMessage, completeMsg)

			closeErr = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Client closed"),
				time.Now().Add(2*time.Second),
			)
			_ = conn.Close()
		}

		c.setState("disconnected")
		c.recordLog("system", "close", "Subscription connection closed", false, CloseNormalClosure, "Normal closure")
	})
	return closeErr
}

func (c *GraphQLWSClient) getConn() *websocket.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

func (c *GraphQLWSClient) setState(s string) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()

	if c.cfg.OnStatus != nil {
		c.cfg.OnStatus(c.Status())
	}
}

// Status returns current session status.
func (c *GraphQLWSClient) Status() types.StreamSessionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return types.StreamSessionStatus{
		StreamID:       c.cfg.StreamID,
		Protocol:       "graphql-ws",
		State:          c.state,
		URL:            c.cfg.URL,
		Subprotocol:    c.subprotocol,
		ReconnectCount: int(c.reconnectCount.Load()),
		TotalSent:      int(c.totalSent.Load()),
		TotalReceived:  int(c.totalReceived.Load()),
		EvictedCount:   c.ring.EvictedCount(),
		CloseCode:      int(c.closeCode.Load()),
	}
}

// RingBuffer returns the underlying ring buffer.
func (c *GraphQLWSClient) RingBuffer() *RingBuffer {
	return c.ring
}

func (c *GraphQLWSClient) recordLog(direction, logType, payload string, isError bool, closeCode int, closeReason string) {
	seq := int(c.logSeq.Add(1))
	entry := types.StreamLogEntry{
		ID:          fmt.Sprintf("log_%d_%d", time.Now().UnixNano(), seq),
		Index:       seq,
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
