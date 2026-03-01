# Contributing to Asguard

Thank you for your interest in contributing to Asguard! This document explains how to propose changes, run tests, and follow the project's conventions.

## Code of Conduct

Be respectful, professional, and collaborative. Treat maintainers and contributors with kindness.

## How to contribute

1. Fork the repository and clone your fork.
2. Create a branch for your change:

   - Features: `feat/<short-description>`
   - Fixes: `fix/<issue-number-or-short-desc>`

3. Make small, focused commits. Use imperative commit messages like "Add input validation for amount".
4. Run tests and linters locally.
5. Open a pull request against the main branch with a clear description and link any related issues.

## Development setup

From the repository root:

```bash
cd backend
go mod download
go test ./...
```

If you use Firestore locally, set `FIREBASE_CREDENTIALS_PATH` to a test service account JSON or mock calls.

## Testing

- Write unit tests using Go's `testing` package.
- Place tests next to the package they validate (e.g., `services/risk_engine_test.go`).
- Run the full test suite with `go test ./...`.

## Style and linting

- Format code with `gofmt` or `gofmt -w .` before committing.
- Follow Go idioms; prefer clear, small functions.

## Pull Request checklist

- [ ] Tests added/updated for new behavior
- [ ] Code formatted (`gofmt`)
- [ ] Linter warnings addressed (if applicable)
- [ ] PR description explains motivation and approach

## Security & secrets

Never commit credentials or service account JSON to the repository. Use `backend/config/asguard.json` locally, but add the file to `.gitignore`.

If you find a security vulnerability, please open a private issue and tag maintainers; do not disclose publicly until fixed.

## Questions

If you're unsure about an approach, open an issue describing the change you want to make—maintainers will help shape the work.
