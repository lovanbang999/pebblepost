# PebblePost MCP Server

PebblePost's MCP server lets a compatible AI client work with one local workspace through the Model Context Protocol over stdio.

## Build and run

From the PebblePost repository root:

```bash
cargo build --release --package pebblepost-mcp
./target/release/pebblepost-mcp --workspace /absolute/path/to/pebblepost-workspace
```

The MCP server reads JSON-RPC messages from stdin and writes protocol messages to stdout. Run it through your MCP client instead of in a regular terminal. Pass `--workspace` an existing workspace directory; if omitted, the current working directory is used.

The project Makefile also provides:

```bash
make build-mcp
make run-mcp
```

## Client configuration

Use the absolute path to the built executable and workspace. For Claude Desktop, add this entry to its MCP server configuration:

```json
{
  "mcpServers": {
    "pebblepost": {
      "command": "/absolute/path/to/pebblepost/target/release/pebblepost-mcp",
      "args": ["--workspace", "/absolute/path/to/pebblepost-workspace"]
    }
  }
}
```

For Cursor, put the same server entry in `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "pebblepost": {
      "command": "/absolute/path/to/pebblepost/target/release/pebblepost-mcp",
      "args": ["--workspace", "/absolute/path/to/pebblepost-workspace"]
    }
  }
}
```

Restart or reload the MCP client after changing its configuration. The binary can also be run from a development checkout by configuring `cargo` as the command and passing `run --quiet --manifest-path /absolute/path/to/pebblepost/Cargo.toml --package pebblepost-mcp -- --workspace /absolute/path/to/pebblepost-workspace` as its arguments.

## Available tools

| Tool | Purpose |
| --- | --- |
| `list_collections` | List collection folders and request counts. |
| `list_requests` | List request paths, methods, and names, optionally within one folder. |
| `get_request` | Read a request with sensitive values redacted. |
| `create_or_update_request` | Create or update a `*.pebble.json` request inside an existing collection folder. |
| `inspect_environment` | List environment names and variable metadata without values. |
| `send_request` | Execute a saved request with PebblePost's request engine. |
| `run_collection` | Run all or selected requests in a folder and return the runner summary. |
| `generate_assertions` | Save post-response status and JSON-field assertions based on a preceding `send_request`. |
| `sync_openapi` | Diff, link, or apply OpenAPI 3 changes. Apply requires explicit endpoint IDs from a diff. |
| `start_mock_server` | Start the example-response mock server on loopback. |
| `add_mock_override` | Override a route in the running mock server. |
| `get_mock_logs` | Read mock request logs. |
| `stop_mock_server` | Stop the mock server started by this process. |

Request and collection paths are workspace-relative and restricted to `collections/`. Local OpenAPI spec files must be inside the collection folder selected for sync. The server is bound to one canonical workspace at startup; a tool call cannot select another workspace.

## Security behavior

- Environment tools return variable names and flags, never variable values.
- Request and execution results redact common credential fields, including tokens, passwords, authorization headers, cookies, and secret key fields.
- Request scripts run according to the workspace's existing trust setting. MCP tools do not grant additional trust.
- OpenAPI apply requires a non-empty list of endpoint IDs and defaults to preserving conflict edits (`force: false`).
- The mock server binds to `127.0.0.1`; it is not exposed to the local network.
- The server does not write logs to stdout because stdout carries MCP protocol messages.
