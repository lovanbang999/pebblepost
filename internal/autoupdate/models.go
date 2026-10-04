package autoupdate

import "time"

// UpdateInfo holds details about a newly available PebblePost release.
type UpdateInfo struct {
	CurrentVersion string    `json:"currentVersion"`
	LatestVersion  string    `json:"latestVersion"`
	HasUpdate      bool      `json:"hasUpdate"`
	ReleaseNotes   string    `json:"releaseNotes"`
	ReleaseURL     string    `json:"releaseUrl"`
	PublishedAt    time.Time `json:"publishedAt"`
	AssetURL       string    `json:"assetUrl,omitempty"`
	SignatureURL   string    `json:"signatureUrl,omitempty"`
}

// GitHubRelease represents GitHub Releases API response structure.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a binary or signature release asset.
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// UpdateSettings represents user preferences for update checking.
type UpdateSettings struct {
	CheckOnStartup bool   `json:"checkOnStartup"`
	Channel        string `json:"channel"` // "stable" or "beta"
}
