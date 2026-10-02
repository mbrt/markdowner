# Coding agents instructions

## Development

Use `make format` to auto-format and fix linting issues.

Always tidy after adding or removing dependencies:

```bash
go get $package
go mod tidy
```

### Tests

- Always use `make test` to run tests
- Test interfaces and intended behavior instead of internals
- Prefer integration tests to mocks as much as possible
