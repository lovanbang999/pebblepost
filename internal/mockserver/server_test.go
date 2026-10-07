package mockserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

func createTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	wsSvc := workspace.NewWorkspaceService()

	// 1. Literal route: GET /api/v1/users/profile
	req1 := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Get Current Profile",
		Method:        "GET",
		URL:           "https://api.example.com/api/v1/users/profile",
		Examples: []types.ExampleResponse{
			{
				ID:          "ex_prof_1",
				Name:        "Success",
				StatusCode:  200,
				ContentType: "application/json",
				Body:        `{"user":"current_user","role":"admin"}`,
			},
		},
	}
	if err := wsSvc.SaveRequest(filepath.Join(dir, "profile.pebble.json"), req1); err != nil {
		t.Fatalf("failed to save profile request: %v", err)
	}

	// 2. Parameterized route: GET /api/v1/users/:id with multiple examples
	req2 := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Get User By ID",
		Method:        "GET",
		URL:           "/api/v1/users/:id",
		Examples: []types.ExampleResponse{
			{
				ID:          "ex_user_1",
				Name:        "Found User",
				StatusCode:  200,
				ContentType: "application/json",
				Body:        `{"id":"123","name":"Alice"}`,
			},
			{
				ID:          "ex_user_2",
				Name:        "User Not Found",
				StatusCode:  404,
				ContentType: "application/json",
				Body:        `{"error":"User does not exist"}`,
			},
			{
				ID:          "ex_user_3",
				Name:        "Unauthorized",
				StatusCode:  401,
				ContentType: "application/json",
				Body:        `{"error":"Token expired"}`,
			},
		},
	}
	if err := wsSvc.SaveRequest(filepath.Join(dir, "get_user.pebble.json"), req2); err != nil {
		t.Fatalf("failed to save get_user request: %v", err)
	}

	// 3. Bracket param route: POST /api/v1/items/{itemId}
	req3 := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "Update Item",
		Method:        "POST",
		URL:           "{{BASE_URL}}/api/v1/items/{itemId}",
		Examples: []types.ExampleResponse{
			{
				ID:          "ex_item_1",
				Name:        "Item Updated",
				StatusCode:  200,
				ContentType: "application/json",
				Body:        `{"updated":true}`,
			},
		},
	}
	if err := wsSvc.SaveRequest(filepath.Join(dir, "update_item.pebble.json"), req3); err != nil {
		t.Fatalf("failed to save update_item request: %v", err)
	}

	return dir
}

func TestMockServer_Lifecycle(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()

	cfg := ServerConfig{
		Host:          "127.0.0.1",
		Port:          0, // OS assigns port
		WorkspacePath: wsDir,
		TargetPath:    wsDir,
	}

	srv := NewServer(cfg, wsSvc)
	if err := srv.LoadRoutes(wsDir); err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}

	if srv.IsRunning() {
		t.Fatal("Expected server to not be running before Start()")
	}

	if err := srv.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer srv.Stop()

	if !srv.IsRunning() {
		t.Fatal("Expected server to be running after Start()")
	}

	u := srv.URL()
	if !strings.HasPrefix(u, "http://127.0.0.1:") {
		t.Fatalf("Unexpected server URL: %s", u)
	}

	status := srv.Status()
	if !status.Running {
		t.Fatal("Expected Status().Running to be true")
	}
	if status.RoutesCount != 3 {
		t.Fatalf("Expected 3 routes loaded; got %d", status.RoutesCount)
	}

	// Verify route specificity order: profile (/api/v1/users/profile) must come before param (/api/v1/users/:id)
	var profileIndex, paramIndex = -1, -1
	for idx, rt := range status.Routes {
		if rt.PathPattern == "/api/v1/users/profile" {
			profileIndex = idx
		}
		if rt.PathPattern == "/api/v1/users/:id" {
			paramIndex = idx
		}
	}
	if profileIndex == -1 || paramIndex == -1 || profileIndex >= paramIndex {
		t.Fatalf("Expected literal route profile (idx %d) to precede param route (idx %d)", profileIndex, paramIndex)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	if srv.IsRunning() {
		t.Fatal("Expected server to be stopped")
	}
}

