# Running PebblePost in CI/CD

`pebblepost run` is designed to integrate cleanly into any CI/CD system that reads standard exit codes and test report formats.

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | All requests and assertions passed |
| `1`  | One or more assertion failures |
| `2`  | Configuration or parse error (bad flags, missing files) |
| `3`  | Network error or timeout (server unreachable) |

This allows pipelines to distinguish infrastructure failures from test failures, enabling conditional retries or separate alerting.

---

## Reporters

Use `--reporter` to control output format. The flag is **repeatable** — you can write to multiple targets in the same run:

```bash
# CLI output to stdout + JUnit XML to file (most common in CI)
pebblepost run ./collections -e ci \
  --reporter cli \
  --reporter junit:results.xml

# HTML report for artifact upload
pebblepost run ./collections -e ci \
  --reporter junit:results.xml \
  --reporter html:report.html

# JSON for downstream processing
pebblepost run ./collections -e ci --reporter json > results.json
```

Shorthand for a single file output:
```bash
pebblepost run ./collections --reporter junit --out results.xml
```

---

## Variable Injection

Variable precedence (highest → lowest):

1. `--var KEY=VALUE` (CLI flags)
2. `PEBBLE_VAR_*` environment variables
3. `--env-file <path>` (KEY=VALUE file)
4. Selected environment (`-e name`)

**Recommended CI approach:** inject secrets via environment variables so they never appear in scripts or logs:

```bash
export PEBBLE_VAR_API_KEY="$SECRET_API_KEY"
export PEBBLE_VAR_BASE_URL="https://staging.api.example.com"
pebblepost run ./collections -e staging --reporter junit:results.xml
```

Secret variable values are automatically **masked** in all reporter output.

---

## GitHub Actions

### Minimal Example

```yaml
name: API Tests

on:
  push:
    branches: [main, develop]
  pull_request:

jobs:
  api-tests:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Install PebblePost CLI
        run: |
          go install pebblepost/cmd/cli@latest
          # Or download from GitHub Releases:
          # curl -sSL https://github.com/your-org/pebblepost/releases/latest/download/pebblepost-linux-amd64 -o /usr/local/bin/pebblepost
          # chmod +x /usr/local/bin/pebblepost

      - name: Run API tests
        env:
          PEBBLE_VAR_BASE_URL: ${{ vars.STAGING_URL }}
          PEBBLE_VAR_API_KEY: ${{ secrets.API_KEY }}
        run: |
          pebblepost run ./collections -e staging \
            --reporter cli \
            --reporter junit:test-results.xml \
            --reporter html:test-report.html \
            --bail \
            --timeout 10000

      - name: Publish JUnit test results
        uses: mikepenz/action-junit-report@v4
        if: always() # run even if tests fail
        with:
          report_paths: test-results.xml

      - name: Upload HTML report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: api-test-report
          path: test-report.html
          retention-days: 14
```

### Advanced: Smoke Tags + Retry on Network Errors

```yaml
      - name: Run smoke tests with retry
        env:
          PEBBLE_VAR_BASE_URL: ${{ vars.PROD_URL }}
          PEBBLE_VAR_TOKEN: ${{ secrets.PROD_TOKEN }}
        run: |
          pebblepost run ./collections \
            --tag smoke \
            --retry 3 \
            --delay 500 \
            --timeout 15000 \
            --reporter junit:smoke-results.xml \
            --bail
```

### Dry Run (Preview without sending requests)

```yaml
      - name: Preview test order
        run: pebblepost run ./collections --dry-run
```

---

## GitLab CI

```yaml
api-tests:
  image: golang:1.25-alpine
  stage: test
  script:
    - go install pebblepost/cmd/cli@latest
    - pebblepost run ./collections -e staging
        --reporter junit:results.xml
        --reporter cli
        --bail
  variables:
    PEBBLE_VAR_BASE_URL: ${STAGING_URL}
    PEBBLE_VAR_TOKEN: ${STAGING_TOKEN}
  artifacts:
    when: always
    reports:
      junit: results.xml
    paths:
      - results.xml
    expire_in: 1 week
```

---

## Jenkins (JUnit plugin)

```groovy
pipeline {
    agent any
    environment {
        PEBBLE_VAR_BASE_URL = "${env.STAGING_URL}"
        PEBBLE_VAR_API_KEY  = credentials('api-key-secret')
    }
    stages {
        stage('API Tests') {
            steps {
                sh '''
                    pebblepost run ./collections \
                      -e staging \
                      --reporter junit:results.xml \
                      --reporter html:report.html \
                      --bail \
                      --timeout 10000
                '''
            }
            post {
                always {
                    junit 'results.xml'
                    publishHTML([
                        reportDir: '.', reportFiles: 'report.html',
                        reportName: 'API Test Report'
                    ])
                }
            }
        }
    }
}
```

---

## Filtering in CI

Run only a subset of tests using tags or folder globs:

```bash
# Run only tests tagged "smoke"
pebblepost run ./collections --tag smoke -e prod

# Run only requests in the "auth" folder
pebblepost run ./collections --folder auth -e dev

# Run only requests matching "*login*"
pebblepost run ./collections --request "*login*" -e dev

# Combine filters (AND semantics)
pebblepost run ./collections --tag smoke --folder auth -e prod
```

---

## Tagging Requests

Add tags to any `*.pebble.json` request file:

```json
{
  "schemaVersion": 1,
  "name": "Login",
  "method": "POST",
  "url": "{{BASE_URL}}/auth/login",
  "tags": ["smoke", "auth", "critical"],
  ...
}
```

Tags can then be targeted with `--tag smoke` in CI to run fast subset checks before full suite runs.

---

## Environment Variable Reference

| Variable | Description |
|----------|-------------|
| `PEBBLE_VAR_<KEY>` | Inject any variable (overrides env-file and selected env) |
| `NO_COLOR` | Set to any value to disable ANSI color in CLI output |
| `TERM=dumb` | Automatically disables color (used by most CI systems) |
