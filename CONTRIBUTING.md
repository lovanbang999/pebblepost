# Contributing to PebblePost

Thank you for your interest in contributing to PebblePost. PebblePost is an open-source, local-first API client built for developers who value privacy, speed, and Git-native collaboration.

---

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](./CODE_OF_CONDUCT.md). Please treat all contributors and community members with respect and professionalism.

---

## Getting Started & Development Setup

### Prerequisites

- **Go**: 1.22 or higher ([Download](https://go.dev/dl/))
- **Node.js**: v20 or higher & npm ([Download](https://nodejs.org/))
- **Wails v2 CLI**: For desktop application development
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```
- **Linux System Dependencies** (Debian / Ubuntu):
  ```bash
  sudo apt update
  sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
  ```

---

### Repository Setup

```bash
# 1. Fork and clone the repository
git clone https://github.com/lovanbang999/pebblepost.git
cd pebblepost

# 2. Install backend dependencies
go mod download

# 3. Install frontend dependencies
cd web
npm install
cd ..
```

---

### Running Locally

#### 1. Native Desktop Studio (Wails v2)

```bash
wails dev -skipbindings
```
*Note: This starts the Go backend and Vite frontend with live hot-reloading.*

#### 2. Standalone Web Server

```bash
# Terminal 1: Vite dev server (Optional for UI development)
cd web && npm run dev

# Terminal 2: Go HTTP Server
go run ./cmd/server
```

---

## Testing & Verification

Before submitting changes, ensure all verification checks pass:

```bash
# 1. Run all Go tests
go test -v ./...

# 2. Typecheck and build frontend
cd web
npm run build
cd ..

# 3. Verify single-binary Go build
go build ./cmd/server
```

---

## Branching & Commit Standards

We follow [Conventional Commits](https://www.conventionalcommits.org/):

### Branch Naming
- Features: `feat/<feature-name>`
- Bug fixes: `fix/<issue-description>`
- Refactors: `refactor/<target-area>`
- Documentation: `docs/<topic>`

### Commit Message Format
```
<type>(<scope>): <short imperative description>

- Detailed bullet point explaining rationale or UI changes
- Technical notes on packages or models modified
```

**Permitted Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`.  
**Allowed Scopes**: `ui`, `workspace`, `httpclient`, `scripting`, `importer`, `codegen`, `server`, `cli`, or omit the scope if changes span multiple subsystems.

---

## Submitting a Pull Request

1. **Fork the repository** and create a feature branch from `main`.
2. **Make focused, atomic changes**. Keep pull requests focused on a single concern.
3. **Follow the Zero-Mock Policy**: Connect components to real backend APIs; do not commit dummy mock arrays.
4. **Ensure clean builds and tests**: Both `go test ./...` and `npm run build` must succeed without warnings.
5. **Open a Pull Request** against `main` using our [PR Template](.github/pull_request_template.md).
6. **Address feedback**: Reviewers may request adjustments before merging.

---

## Areas to Contribute

- **Language Code Generators**: Extend [codegen.ts](web/src/lib/codegen.ts) to support Rust (`reqwest`), PHP (`Guzzle`), Java (`HttpClient`), or Swift.
- **Collection Importers**: Add Insomnia v4 export parsing or HAR file import in [importer.go](internal/workspace/importer.go).
- **CLI Runner**: Add JUnit, TAP, or HTML reporting formats to `cmd/cli`.
- **UI/UX Refinements**: Improve keyboard shortcuts, syntax themes, and accessibility in [web/src/components](web/src/components).
