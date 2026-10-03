package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestInheritanceResolver_MergeHeaders(t *testing.T) {
	resolver := NewInheritanceResolver(nil)

	tests := []struct {
		name                 string
		chain                []FolderChainItem
		reqHeaders           []types.KeyValue
		expectedCount        int
		expectedLookup       map[string]string // lowercase key -> expected value
		expectedProvenance   map[string]string // lowercase key -> expected source folder
		expectedNonInherited []string          // lowercase keys that should NOT have provenance
	}{
		{
			name: "root to parent to child to request override",
			chain: []FolderChainItem{
				{
					DirName: "root-coll",
					RelPath: "collections",
					FolderDef: &types.FolderDefinition{
						Headers: []types.KeyValue{
							{Key: "X-Root-Only", Value: "root-val", Enabled: true},
							{Key: "X-Common", Value: "from-root", Enabled: true},
						},
					},
				},
				{
					DirName: "01-parent",
					RelPath: "collections/01-parent",
					FolderDef: &types.FolderDefinition{
						Headers: []types.KeyValue{
							{Key: "X-Common", Value: "from-parent", Enabled: true},
							{Key: "X-Parent-Only", Value: "parent-val", Enabled: true},
						},
					},
				},
				{
					DirName: "02-child",
					RelPath: "collections/01-parent/02-child",
					FolderDef: &types.FolderDefinition{
						Headers: []types.KeyValue{
							{Key: "X-Child-Only", Value: "child-val", Enabled: true},
						},
					},
				},
			},
			reqHeaders: []types.KeyValue{
				{Key: "X-Common", Value: "from-request", Enabled: true},
				{Key: "X-Req-Only", Value: "req-val", Enabled: true},
			},
			expectedCount: 5,
			expectedLookup: map[string]string{
				"x-root-only":   "root-val",
				"x-parent-only": "parent-val",
				"x-child-only":  "child-val",
				"x-req-only":    "req-val",
				"x-common":      "from-request",
			},
			expectedProvenance: map[string]string{
				"x-root-only":   "root-coll",
				"x-parent-only": "01-parent",
				"x-child-only":  "02-child",
			},
			expectedNonInherited: []string{"x-common", "x-req-only"},
		},
		{
			name: "case-insensitive conflict override",
			chain: []FolderChainItem{
				{
					DirName: "01-api",
					RelPath: "collections/01-api",
					FolderDef: &types.FolderDefinition{
						Headers: []types.KeyValue{
							{Key: "Accept", Value: "application/json", Enabled: true},
							{Key: "X-CUSTOM-TRACE", Value: "trace-1", Enabled: true},
						},
					},
				},
				{
					DirName: "02-v1",
					RelPath: "collections/01-api/02-v1",
					FolderDef: &types.FolderDefinition{
						Headers: []types.KeyValue{
							{Key: "accept", Value: "text/plain", Enabled: true},
						},
					},
				},
			},
			reqHeaders: []types.KeyValue{
				{Key: "x-custom-trace", Value: "trace-override", Enabled: false},
			},
			expectedCount: 2,
			expectedLookup: map[string]string{
				"accept":         "text/plain",
				"x-custom-trace": "trace-override",
			},
			expectedProvenance: map[string]string{
				"accept": "02-v1",
			},
			expectedNonInherited: []string{"x-custom-trace"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged, provenance := resolver.MergeHeaders(tc.chain, tc.reqHeaders)

			if len(merged) != tc.expectedCount {
				t.Fatalf("expected %d headers, got %d", tc.expectedCount, len(merged))
			}

			mergedMap := make(map[string]types.KeyValue)
			for _, h := range merged {
				mergedMap[strings.ToLower(h.Key)] = h
			}

			for lowerKey, expectedVal := range tc.expectedLookup {
				h, exists := mergedMap[lowerKey]
				if !exists {
					t.Errorf("expected header %q not found in merged headers", lowerKey)
					continue
				}
				if h.Value != expectedVal {
					t.Errorf("header %q: expected value %q, got %q", lowerKey, expectedVal, h.Value)
				}
			}

			for lowerKey, expectedSource := range tc.expectedProvenance {
				p, exists := provenance[lowerKey]
				if !exists {
					t.Errorf("expected provenance for %q not found", lowerKey)
					continue
				}
				if p.SourceFolder != expectedSource {
					t.Errorf("header %q provenance: expected source folder %q, got %q", lowerKey, expectedSource, p.SourceFolder)
				}
			}

			for _, lowerKey := range tc.expectedNonInherited {
				if _, exists := provenance[lowerKey]; exists {
					t.Errorf("header %q should not be marked as inherited in provenance map", lowerKey)
				}
			}
		})
	}
}

