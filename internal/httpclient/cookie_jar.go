package httpclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pebblepost/internal/types"
)

// PersistentJar implements http.CookieJar with thread-safe persistence to .pebble/cookies.json.
type PersistentJar struct {
	mu            sync.RWMutex
	workspacePath string
	// domain -> path -> name -> CookieItem
	cookies map[string]map[string]map[string]types.CookieItem
}

// NewPersistentJar initializes a new cookie jar for the specified workspace.
func NewPersistentJar(workspacePath string) *PersistentJar {
	jar := &PersistentJar{
		workspacePath: workspacePath,
		cookies:       make(map[string]map[string]map[string]types.CookieItem),
	}
	if workspacePath != "" {
		_ = jar.Load()
	}
	return jar
}

// SetWorkspace updates the workspace root directory and reloads cookies from disk.
func (j *PersistentJar) SetWorkspace(workspacePath string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.workspacePath = workspacePath
	j.cookies = make(map[string]map[string]map[string]types.CookieItem)
	if workspacePath != "" {
		j.loadLocked()
	}
}

// SetCookies handles the Set-Cookie headers from an HTTP response (implements http.CookieJar).
func (j *PersistentJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u == nil || len(cookies) == 0 {
		return
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now().UTC()
	host := strings.ToLower(u.Hostname())

	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}

		domain := strings.ToLower(c.Domain)
		if domain == "" {
			domain = host
		} else {
			domain = strings.TrimPrefix(domain, ".")
		}

		path := c.Path
		if path == "" {
			path = "/"
		}

		// Check for expired cookies (MaxAge < 0 or Expires in the past)
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(now)) {
			j.deleteLocked(domain, path, c.Name)
			continue
		}

		sameSiteStr := ""
		switch c.SameSite {
		case http.SameSiteLaxMode:
			sameSiteStr = "Lax"
		case http.SameSiteStrictMode:
			sameSiteStr = "Strict"
		case http.SameSiteNoneMode:
			sameSiteStr = "None"
		}

		item := types.CookieItem{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   domain,
			Path:     path,
			Expires:  c.Expires,
			MaxAge:   c.MaxAge,
			Secure:   c.Secure,
			HTTPOnly: c.HttpOnly,
			SameSite: sameSiteStr,
		}

		j.setLocked(item)
	}

	if j.workspacePath != "" {
		_ = j.saveLocked()
	}
}

// Cookies returns the cookies to send in an HTTP request to u (implements http.CookieJar).
func (j *PersistentJar) Cookies(u *url.URL) []*http.Cookie {
	if u == nil {
		return nil
	}

	j.mu.RLock()
	defer j.mu.RUnlock()

	now := time.Now().UTC()
	host := strings.ToLower(u.Hostname())
	path := u.Path
	if path == "" {
		path = "/"
	}
	isSecure := strings.EqualFold(u.Scheme, "https")

	var matched []*http.Cookie

	for domain, paths := range j.cookies {
		if !domainMatches(host, domain) {
			continue
		}

		for cPath, names := range paths {
			if !pathMatches(path, cPath) {
				continue
			}

			for _, item := range names {
				// Expired cookie check
				if !item.Expires.IsZero() && item.Expires.Before(now) {
					continue
				}
				if item.Secure && !isSecure {
					continue
				}

				matched = append(matched, &http.Cookie{
					Name:     item.Name,
					Value:    item.Value,
					Path:     item.Path,
					Domain:   item.Domain,
					Expires:  item.Expires,
					MaxAge:   item.MaxAge,
					Secure:   item.Secure,
					HttpOnly: item.HTTPOnly,
				})
			}
		}
	}

	return matched
}

// GetAllCookies returns all active cookies in the jar.
func (j *PersistentJar) GetAllCookies() []types.CookieItem {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var result []types.CookieItem
	now := time.Now().UTC()

	for _, paths := range j.cookies {
		for _, names := range paths {
			for _, item := range names {
				if !item.Expires.IsZero() && item.Expires.Before(now) {
					continue
				}
				result = append(result, item)
			}
		}
	}
	return result
}

// GetCookiesByDomain returns all active cookies grouped by domain.
func (j *PersistentJar) GetCookiesByDomain() map[string][]types.CookieItem {
	all := j.GetAllCookies()
	grouped := make(map[string][]types.CookieItem)
	for _, c := range all {
		grouped[c.Domain] = append(grouped[c.Domain], c)
	}
	return grouped
}

