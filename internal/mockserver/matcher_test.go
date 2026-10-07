package mockserver

import (
	"net/http/httptest"
	"testing"

	"pebblepost/internal/types"
)

func TestExtractPathFromURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.example.com/users/:id", "/users/:id"},
		{"http://localhost:8080/api/v1/orders/{id}?limit=10#top", "/api/v1/orders/{id}"},
		{"{{BASE_URL}}/users/:id", "/users/:id"},
		{"{{HOST}}/items/{itemId}", "/items/{itemId}"},
		{"/users/:id", "/users/:id"},
		{"http://localhost:8080", "/"},
		{"api.domain.com/v1/health", "/v1/health"},
		{"", "/"},
		{"///multiple///slashes//here", "/multiple/slashes/here"},
	}

	for _, tc := range tests {
		actual := ExtractPathFromURL(tc.input)
		if actual != tc.expected {
			t.Errorf("ExtractPathFromURL(%q) = %q; want %q", tc.input, actual, tc.expected)
		}
	}
}

func TestParsePathSegmentsAndSpecificity(t *testing.T) {
	// Literal route should have strictly higher specificity than parameterized route
	_, literalSpec := ParsePathSegments("/users/profile")
	_, paramSpec := ParsePathSegments("/users/:id")
	_, bracketSpec := ParsePathSegments("/users/{id}")
	_, wildSpec := ParsePathSegments("/users/*")

	if literalSpec <= paramSpec {
		t.Fatalf("Expected literal specificity (%d) > param specificity (%d)", literalSpec, paramSpec)
	}
	if paramSpec != bracketSpec {
		t.Errorf("Expected colon and bracket param specificity to match: %d vs %d", paramSpec, bracketSpec)
	}
	if paramSpec <= wildSpec {
		t.Fatalf("Expected param specificity (%d) > wild specificity (%d)", paramSpec, wildSpec)
	}
}

func TestMatchRoute(t *testing.T) {
	// Route 1: GET /users/:id
	seg1, spec1 := ParsePathSegments("/users/:id")
	routeParam := &MockRoute{
		ID:          "GET:/users/:id",
		Method:      "GET",
		PathPattern: "/users/:id",
		Segments:    seg1,
		Specificity: spec1,
	}

	// Route 2: GET /users/profile
	seg2, spec2 := ParsePathSegments("/users/profile")
	routeLiteral := &MockRoute{
		ID:          "GET:/users/profile",
		Method:      "GET",
		PathPattern: "/users/profile",
		Segments:    seg2,
		Specificity: spec2,
	}

	// Route 3: POST /items/{itemId}
	seg3, spec3 := ParsePathSegments("/items/{itemId}")
	routeBracket := &MockRoute{
		ID:          "POST:/items/{itemId}",
		Method:      "POST",
		PathPattern: "/items/{itemId}",
		Segments:    seg3,
		Specificity: spec3,
	}

	// Test 1: Match literal
	matched, _ := MatchRoute(routeLiteral, "GET", "/users/profile")
	if !matched {
		t.Errorf("Expected routeLiteral to match GET /users/profile")
	}

	// Test 2: Match param and extract parameter
	matched, params := MatchRoute(routeParam, "GET", "/users/42")
	if !matched {
		t.Errorf("Expected routeParam to match GET /users/42")
	}
	if params["id"] != "42" {
		t.Errorf("Expected params['id'] = '42'; got %q", params["id"])
	}

	// Test 3: Match bracket param
	matched, params = MatchRoute(routeBracket, "POST", "/items/item_abc123")
	if !matched {
		t.Errorf("Expected routeBracket to match POST /items/item_abc123")
	}
	if params["itemId"] != "item_abc123" {
		t.Errorf("Expected params['itemId'] = 'item_abc123'; got %q", params["itemId"])
	}

	// Test 4: Method mismatch
	matched, _ = MatchRoute(routeBracket, "GET", "/items/item_abc123")
	if matched {
		t.Errorf("Expected routeBracket to NOT match GET /items/item_abc123")
	}

	// Test 5: Path mismatch
	matched, _ = MatchRoute(routeLiteral, "GET", "/organizations/profile")
	if matched {
		t.Errorf("Expected routeLiteral to NOT match GET /organizations/profile")
	}
}

func TestPickExample(t *testing.T) {
	examples := []types.ExampleResponse{
		{ID: "1", Name: "Default 200", StatusCode: 200, Body: `{"status":"ok"}`},
		{ID: "2", Name: "Error 404", StatusCode: 404, Body: `{"error":"not found"}`},
		{ID: "3", Name: "Unauthorized", StatusCode: 401, Body: `{"error":"unauthorized"}`},
	}

	// 1. Without header -> default to first
	req1 := httptest.NewRequest("GET", "/test", nil)
	ex := PickExample(examples, req1)
	if ex.Name != "Default 200" {
		t.Errorf("Expected default example 'Default 200'; got %q", ex.Name)
	}

	// 2. With X-Mock-Example header
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Mock-Example", "Error 404")
	ex = PickExample(examples, req2)
	if ex.Name != "Error 404" {
		t.Errorf("Expected 'Error 404'; got %q", ex.Name)
	}

	// 3. With Prefer: example="Unauthorized"
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("Prefer", `example="Unauthorized"`)
	ex = PickExample(examples, req3)
	if ex.Name != "Unauthorized" {
		t.Errorf("Expected 'Unauthorized'; got %q", ex.Name)
	}

	// 4. Non-matching header -> fallback to first
	req4 := httptest.NewRequest("GET", "/test", nil)
	req4.Header.Set("X-Mock-Example", "NonExistent")
	ex = PickExample(examples, req4)
	if ex.Name != "Default 200" {
		t.Errorf("Expected fallback to 'Default 200'; got %q", ex.Name)
	}
}