func TestInheritanceResolver_ResolveAuth(t *testing.T) {
	resolver := NewInheritanceResolver(nil)

	tests := []struct {
		name                 string
		chain                []FolderChainItem
		reqAuth              types.AuthDefinition
		expectedType         string
		expectedToken        string
		expectedSourceFolder string
	}{
		{
			name: "request inherit walks up to closest parent bearer",
			chain: []FolderChainItem{
				{
					DirName: "root",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "basic", Username: "admin"},
					},
				},
				{
					DirName: "01-auth",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "bearer", Token: "parent-token"},
					},
				},
				{
					DirName: "02-sub",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "inherit"},
					},
				},
			},
			reqAuth:              types.AuthDefinition{Type: "inherit"},
			expectedType:         "bearer",
			expectedToken:        "parent-token",
			expectedSourceFolder: "01-auth",
		},
		{
			name: "request overrides parent with specific auth",
			chain: []FolderChainItem{
				{
					DirName: "01-auth",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "bearer", Token: "parent-token"},
					},
				},
			},
			reqAuth:              types.AuthDefinition{Type: "basic", Username: "req-user"},
			expectedType:         "basic",
			expectedToken:        "",
			expectedSourceFolder: "",
		},
		{
			name: "request overrides parent with none",
			chain: []FolderChainItem{
				{
					DirName: "01-auth",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "bearer", Token: "parent-token"},
					},
				},
			},
			reqAuth:              types.AuthDefinition{Type: "none"},
			expectedType:         "none",
			expectedToken:        "",
			expectedSourceFolder: "",
		},
		{
			name: "request inherit with no folder auth defaults to none",
			chain: []FolderChainItem{
				{
					DirName: "01-auth",
					FolderDef: &types.FolderDefinition{
						Auth: types.AuthDefinition{Type: "inherit"},
					},
				},
			},
			reqAuth:              types.AuthDefinition{Type: "inherit"},
			expectedType:         "none",
			expectedToken:        "",
			expectedSourceFolder: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auth, prov := resolver.ResolveAuth(tc.chain, tc.reqAuth)

			if auth.Type != tc.expectedType {
				t.Errorf("expected auth type %q, got %q", tc.expectedType, auth.Type)
			}
			if tc.expectedToken != "" && auth.Token != tc.expectedToken {
				t.Errorf("expected auth token %q, got %q", tc.expectedToken, auth.Token)
			}

			if tc.expectedSourceFolder != "" {
				if prov == nil {
					t.Fatalf("expected provenance for auth, got nil")
				}
				if prov.SourceFolder != tc.expectedSourceFolder {
					t.Errorf("expected auth source folder %q, got %q", tc.expectedSourceFolder, prov.SourceFolder)
				}
			} else if prov != nil {
				t.Errorf("expected nil auth provenance for direct override, got %+v", prov)
			}
		})
	}
}

func TestInheritanceResolver_ResolveScripts(t *testing.T) {
	resolver := NewInheritanceResolver(nil)

	chain := []FolderChainItem{
		{
			DirName: "root",
			RelPath: "collections",
			FolderDef: &types.FolderDefinition{
				Scripts: types.ScriptDefinition{
					PreRequest:   "console.log('root-pre');",
					PostResponse: "console.log('root-post');",
				},
			},
		},
		{
			DirName: "01-parent",
			RelPath: "collections/01-parent",
			FolderDef: &types.FolderDefinition{
				Scripts: types.ScriptDefinition{
					PreRequest:   "console.log('parent-pre');",
					PostResponse: "console.log('parent-post');",
				},
			},
		},
		{
			DirName: "02-child",
			RelPath: "collections/01-parent/02-child",
			FolderDef: &types.FolderDefinition{
				Scripts: types.ScriptDefinition{
					PreRequest:   "console.log('child-pre');",
					PostResponse: "console.log('child-post');",
				},
			},
		},
	}

	reqScripts := types.ScriptDefinition{
		PreRequest:   "console.log('req-pre');",
		PostResponse: "console.log('req-post');",
	}

	preScripts, postScripts := resolver.ResolveScripts(chain, reqScripts)

	// Pre-request scripts: Root → Parent → Child → Request
	expectedPreOrder := []string{"root", "01-parent", "02-child", "request"}
	if len(preScripts) != len(expectedPreOrder) {
		t.Fatalf("expected %d pre-request scripts, got %d", len(expectedPreOrder), len(preScripts))
	}
	for i, expectedSource := range expectedPreOrder {
		if preScripts[i].Source != expectedSource {
			t.Errorf("pre-request script %d: expected source %q, got %q", i, expectedSource, preScripts[i].Source)
		}
	}

	// Post-response scripts: Request → Child → Parent → Root
	expectedPostOrder := []string{"request", "02-child", "01-parent", "root"}
	if len(postScripts) != len(expectedPostOrder) {
		t.Fatalf("expected %d post-response scripts, got %d", len(expectedPostOrder), len(postScripts))
	}
	for i, expectedSource := range expectedPostOrder {
		if postScripts[i].Source != expectedSource {
			t.Errorf("post-response script %d: expected source %q, got %q", i, expectedSource, postScripts[i].Source)
		}
	}
}

