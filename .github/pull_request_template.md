## Description
<!-- Please include a summary of the change, relevant motivation, and context. -->

Fixes # (issue)

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Packaging / Distribution release update
- [ ] Performance or refactoring improvement

## How Has This Been Tested?
<!-- Please describe the tests that you ran to verify your changes. -->
- [ ] Go backend tests pass with race detection: `go test -race ./...`
- [ ] Frontend unit tests pass: `cd web && npm test`
- [ ] Frontend production build passes: `cd web && npm run build`
- [ ] Desktop studio / server verified: `go build ./cmd/server && go build ./cmd/cli`

## Checklist
- [ ] My code follows the style guidelines of this project
- [ ] I have performed a self-review of my own code
- [ ] I have adhered to the Zero-Mock Data Policy (no fake mock arrays, authentic empty states)
- [ ] I have commented my code where necessary, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation (e.g. `docs/`, `README.md`)
- [ ] My changes generate no new warnings or type errors
- [ ] Commit history follows Conventional Commits format
