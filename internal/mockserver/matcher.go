package mockserver

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"pebblepost/internal/types"
)

var (
	// Regex matching {{...}} variable prefixes
	variablePrefixRegex = regexp.MustCompile(`^\{\{[^}]+\}\}`)
	// Regex for prefer header: example="name" or example=name
	preferExampleRegex = regexp.MustCompile(`(?:^|[;,])\s*example=(?:"([^"]+)"|([^\s;,]+))`)
)

// ExtractPathFromURL extracts the pathname from a URL or variable template string.
// Examples:
//
//	"https://api.example.com/users/:id" -> "/users/:id"
//	"{{BASE_URL}}/api/v1/orders/{id}?limit=10" -> "/api/v1/orders/{id}"
//	"/users/:id" -> "/users/:id"
//	"http://localhost:8080" -> "/"
func ExtractPathFromURL(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return "/"
	}

	// Strip query parameters and hash fragments
	if idx := strings.IndexAny(raw, "?#"); idx != -1 {
		raw = raw[:idx]
	}

	// If it starts with a template variable like {{HOST}} or {{BASE_URL}}
	if variablePrefixRegex.MatchString(raw) {
		raw = variablePrefixRegex.ReplaceAllString(raw, "")
		if raw == "" {
			return "/"
		}
		if !strings.HasPrefix(raw, "/") {
			raw = "/" + raw
		}
		return cleanPath(raw)
	}

	// If it has a scheme (http://, https://, etc.)
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err == nil {
			p := u.Path
			if p == "" {
				p = "/"
			}
			return cleanPath(p)
		}
		// Fallback: strip scheme and authority manually
		idx := strings.Index(raw, "://")
		afterScheme := raw[idx+3:]
		slashIdx := strings.Index(afterScheme, "/")
		if slashIdx == -1 {
			return "/"
		}
		return cleanPath(afterScheme[slashIdx:])
	}

	// If it contains a host-like prefix without scheme (e.g. localhost:8080/foo or api.domain.com/bar)
	if !strings.HasPrefix(raw, "/") {
		slashIdx := strings.Index(raw, "/")
		if slashIdx != -1 && (strings.Contains(raw[:slashIdx], ".") || strings.Contains(raw[:slashIdx], ":")) {
			return cleanPath(raw[slashIdx:])
		}
		return cleanPath("/" + raw)
	}

	return cleanPath(raw)
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	// Collapse multiple slashes
	parts := strings.Split(p, "/")
	var nonEmpties []string
	for _, part := range parts {
		if part != "" {
			nonEmpties = append(nonEmpties, part)
		}
	}
	if len(nonEmpties) == 0 {
		return "/"
	}
	return "/" + strings.Join(nonEmpties, "/")
}

// ParsePathSegments parses a route path pattern into structured segments and computes specificity.
func ParsePathSegments(pathPattern string) ([]PathSegment, int) {
	cleaned := cleanPath(pathPattern)
	if cleaned == "/" {
		return []PathSegment{}, 1000
	}

	parts := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")
	segments := make([]PathSegment, 0, len(parts))
	literalCount := 0
	paramCount := 0
	wildCount := 0

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, ":") && len(trimmed) > 1 {
			// e.g. ":id"
			paramName := trimmed[1:]
			segments = append(segments, PathSegment{
				Raw:     trimmed,
				IsParam: true,
				Param:   paramName,
			})
			paramCount++
		} else if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") && len(trimmed) > 2 {
			// e.g. "{id}"
			paramName := trimmed[1 : len(trimmed)-1]
			segments = append(segments, PathSegment{
				Raw:     trimmed,
				IsParam: true,
				Param:   paramName,
			})
			paramCount++
		} else if trimmed == "*" {
			segments = append(segments, PathSegment{
				Raw:    trimmed,
				IsWild: true,
			})
			wildCount++
		} else {
			segments = append(segments, PathSegment{
				Raw: trimmed,
			})
			literalCount++
		}
	}

	// Specificity score: higher score = higher precedence in routing
	// Literals are valued far above parameters, which are valued above wildcards
	specificity := (literalCount * 100) - (paramCount * 10) - (wildCount * 25) + len(segments)

	return segments, specificity
}

// MatchRoute checks if an incoming HTTP request matches a MockRoute.
// Returns true and extracted path parameters if matched.
func MatchRoute(route *MockRoute, method string, reqPath string) (bool, map[string]string) {
	if !strings.EqualFold(route.Method, method) {
		return false, nil
	}

	reqCleaned := cleanPath(reqPath)
	if len(route.Segments) == 0 {
		return reqCleaned == "/", make(map[string]string)
	}

	reqParts := strings.Split(strings.TrimPrefix(reqCleaned, "/"), "/")
	if len(reqParts) == 1 && reqParts[0] == "" {
		reqParts = []string{}
	}

	// If no wildcards, segment lengths must match exactly
	hasWildcard := false
	for _, seg := range route.Segments {
		if seg.IsWild {
			hasWildcard = true
			break
		}
	}

	if !hasWildcard && len(reqParts) != len(route.Segments) {
		return false, nil
	}

	paramMap := make(map[string]string)

	for i, seg := range route.Segments {
		if seg.IsWild {
			// Wildcard matches the rest of the path
			return true, paramMap
		}
		if i >= len(reqParts) {
			return false, nil
		}

		reqSeg := reqParts[i]
		if seg.IsParam {
			paramMap[seg.Param] = reqSeg
		} else {
			// Literal comparison (case-insensitive for developer ergonomics)
			if !strings.EqualFold(seg.Raw, reqSeg) {
				return false, nil
			}
		}
	}

	return len(reqParts) == len(route.Segments), paramMap
}

// PickExample selects an ExampleResponse from a list based on request headers,
// defaulting to the first example if not matched.
func PickExample(examples []types.ExampleResponse, r *http.Request) *types.ExampleResponse {
	if len(examples) == 0 {
		return nil
	}
	if len(examples) == 1 {
		return &examples[0]
	}

	// Check 1: X-Mock-Example header
	requestedName := strings.TrimSpace(r.Header.Get("X-Mock-Example"))
	if requestedName == "" {
		// Check 2: Prefer: example="name" or example=name
		preferHeader := r.Header.Get("Prefer")
		if preferHeader != "" {
			if matches := preferExampleRegex.FindStringSubmatch(preferHeader); len(matches) > 0 {
				if matches[1] != "" {
					requestedName = matches[1]
				} else if matches[2] != "" {
					requestedName = matches[2]
				}
			}
		}
	}

	if requestedName != "" {
		for i := range examples {
			if strings.EqualFold(examples[i].Name, requestedName) {
				return &examples[i]
			}
		}
	}

	// Default to first example
	return &examples[0]
}
