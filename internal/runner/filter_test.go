package runner

import (
	"testing"

	"pebblepost/internal/types"
)

func makeRequest(name, relPath string, tags []string) (*types.RequestDefinition, string) {
	req := &types.RequestDefinition{
		Name:   name,
		Method: "GET",
		URL:    "http://localhost",
		Tags:   tags,
	}
	return req, relPath
}

func TestFilterRequest_NoFilter(t *testing.T) {
	req, relPath := makeRequest("Login", "auth/login.pebble.json", nil)
	if !FilterRequest(relPath, req, FilterOptions{}) {
		t.Error("empty filter should match everything")
	}
}

func TestFilterRequest_TagMatch(t *testing.T) {
	req, relPath := makeRequest("Login", "auth/login.pebble.json", []string{"smoke", "auth"})

	if !FilterRequest(relPath, req, FilterOptions{Tag: "smoke"}) {
		t.Error("should match tag 'smoke'")
	}
	if !FilterRequest(relPath, req, FilterOptions{Tag: "auth"}) {
		t.Error("should match tag 'auth'")
	}
	if FilterRequest(relPath, req, FilterOptions{Tag: "regression"}) {
		t.Error("should NOT match tag 'regression'")
	}
}

func TestFilterRequest_TagCaseInsensitive(t *testing.T) {
	req, relPath := makeRequest("Login", "auth/login.pebble.json", []string{"Smoke"})
	if !FilterRequest(relPath, req, FilterOptions{Tag: "smoke"}) {
		t.Error("tag matching should be case insensitive")
	}
}

func TestFilterRequest_NoTags_TagFilter(t *testing.T) {
	req, relPath := makeRequest("Login", "auth/login.pebble.json", nil)
	if FilterRequest(relPath, req, FilterOptions{Tag: "smoke"}) {
		t.Error("request with no tags should not match a tag filter")
	}
}

func TestFilterRequest_RequestNameSubstring(t *testing.T) {
	req, relPath := makeRequest("Login Request", "auth/login.pebble.json", nil)

	if !FilterRequest(relPath, req, FilterOptions{Request: "*login*"}) {
		t.Error("glob *login* should match 'login.pebble.json'")
	}
	if !FilterRequest(relPath, req, FilterOptions{Request: "login"}) {
		t.Error("substring 'login' should match relPath containing 'login'")
	}
	if FilterRequest(relPath, req, FilterOptions{Request: "profile"}) {
		t.Error("'profile' should not match 'login'")
	}
}

func TestFilterRequest_AND_Semantics(t *testing.T) {
	req, relPath := makeRequest("Login", "auth/login.pebble.json", []string{"smoke"})

	// Both tag AND request must match
	if !FilterRequest(relPath, req, FilterOptions{Tag: "smoke", Request: "login"}) {
		t.Error("should match when both tag and request filter match")
	}
	if FilterRequest(relPath, req, FilterOptions{Tag: "smoke", Request: "profile"}) {
		t.Error("should NOT match when tag matches but request filter does not")
	}
	if FilterRequest(relPath, req, FilterOptions{Tag: "regression", Request: "login"}) {
		t.Error("should NOT match when request filter matches but tag does not")
	}
}

func TestFilterRequest_FolderGlob(t *testing.T) {
	req, relPath := makeRequest("Login", "collections/auth/login.pebble.json", nil)

	if !FilterRequest(relPath, req, FilterOptions{Folder: "auth"}) {
		t.Error("'auth' should match folder part 'auth'")
	}
	if FilterRequest(relPath, req, FilterOptions{Folder: "payments"}) {
		t.Error("'payments' should NOT match folder 'auth'")
	}
}
