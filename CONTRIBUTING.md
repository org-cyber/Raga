# Contributing to Asguard

Thank you for your interest in contributing to Asguard! This document explains how to propose changes, set up your development environment, run tests, and follow the project's conventions. Our goal is to make the contribution process as smooth and transparent as possible.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct:

- Be respectful, professional, and collaborative.
- Treat maintainers and contributors with kindness.
- Welcome newcomers and encourage dialogue.
- Harassment or unacceptable behavior will not be tolerated.

---

## How to Contribute

We welcome all types of contributions: bug fixes, new features, documentation improvements, and architectural suggestions.

### 1. Discuss Before You Build

For significant changes, architectural updates, or adding large new dependencies, please **open an issue** to discuss your ideas first. This ensures your work aligns with the project's direction and prevents wasted effort.

### 2. Fork & Clone

1. Fork the repository to your own GitHub account.
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/asguard.git
   cd asguard/backend
   ```
3. Set up the upstream remote to stay up-to-date:
   ```bash
   git remote add upstream https://github.com/ORIGINAL_OWNER/asguard.git
   ```

### 3. Branch Naming Strategy

Create a new branch for your work. Use the following convention to categorize your changes:

- **Features:** `feat/<short-description>` (e.g., `feat/add-stripe-webhook`)
- **Bug Fixes:** `fix/<issue-number-or-short-desc>` (e.g., `fix/132-nil-pointer-deref`)
- **Documentation:** `docs/<short-description>` (e.g., `docs/update-readme-api`)
- **Refactoring:** `refactor/<short-description>` (e.g., `refactor/extract-ai-logic`)

```bash
git checkout -b feat/add-stripe-webhook
```

---

## Development Setup

The project relies heavily on Go and optionally Docker for local development.

### Prerequisites

- Go 1.25 or newer
- Docker & Docker Compose (optional but recommended)

### Local Go Setup

1. Navigate to the `backend` directory.
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Copy environment variables:
   Create a `.env` file in the `backend/` directory by copying the example or using the following keys:
   ```env
   ASGUARD_API_KEY=test_api_key_123
   GROQ_API_KEY=test_groq_key_123
   PORT=8081
   ```
4. The project is a stateless fraud-detection API by default. If you need persistence for a custom deployment, implement an external adapter and mock it in tests.

5. Run the server:
   ```bash
   go run main.go
   ```

### Docker Setup

For an isolated environment, run:

```bash
docker compose -f docker-compose.dev.yml up --build
```

This mounts the local directory into the container.

---

## Testing Guidelines

Testing is critical for fraud detection systems. All new features and bug fixes must include tests.

- **Unit Tests:** Write tests using Go's built-in `testing` package.
- **Location:** Place tests next to the package they validate (e.g., `services/risk_engine_test.go`).
- **Running Tests:**
  ```bash
  go test ./... -v
  ```
- Make sure to mock external network calls like the Groq API in unit tests.

---

## Style and Linting

- **Formatting:** Always format your code with `gofmt` before committing.
  ```bash
  gofmt -w .
  ```
- **Idiomatic Go:** Follow standard Go idioms. Prefer clear, small functions with explicit error handling. Avoid panics.
- **Linters:** (Optional but encouraged) Run `golangci-lint run` locally to catch potential issues early.

---

## Commit Conventions

We encourage the use of [Conventional Commits](https://www.conventionalcommits.org/). This allows us to auto-generate changelogs in the future.

**Format:** `<type>(<scope>): <subject>`

**Examples:**

- `feat(api): add endpoint for batch transactions`
- `fix(ai): handle rate limit 429 response gracefully`
- `docs(readme): update quickstart section`
- `test(risk): add boundary tests for amount tiering`

Keep commits small, logical, and focused on a single change.

---

## Pull Request Process

When you are ready to submit your code:

1. Push your branch to your fork:
   ```bash
   git push origin feat/your-feature
   ```
2. Open a Pull Request against the `main` branch of the original repository.
3. **PR Checklist:** Ensure the following items are met before submitting:
   - [ ] All local tests pass (`go test ./...`).
   - [ ] Code is formatted (`gofmt -w .`).
   - [ ] New functionality is covered by tests.
   - [ ] Documentation (README / ARCHITECTURE) is updated if applicable.
   - [ ] The PR description clearly explains the _motivation_ and _approach_.
   - [ ] Any related issues are linked (e.g., `Closes #123`).

Maintainers will review your PR, provide constructive feedback, and may request changes. Once approved and CI passes, your PR will be merged!

---

## Security & Secrets

**NEVER** commit API keys or any secrets to the repository. The `.gitignore` is set up to ignore `.env` and typical credential files, but please double-check your commits.

### Reporting Vulnerabilities

If you discover a security vulnerability, please do **NOT** open a public issue. Instead, reach out to the core maintainers privately. Once a patch is developed and deployed, a public disclosure will be made.

---

## Questions?

If you're unsure about an approach, stuck on a bug, or just want to ask a question, please open an Issue. The maintainers and the community are happy to help guide you!
