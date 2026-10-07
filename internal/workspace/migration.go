package workspace

import "pebblepost/internal/types"

const (
	DefaultRequestSchema        = "https://pebblepost.dev/schemas/v2/request.json"
	DefaultRequestSchemaVersion = 2
	DefaultFolderSchemaVersion  = 2
	DefaultSchemaVersion        = 1
)

// MigrateRequest migrates a request in-memory to schemaVersion 2.
// If the request is already at schemaVersion >= 2, it returns wasMigrated=false.
// It never touches the disk.
func MigrateRequest(req *types.RequestDefinition) (*types.RequestDefinition, bool) {
	if req == nil {
		return nil, false
	}

	wasMigrated := false
	if req.SchemaVersion < DefaultRequestSchemaVersion {
		if req.SchemaVersion < 1 {
			if req.Auth.Type == "" {
				req.Auth.Type = "none"
			}
		} else if req.Auth.Type == "" {
			req.Auth.Type = "inherit"
		}
		if req.Extractors == nil {
			req.Extractors = make([]types.ExtractorDefinition, 0)
		}
		req.SchemaVersion = DefaultRequestSchemaVersion
		wasMigrated = true
	} else if req.Auth.Type == "" {
		req.Auth.Type = "inherit"
	}

	if req.Schema == "" || req.Schema == "https://pebblepost.dev/schemas/v1/request.json" {
		req.Schema = DefaultRequestSchema
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	return req, wasMigrated
}

// MigrateEnvironment migrates an environment in-memory to schemaVersion 1.
func MigrateEnvironment(env *types.EnvironmentDefinition) (*types.EnvironmentDefinition, bool) {
	if env == nil {
		return nil, false
	}

	wasMigrated := false
	if env.SchemaVersion < DefaultSchemaVersion {
		env.SchemaVersion = DefaultSchemaVersion
		wasMigrated = true
	}

	return env, wasMigrated
}

// MigrateWorkspace migrates workspace metadata in-memory to schemaVersion 1.
func MigrateWorkspace(ws *types.WorkspaceDefinition) (*types.WorkspaceDefinition, bool) {
	if ws == nil {
		return nil, false
	}

	wasMigrated := false
	if ws.SchemaVersion < DefaultSchemaVersion {
		ws.SchemaVersion = DefaultSchemaVersion
		wasMigrated = true
	}

	return ws, wasMigrated
}

// MigrateFolder migrates folder metadata in-memory to schemaVersion 2.
func MigrateFolder(f *types.FolderDefinition) (*types.FolderDefinition, bool) {
	if f == nil {
		return nil, false
	}

	wasMigrated := false
	if f.SchemaVersion < DefaultFolderSchemaVersion {
		f.SchemaVersion = DefaultFolderSchemaVersion
		wasMigrated = true
	}

	return f, wasMigrated
}