func TestInheritanceResolver_MergeVariables(t *testing.T) {
	resolver := NewInheritanceResolver(nil)

	baseEnv := map[string]string{
		"HOST": "api.example.com",
		"ENV":  "staging",
	}

	chain := []FolderChainItem{
		{
			DirName: "root",
			FolderDef: &types.FolderDefinition{
				Variables: []types.KeyValue{
					{Key: "ENV", Value: "production", Enabled: true},
					{Key: "ROOT_VAR", Value: "root", Enabled: true},
				},
			},
		},
		{
			DirName: "01-child",
			FolderDef: &types.FolderDefinition{
				Variables: []types.KeyValue{
					{Key: "CHILD_VAR", Value: "child", Enabled: true},
					{Key: "DISABLED_VAR", Value: "ignored", Enabled: false},
				},
			},
		},
	}

	overrides := map[string]string{
		"ENV": "local-override",
	}

	merged, prov := resolver.MergeVariables(baseEnv, chain, overrides)

	if merged["HOST"] != "api.example.com" {
		t.Errorf("expected HOST=api.example.com, got %q", merged["HOST"])
	}
	if merged["ROOT_VAR"] != "root" {
		t.Errorf("expected ROOT_VAR=root, got %q", merged["ROOT_VAR"])
	}
	if merged["CHILD_VAR"] != "child" {
		t.Errorf("expected CHILD_VAR=child, got %q", merged["CHILD_VAR"])
	}
	if _, exists := merged["DISABLED_VAR"]; exists {
		t.Errorf("disabled variable should not be in merged variables")
	}
	if merged["ENV"] != "local-override" {
		t.Errorf("expected ENV=local-override, got %q", merged["ENV"])
	}

	// Provenance checks
	if p, ok := prov["CHILD_VAR"]; !ok || p.SourceFolder != "01-child" {
		t.Errorf("expected CHILD_VAR provenance from 01-child, got %+v", p)
	}
	if _, ok := prov["ENV"]; ok {
		t.Errorf("overridden ENV variable should not have folder provenance")
	}
}

func TestInheritanceResolver_DiscoverFolderChain(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workspace structure:
	// tmpDir/
	//   collections/
	//     _folder.pebble.json
	//     01-auth/
	//       _folder.pebble.json
	//       02-login/
	//         _folder.pebble.json
	//         login.pebble.json

	collDir := filepath.Join(tmpDir, "collections")
	authDir := filepath.Join(collDir, "01-auth")
	loginDir := filepath.Join(authDir, "02-login")

	_ = os.MkdirAll(loginDir, 0755)

	_ = os.WriteFile(filepath.Join(collDir, FolderFile), []byte(`{"schemaVersion":1,"name":"Root Collection"}`), 0644)
	_ = os.WriteFile(filepath.Join(authDir, FolderFile), []byte(`{"schemaVersion":1,"name":"Auth Module"}`), 0644)
	_ = os.WriteFile(filepath.Join(loginDir, FolderFile), []byte(`{"schemaVersion":1,"name":"Login Group"}`), 0644)

	reqFile := filepath.Join(loginDir, "login.pebble.json")
	_ = os.WriteFile(reqFile, []byte(`{"schemaVersion":1,"name":"Login"}`), 0644)

	resolver := NewInheritanceResolver(NewWorkspaceService())
	chain, err := resolver.DiscoverFolderChain(tmpDir, reqFile)
	if err != nil {
		t.Fatalf("DiscoverFolderChain failed: %v", err)
	}

	if len(chain) != 3 {
		t.Fatalf("expected chain length 3, got %d", len(chain))
	}

	if chain[0].DirName != "Root Collection" {
		t.Errorf("chain[0]: expected 'Root Collection', got %q", chain[0].DirName)
	}
	if chain[1].DirName != "Auth Module" {
		t.Errorf("chain[1]: expected 'Auth Module', got %q", chain[1].DirName)
	}
	if chain[2].DirName != "Login Group" {
		t.Errorf("chain[2]: expected 'Login Group', got %q", chain[2].DirName)
	}
}