func TestMockServer_ServeHTTP_RoutingAndExamples(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()

	srv := NewServer(ServerConfig{
		Host: "127.0.0.1",
		Port: 0,
	}, wsSvc)

	if err := srv.LoadRoutes(wsDir); err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	client := &http.Client{Timeout: 3 * time.Second}
	baseURL := srv.URL()

	// 1. Literal path matches: GET /api/v1/users/profile
	resp, err := client.Get(baseURL + "/api/v1/users/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK; got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "current_user") {
		t.Fatalf("Expected profile body; got %s", string(body))
	}

	// 2. Parameterized path matches: GET /api/v1/users/42 -> default example
	resp, err = client.Get(baseURL + "/api/v1/users/42")
	if err != nil {
		t.Fatalf("GET /users/42 failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK; got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Alice") {
		t.Fatalf("Expected default example body; got %s", string(body))
	}

	// 3. Example selection via X-Mock-Example
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/users/42", nil)
	req.Header.Set("X-Mock-Example", "User Not Found")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET with X-Mock-Example failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found; got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "User does not exist") {
		t.Fatalf("Expected 404 example body; got %s", string(body))
	}

	// 4. Example selection via Prefer header
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/users/42", nil)
	req.Header.Set("Prefer", `example="Unauthorized"`)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET with Prefer failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized; got %d", resp.StatusCode)
	}

	// 5. Bracket parameter path: POST /api/v1/items/item_999
	resp, err = client.Post(baseURL+"/api/v1/items/item_999", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /items/item_999 failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK; got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"updated":true`) {
		t.Fatalf("Expected update body; got %s", string(body))
	}

	// 6. Unmatched Route: GET /unknown/endpoint -> 404 with helpful diagnostic payload
	resp, err = client.Get(baseURL + "/unknown/endpoint")
	if err != nil {
		t.Fatalf("GET /unknown/endpoint failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404; got %d", resp.StatusCode)
	}

	var unmatched UnmatchedResponse
	if err := json.Unmarshal(body, &unmatched); err != nil {
		t.Fatalf("Expected valid UnmatchedResponse JSON: %v", err)
	}
	if unmatched.Method != "GET" || unmatched.Path != "/unknown/endpoint" {
		t.Fatalf("Unexpected unmatched payload: %+v", unmatched)
	}
	if len(unmatched.AvailableRoutes) != 3 {
		t.Fatalf("Expected 3 available routes in diagnostic; got %d", len(unmatched.AvailableRoutes))
	}
}

func TestMockServer_OverridesAndChaos(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()

	srv := NewServer(ServerConfig{
		Host: "127.0.0.1",
		Port: 0,
	}, wsSvc)

	if err := srv.LoadRoutes(wsDir); err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := srv.URL()

	// 1. Status override: Force route GET:/api/v1/users/profile to return 418 I'm a Teapot
	srv.SetRouteOverride("GET:/api/v1/users/profile", &RouteOverride{
		StatusCode: http.StatusTeapot,
	})

	resp, err := client.Get(baseURL + "/api/v1/users/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("Expected 418 Teapot; got %d", resp.StatusCode)
	}

	// 2. Latency override: 50ms delay
	srv.SetRouteOverride("GET:/api/v1/users/profile", &RouteOverride{
		StatusCode: 200,
		DelayMs:    50,
	})

	start := time.Now()
	resp, err = client.Get(baseURL + "/api/v1/users/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("Expected simulated latency >= 40ms; got %v", elapsed)
	}

	// 3. Error-injection rate: 1.0 (100% failure)
	srv.SetRouteOverride("GET:/api/v1/users/profile", &RouteOverride{
		ErrorRate: 1.0,
	})

	resp, err = client.Get(baseURL + "/api/v1/users/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Expected 500 Internal Server Error; got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "error-injection") {
		t.Fatalf("Expected error injection payload; got %s", string(body))
	}
}

func TestMockServer_Concurrency(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()

	srv := NewServer(ServerConfig{
		Host: "127.0.0.1",
		Port: 0,
	}, wsSvc)

	if err := srv.LoadRoutes(wsDir); err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := srv.URL()

	var wg sync.WaitGroup
	errCh := make(chan error, 30)

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			url := fmt.Sprintf("%s/api/v1/users/%d", baseURL, id)
			resp, err := client.Get(url)
			if err != nil {
				errCh <- fmt.Errorf("request %d failed: %w", id, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("request %d returned status %d", id, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	logs := srv.Logs()
	if len(logs) < 30 {
		t.Fatalf("Expected at least 30 logs recorded; got %d", len(logs))
	}
}

func TestMockServer_LiveLogsSubscription(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()

	srv := NewServer(ServerConfig{
		Host: "127.0.0.1",
		Port: 0,
	}, wsSvc)

	if err := srv.LoadRoutes(wsDir); err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logCh := srv.SubscribeLogs(ctx)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL() + "/api/v1/users/profile")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	select {
	case entry := <-logCh:
		if entry.Path != "/api/v1/users/profile" {
			t.Fatalf("Unexpected log path: %s", entry.Path)
		}
		if entry.StatusCode != 200 {
			t.Fatalf("Unexpected log status: %d", entry.StatusCode)
		}
		if entry.MatchedExample != "Success" {
			t.Fatalf("Unexpected matched example: %s", entry.MatchedExample)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for live log event")
	}
}
