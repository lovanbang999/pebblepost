package mockserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

const (
	defaultPort    = 8080
	defaultHost    = "127.0.0.1"
	maxLogCapacity = 500
)

// Server implements the PebblePost mock server engine.
type Server struct {
	mu          sync.RWMutex
	config      ServerConfig
	wsService   *workspace.WorkspaceService
	routes      []MockRoute
	overrides   map[string]*RouteOverride // route ID or "METHOD:path" -> override
	logs        []RequestLog
	subscribers map[chan RequestLog]struct{}

	httpServer *http.Server
	listener   net.Listener
	running    bool
	actualPort int
	actualHost string
	stopErr    error
}

// NewServer creates a new unstarted mock server.
func NewServer(cfg ServerConfig, wsSvc *workspace.WorkspaceService) *Server {
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port <= 0 {
		cfg.Port = defaultPort
	}
	if wsSvc == nil {
		wsSvc = workspace.NewWorkspaceService()
	}

	overrides := make(map[string]*RouteOverride)
	for k, v := range cfg.Overrides {
		overrides[k] = v
	}

	return &Server{
		config:      cfg,
		wsService:   wsSvc,
		overrides:   overrides,
		logs:        make([]RequestLog, 0, maxLogCapacity),
		subscribers: make(map[chan RequestLog]struct{}),
	}
}

// LoadRoutes recursively discovers all requests with Examples under targetPath.
func (s *Server) LoadRoutes(targetPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if targetPath == "" {
		targetPath = s.config.WorkspacePath
	}
	if targetPath == "" {
		targetPath = "."
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid mock target path: %w", err)
	}

	fi, err := os.Stat(absTarget)
	if err != nil {
		return fmt.Errorf("cannot access target path %q: %w", targetPath, err)
	}

	var discoveredRoutes []MockRoute

	if !fi.IsDir() {
		if strings.HasSuffix(absTarget, workspace.PebbleExt) {
			req, err := s.wsService.ReadRequest(absTarget)
			if err != nil {
				return fmt.Errorf("failed to read request file: %w", err)
			}
			if len(req.Examples) > 0 {
				route := s.createMockRoute(absTarget, req)
				discoveredRoutes = append(discoveredRoutes, route)
			}
		}
	} else {
		err = filepath.Walk(absTarget, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				base := info.Name()
				if strings.HasPrefix(base, ".") || base == workspace.PebbleDir {
					return filepath.SkipDir
				}
				return nil
			}

			if strings.HasSuffix(path, workspace.PebbleExt) {
				req, readErr := s.wsService.ReadRequest(path)
				if readErr == nil && len(req.Examples) > 0 {
					route := s.createMockRoute(path, req)
					discoveredRoutes = append(discoveredRoutes, route)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to scan workspace for mock routes: %w", err)
		}
	}

	// Sort routes by specificity descending (literal routes precede parameterized/wildcards)
	sort.SliceStable(discoveredRoutes, func(i, j int) bool {
		if discoveredRoutes[i].Specificity != discoveredRoutes[j].Specificity {
			return discoveredRoutes[i].Specificity > discoveredRoutes[j].Specificity
		}
		return len(discoveredRoutes[i].Segments) > len(discoveredRoutes[j].Segments)
	})

	s.routes = discoveredRoutes
	return nil
}

func (s *Server) createMockRoute(filePath string, req *types.RequestDefinition) MockRoute {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "GET"
	}

	pathPattern := ExtractPathFromURL(req.URL)
	segments, specificity := ParsePathSegments(pathPattern)
	id := fmt.Sprintf("%s:%s", method, pathPattern)

	route := MockRoute{
		ID:          id,
		RequestName: req.Name,
		FilePath:    filePath,
		Method:      method,
		RawURL:      req.URL,
		PathPattern: pathPattern,
		Segments:    segments,
		Specificity: specificity,
		Examples:    req.Examples,
	}

	if ov, ok := s.overrides[id]; ok {
		route.Override = ov
	}

	return route
}

// Start binds to the configured host and port and starts serving mock requests.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("mock server is already running on %s", s.URL())
	}

	host := s.config.Host
	if host == "" {
		host = defaultHost
	}
	port := s.config.Port
	if port <= 0 {
		port = defaultPort
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to bind mock server on %s: %w", addr, err)
	}

	tcpAddr := listener.Addr().(*net.TCPAddr)
	s.listener = listener
	s.actualHost = host
	s.actualPort = tcpAddr.Port
	s.running = true

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleMockRequest)
	s.httpServer = &http.Server{
		Handler: mux,
	}
	s.mu.Unlock()

	go func() {
		serveErr := s.httpServer.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			s.mu.Lock()
			s.stopErr = serveErr
			s.running = false
			s.mu.Unlock()
		}
	}()

	return nil
}

