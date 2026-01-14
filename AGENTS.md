# AGENTS.md

## Build & Test Commands
- Build: `task build` or `go build .`
- Run: `task run` or `go run .`
- Test all: `task test` or `go test ./...`
- Test single: `go test -v -run TestName ./path/to/package`
- Lint: `task lint` (uses golangci-lint)
- Format: `task fmt` (uses gofumpt)

## Code Style
- Go 1.25+, CGO disabled, uses `GOEXPERIMENT=greenteagc`
- Formatting: gofumpt and goimports (enforced by linter)
- Imports: stdlib first, then external, then internal (grouped with blank lines)
- Naming: standard Go conventions (camelCase locals, PascalCase exports)
- Errors: return `error` as last value, wrap with `fmt.Errorf("context: %w", err)`
- Package comments: use `// Package name ...` doc comments on main file
- Avoid: naked returns, unused variables, unclosed resources (sql, http body)
- Tests: use `testify/require` for assertions, table-driven tests preferred