// SetCookie adds or updates a cookie in the jar and persists it.
func (j *PersistentJar) SetCookie(item types.CookieItem) error {
	if item.Name == "" || item.Domain == "" {
		return fmt.Errorf("cookie name and domain are required")
	}
	if item.Path == "" {
		item.Path = "/"
	}
	item.Domain = strings.ToLower(strings.TrimPrefix(item.Domain, "."))

	j.mu.Lock()
	defer j.mu.Unlock()

	j.setLocked(item)

	if j.workspacePath != "" {
		return j.saveLocked()
	}
	return nil
}

// DeleteCookie removes a specific cookie identified by domain, path, and name.
func (j *PersistentJar) DeleteCookie(domain, path, name string) error {
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	if path == "" {
		path = "/"
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	j.deleteLocked(domain, path, name)

	if j.workspacePath != "" {
		return j.saveLocked()
	}
	return nil
}

// ClearDomain removes all cookies belonging to the specified domain.
func (j *PersistentJar) ClearDomain(domain string) error {
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))

	j.mu.Lock()
	defer j.mu.Unlock()

	delete(j.cookies, domain)

	if j.workspacePath != "" {
		return j.saveLocked()
	}
	return nil
}

// ClearAll removes all cookies from the jar and clears the persistent file.
func (j *PersistentJar) ClearAll() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.cookies = make(map[string]map[string]map[string]types.CookieItem)

	if j.workspacePath != "" {
		return j.saveLocked()
	}
	return nil
}

// Load reads cookies from .pebble/cookies.json.
func (j *PersistentJar) Load() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.loadLocked()
}

func (j *PersistentJar) loadLocked() error {
	if j.workspacePath == "" {
		return nil
	}
	filePath := filepath.Join(j.workspacePath, ".pebble", "cookies.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var items []types.CookieItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	j.cookies = make(map[string]map[string]map[string]types.CookieItem)
	now := time.Now().UTC()
	for _, item := range items {
		if !item.Expires.IsZero() && item.Expires.Before(now) {
			continue
		}
		j.setLocked(item)
	}

	return nil
}

func (j *PersistentJar) saveLocked() error {
	if j.workspacePath == "" {
		return nil
	}
	pebbleDir := filepath.Join(j.workspacePath, ".pebble")
	if err := os.MkdirAll(pebbleDir, 0700); err != nil {
		return err
	}
	_ = os.Chmod(pebbleDir, 0700)

	var items []types.CookieItem
	now := time.Now().UTC()
	for _, paths := range j.cookies {
		for _, names := range paths {
			for _, item := range names {
				if !item.Expires.IsZero() && item.Expires.Before(now) {
					continue
				}
				items = append(items, item)
			}
		}
	}

	filePath := filepath.Join(pebbleDir, "cookies.json")
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return err
	}
	_ = os.Chmod(filePath, 0600)
	return nil
}

func (j *PersistentJar) setLocked(item types.CookieItem) {
	d := item.Domain
	p := item.Path
	if _, ok := j.cookies[d]; !ok {
		j.cookies[d] = make(map[string]map[string]types.CookieItem)
	}
	if _, ok := j.cookies[d][p]; !ok {
		j.cookies[d][p] = make(map[string]types.CookieItem)
	}
	j.cookies[d][p][item.Name] = item
}

func (j *PersistentJar) deleteLocked(domain, path, name string) {
	if paths, ok := j.cookies[domain]; ok {
		if names, ok := paths[path]; ok {
			delete(names, name)
			if len(names) == 0 {
				delete(paths, path)
			}
		}
		if len(paths) == 0 {
			delete(j.cookies, domain)
		}
	}
}

func domainMatches(host, domain string) bool {
	if host == domain {
		return true
	}
	if strings.HasSuffix(host, "."+domain) {
		return true
	}
	return false
}

func pathMatches(reqPath, cookiePath string) bool {
	if cookiePath == "/" || reqPath == cookiePath {
		return true
	}
	if strings.HasPrefix(reqPath, cookiePath) {
		if strings.HasSuffix(cookiePath, "/") || len(reqPath) > len(cookiePath) && reqPath[len(cookiePath)] == '/' {
			return true
		}
	}
	return false
}