// Stop gracefully stops the mock server.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running || s.httpServer == nil {
		s.mu.Unlock()
		return nil
	}
	server := s.httpServer
	s.running = false
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}

	return err
}

// URL returns the base URL of the active mock server.
func (s *Server) URL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.running {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", s.actualHost, s.actualPort)
}

// IsRunning returns whether the mock server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Status returns a snapshot of server status.
func (s *Server) Status() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routesSummary := make([]MockRouteSummary, len(s.routes))
	for i, r := range s.routes {
		names := make([]string, len(r.Examples))
		for j, ex := range r.Examples {
			names[j] = ex.Name
		}
		routesSummary[i] = MockRouteSummary{
			ID:           r.ID,
			RequestName:  r.RequestName,
			FilePath:     r.FilePath,
			Method:       r.Method,
			PathPattern:  r.PathPattern,
			ExampleCount: len(r.Examples),
			ExampleNames: names,
			Override:     r.Override,
		}
	}

	host := s.actualHost
	if host == "" {
		host = s.config.Host
	}
	port := s.actualPort
	if port == 0 {
		port = s.config.Port
	}

	isNonLoopback := host != "127.0.0.1" && host != "localhost" && host != "::1" && host != ""
	warning := ""
	if isNonLoopback {
		warning = fmt.Sprintf("Mock server bound to non-loopback host %s. This exposes your mock endpoints externally.", host)
	}

	urlStr := ""
	if s.running {
		urlStr = fmt.Sprintf("http://%s:%d", host, port)
	}

	errStr := ""
	if s.stopErr != nil {
		errStr = s.stopErr.Error()
	}

	overridesCopy := make(map[string]*RouteOverride)
	for k, v := range s.overrides {
		overridesCopy[k] = v
	}

	return ServerStatus{
		Running:     s.running,
		Host:        host,
		Port:        port,
		URL:         urlStr,
		Target:      s.config.TargetPath,
		RoutesCount: len(s.routes),
		Routes:      routesSummary,
		Overrides:   overridesCopy,
		NonLoopback: isNonLoopback,
		Warning:     warning,
		Error:       errStr,
	}
}

// SetRouteOverride updates or removes an override for a given route.
func (s *Server) SetRouteOverride(routeID string, override *RouteOverride) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if override == nil {
		delete(s.overrides, routeID)
	} else {
		if override.DelayMs > 0 && override.Delay == 0 {
			override.Delay = time.Duration(override.DelayMs) * time.Millisecond
		}
		s.overrides[routeID] = override
	}

	// Update route in-memory
	for i := range s.routes {
		if s.routes[i].ID == routeID {
			s.routes[i].Override = override
			break
		}
	}
}

// Logs returns a copy of recent request logs.
func (s *Server) Logs() []RequestLog {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]RequestLog, len(s.logs))
	copy(res, s.logs)
	return res
}

// SubscribeLogs provides a channel receiving live RequestLogs.
func (s *Server) SubscribeLogs(ctx context.Context) <-chan RequestLog {
	ch := make(chan RequestLog, 50)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subscribers, ch)
		close(ch)
		s.mu.Unlock()
	}()

	return ch
}

func (s *Server) recordLog(logEntry RequestLog) {
	s.mu.Lock()
	if len(s.logs) >= maxLogCapacity {
		s.logs = s.logs[1:]
	}
	s.logs = append(s.logs, logEntry)

	// Broadcast to active subscribers
	for ch := range s.subscribers {
		select {
		case ch <- logEntry:
		default:
		}
	}
	s.mu.Unlock()
}

// handleMockRequest processes incoming requests against loaded routes.
func (s *Server) handleMockRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Add CORS headers for web test interoperability
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "*")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	reqPath := r.URL.Path
	if reqPath == "" {
		reqPath = "/"
	}

	s.mu.RLock()
	routesSnapshot := make([]MockRoute, len(s.routes))
	copy(routesSnapshot, s.routes)
	globalDelayMs := s.config.GlobalDelayMs
	globalStatus := s.config.GlobalStatusCode
	globalErrRate := s.config.GlobalErrorRate
	s.mu.RUnlock()

	var matchedRoute *MockRoute
	for i := range routesSnapshot {
		rt := &routesSnapshot[i]
		if ok, _ := MatchRoute(rt, r.Method, reqPath); ok {
			matchedRoute = rt
			break
		}
	}

	// 1. Unmatched Route Handler (HTTP 404 with helpful diagnostics)
	if matchedRoute == nil {
		duration := time.Since(start)
		available := make([]AvailableRouteSummary, 0, len(routesSnapshot))
		for _, rt := range routesSnapshot {
			names := make([]string, len(rt.Examples))
			for idx, ex := range rt.Examples {
				names[idx] = ex.Name
			}
			available = append(available, AvailableRouteSummary{
				Method:   rt.Method,
				Path:     rt.PathPattern,
				Examples: names,
			})
		}

		resp := UnmatchedResponse{
			Error:           "Not Found",
			Method:          r.Method,
			Path:            reqPath,
			Message:         fmt.Sprintf("No saved mock example found for %s %s", r.Method, reqPath),
			AvailableRoutes: available,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(resp)

		headersMap := make(map[string]string)
		for k, v := range r.Header {
			headersMap[k] = strings.Join(v, ", ")
		}

		s.recordLog(RequestLog{
			ID:           fmt.Sprintf("log_%d", time.Now().UnixNano()),
			Timestamp:    start,
			Method:       r.Method,
			Path:         reqPath,
			Query:        r.URL.RawQuery,
			Headers:      headersMap,
			StatusCode:   http.StatusNotFound,
			DurationMs:   duration.Milliseconds(),
			Matched:      false,
			ErrorMessage: "No route matched request method and path",
		})
		return
	}

	// 2. Resolve Overrides (Route-specific > Global)
	statusCode := 0
	delay := time.Duration(0)
	errorRate := globalErrRate

	if matchedRoute.Override != nil {
		if matchedRoute.Override.StatusCode > 0 {
			statusCode = matchedRoute.Override.StatusCode
		}
		if matchedRoute.Override.Delay > 0 {
			delay = matchedRoute.Override.Delay
		} else if matchedRoute.Override.DelayMs > 0 {
			delay = time.Duration(matchedRoute.Override.DelayMs) * time.Millisecond
		}
		if matchedRoute.Override.ErrorRate > 0 {
			errorRate = matchedRoute.Override.ErrorRate
		}
	}

	if statusCode == 0 && globalStatus > 0 {
		statusCode = globalStatus
	}
	if delay == 0 && globalDelayMs > 0 {
		delay = time.Duration(globalDelayMs) * time.Millisecond
	}

	// 3. Error-Injection Simulation
	if errorRate > 0 && rand.Float64() < errorRate {
		if delay > 0 {
			time.Sleep(delay)
		}
		duration := time.Since(start)
		errStatus := http.StatusInternalServerError
		if statusCode > 0 {
			errStatus = statusCode
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(errStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "Simulated mock failure via error-injection",
			"status":    errStatus,
			"errorRate": errorRate,
		})

		s.recordLog(RequestLog{
			ID:             fmt.Sprintf("log_%d", time.Now().UnixNano()),
			Timestamp:      start,
			Method:         r.Method,
			Path:           reqPath,
			Query:          r.URL.RawQuery,
			StatusCode:     errStatus,
			DurationMs:     duration.Milliseconds(),
			Matched:        true,
			MatchedRoute:   matchedRoute.ID,
			MatchedExample: "error-injection",
			ErrorMessage:   fmt.Sprintf("Simulated error injection triggered (rate: %.2f)", errorRate),
		})
		return
	}

	// 4. Delay Simulation
	if delay > 0 {
		time.Sleep(delay)
	}

	// 5. Example Selection
	selectedExample := PickExample(matchedRoute.Examples, r)
	if selectedExample == nil {
		// Fallback empty example if slice was corrupted
		selectedExample = &types.ExampleResponse{
			StatusCode: 200,
			Body:       "{}",
		}
	}

	finalStatus := selectedExample.StatusCode
	if finalStatus <= 0 {
		finalStatus = http.StatusOK
	}
	if statusCode > 0 {
		finalStatus = statusCode
	}

	// 6. Write Response Headers
	for _, h := range selectedExample.Headers {
		if h.Key != "" && !strings.EqualFold(h.Key, "Content-Length") {
			w.Header().Set(h.Key, h.Value)
		}
	}

	if selectedExample.ContentType != "" {
		w.Header().Set("Content-Type", selectedExample.ContentType)
	} else if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(finalStatus)
	_, _ = w.Write([]byte(selectedExample.Body))

	duration := time.Since(start)

	headersMap := make(map[string]string)
	for k, v := range r.Header {
		headersMap[k] = strings.Join(v, ", ")
	}

	s.recordLog(RequestLog{
		ID:             fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:      start,
		Method:         r.Method,
		Path:           reqPath,
		Query:          r.URL.RawQuery,
		Headers:        headersMap,
		StatusCode:     finalStatus,
		DurationMs:     duration.Milliseconds(),
		Matched:        true,
		MatchedRoute:   matchedRoute.ID,
		MatchedExample: selectedExample.Name,
	})
}
