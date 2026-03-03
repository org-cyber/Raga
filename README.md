<img src="Gemini_Generated_Image_2r9uh62r9uh62r9u-removebg-preview (1).png" width="300">

# Asguard — AI-Powered Fraud Detection + Biometric Face Verification

![Version](https://img.shields.io/badge/version-1.1.0-blue.svg) ![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg) ![License](https://img.shields.io/badge/license-MIT-green.svg) ![Services](https://img.shields.io/badge/services-2-orange.svg)

Asguard is a real-time fraud detection platform combining rule-based risk scoring, AI-enhanced transaction analysis, and biometric face recognition. It is built as a multi-service system: a **Backend API** for transaction risk scoring and an **asguard-face** microservice for facial embedding extraction and comparison.

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Architecture & Request Flow](#architecture--request-flow)
- [Services](#services)
  - [Backend Service](#backend-service-port-8081)
  - [Face Service](#face-service-port-8082)
- [Prerequisites](#prerequisites)
- [Quickstart: Local Development](#quickstart-local-development)
  - [Running with Go](#running-with-go)
  - [Running with Docker Compose](#running-with-docker-compose)
- [Configuration Details](#configuration-details)
- [API Reference](#api-reference)
- [Project Layout](#project-layout)
- [Testing & CI](#testing--ci)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

Asguard scores financial transactions using a weighted rule engine and optionally escalates higher-risk cases to an AI (LLM) analysis service. In addition, a dedicated **face microservice** enables biometric identity verification — accepting images, extracting 128-dimension facial embeddings using dlib, and comparing them against stored references.

The system is designed to be lightweight, modular, containerized, and highly extensible.

**Primary Use Cases:**

- Real-time risk scoring for payments.
- Pre-authorization fraud checks.
- Triaging suspicious transactions for manual review.
- Biometric identity verification at onboarding or transaction time.

---

## Key Features

- **Rule-Based Scoring:** Configurable, tiered scoring engine based on transaction amount, currency, device, IP, and location.
- **AI Escalation:** Automatically calls an LLM (Groq AI `llama-3.3-70b-versatile`) for transactions crossing a risk threshold (Score ≥ 40) for a second opinion.
- **Face Recognition Microservice:** A standalone Go service powered by `go-face` (dlib) that exposes REST endpoints for facial embedding extraction and similarity comparison.
- **CORS Support:** The face service is configured with full Cross-Origin Resource Sharing (CORS) support via `gin-contrib/cors`, allowing frontend applications and other services to query it directly. Allows all origins with `GET`, `POST`, and `OPTIONS` methods, including `Authorization` headers.
- **Multi-Stage Docker Builds:** The `asguard-face` Dockerfile uses a two-stage build — a `golang:1.25-bookworm` builder stage for compiling, and a lean `debian:bookworm-slim` runtime stage — drastically reducing the final image size.
- **RESTful API:** Clean, JSON-based APIs powered by Gin across both services.
- **API Key Security:** Built-in auth middleware on all protected endpoints. The face service reads API keys from a comma-separated `API_KEYS` environment variable and validates `Authorization: Bearer <key>` headers.
- **Request Tracing:** Each request to the face service is assigned a UUID-based `X-Request-ID` header for distributed tracing and log correlation.
- **Image Quality Scoring:** The face service optionally computes brightness, sharpness (Laplacian variance), and face-size ratio to flag poor-quality images before processing.
- **Dockerized:** Full multi-service local setup via `docker-compose`.

---

## Architecture & Request Flow

Asguard now operates as a two-service platform. Each service has its own responsibility and can be deployed and scaled independently.

### High-Level Architecture

```mermaid
flowchart TD
  ClientApp[(Client Apps / Frontend)] -->|HTTP POST| BackendAPI["Backend API\n(Gin · Port 8081)"]
  ClientApp -->|HTTP POST| FaceAPI["Face Service\n(Gin · Port 8082)"]

  BackendAPI --> MW["Middleware\n(x-api-key Auth)"]
  MW --> RiskEngine["Risk Engine\n(Rule-based Scorer)"]
  RiskEngine -- "Score ≥ 40" --> GroqAI["Groq AI\n(llama-3.3-70b)"]

  FaceAPI --> CORS["CORS Middleware\n(gin-contrib/cors)"]
  CORS --> FaceAuth["Auth Middleware\n(Bearer Token)"]
  FaceAuth --> FaceRoutes["Routes\n/v1/analyze · /v1/compare · /health"]
  FaceRoutes --> DlibRecognizer["go-face Recognizer\n(dlib · 128D Embeddings)"]
```

### Request Lifecycle — Backend (Transaction Risk)

1. **Auth:** The `APIKeyAuth` middleware validates the `x-api-key` header.
2. **Binding:** The route handler parses and validates the JSON payload.
3. **Rule Evaluation:** `risk_engine.go` applies weights to Amount, Currency, IP, Device, and Location — producing a score from 0–100.
4. **AI Gating:** If score ≥ 40, `ai_service.go` calls the Groq API with a structured system prompt.
5. **AI Influence:** The AI's recommendation (`APPROVE`, `REVIEW`, `BLOCK`) can upgrade but never downgrade the rule-based risk level.
6. **Response:** A JSON object is returned with the score, risk level, reasons, and the AI's full analysis.

### Request Lifecycle — Face Service (Biometric)

1. **CORS Preflight:** `gin-contrib/cors` handles browser `OPTIONS` requests and injects CORS headers.
2. **Auth:** The `authMiddleware` validates `Authorization: Bearer <key>` against the `API_KEYS` env var (health check is exempt).
3. **Request ID:** A UUID is generated per-request and returned as `X-Request-ID`.
4. **Image Processing:** The base64-encoded image is decoded to raw bytes and passed to the global `face.Recognizer` (loaded once at startup from the models path).
5. **Embedding Extraction or Comparison:** For `/v1/analyze`, a 128D embedding is returned. For `/v1/compare`, the probe embedding is compared against the supplied reference using Euclidean distance.
6. **Quality Checks:** Brightness, sharpness, and face-size ratio are computed and returned alongside warnings if requested.

_For a deep code walkthrough, see [ARCHITECTURE.md](ARCHITECTURE.md)._

---

## Services

### Backend Service (Port 8081)

Handles financial transaction analysis using the rule-based scoring engine + Groq LLM.

| Endpoint   | Method | Auth Required   | Description                    |
| ---------- | ------ | --------------- | ------------------------------ |
| `/health`  | GET    | No              | Service health check           |
| `/analyze` | POST   | Yes (x-api-key) | Analyze a transaction for risk |

### Face Service (Port 8082)

Handles biometric face analysis using dlib-powered deep learning embeddings.

| Endpoint      | Method | Auth Required      | Description                                    |
| ------------- | ------ | ------------------ | ---------------------------------------------- |
| `/health`     | GET    | No                 | Service health check                           |
| `/v1/analyze` | POST   | Yes (Bearer token) | Extract 128D embedding from an image           |
| `/v1/compare` | POST   | Yes (Bearer token) | Compare a probe image to a reference embedding |

---

## Prerequisites

To run Asguard locally, you will need:

- **[Go 1.25+](https://go.dev/dl/)**
- **[Git](https://git-scm.com/)**
- **[Docker & Docker Compose](https://www.docker.com/)** (recommended for the face service — requires dlib)
- **Groq API Key:** Required for AI-enhanced analysis. Get one at [console.groq.com](https://console.groq.com/).
- **dlib Models:** Required for the face service. Download and place in the `./models/` directory at the project root:
  - `shape_predictor_5_face_landmarks.dat`
  - `dlib_face_recognition_resnet_model_v1.dat`
  - `mmod_human_face_detector.dat`

---

## Quickstart: Local Development

### Running with Go

#### Backend

```bash
git clone <repo-url>
cd asguard/backend
go mod download
```

Create a `.env` file in `backend/`:

```env
ASGUARD_API_KEY=super-secret-key
GROQ_API_KEY=your_groq_api_key
PORT=8081
```

```bash
go run main.go
```

#### Face Service

> ⚠️ The face service requires native dlib libraries. Docker is strongly recommended.

```bash
cd asguard/asguard-face
go mod download
MODELS_PATH=../../models API_KEYS=dev-key-123 PORT=8082 go run main.go
```

### Running with Docker Compose

The `docker-compose.yml` at the project root orchestrates both services simultaneously:

```bash
# From the project root
docker compose up --build
```

This will:

- Build and start the **backend** service on port `8081`
- Build and start the **asguard-face** service on port `8082` using the multi-stage Dockerfile
- Mount the `./models/` directory into the face container (read-only)
- Connect both services on the `asguard-network` bridge network

_To stop gracefully: `Ctrl+C` then `docker compose down`_

---

## Configuration Details

### Backend Environment Variables

| Variable          | Description                                          | Default | Required? |
| ----------------- | ---------------------------------------------------- | ------- | --------- |
| `ASGUARD_API_KEY` | Secret key required by clients (`x-api-key` header). | None    | Yes       |
| `GROQ_API_KEY`    | API key for the Groq LLM service.                    | None    | Yes       |
| `PORT`            | Port the Gin server binds to.                        | `8081`  | No        |

### Face Service Environment Variables

| Variable      | Description                                             | Default       | Required? |
| ------------- | ------------------------------------------------------- | ------------- | --------- |
| `API_KEYS`    | Comma-separated list of valid API keys (Bearer tokens). | `dev-key-123` | Yes       |
| `MODELS_PATH` | Path to the directory containing dlib model files.      | `./models`    | Yes       |
| `PORT`        | Port the Gin server binds to.                           | `8082`        | No        |

---

## API Reference

### Backend — Health Check

```http
GET http://localhost:8081/health
```

**Response:**

```json
{ "status": "asguard health running" }
```

### Backend — Analyze Transaction

```http
POST http://localhost:8081/analyze
x-api-key: <ASGUARD_API_KEY>
Content-Type: application/json
```

**Request Body:**

```json
{
  "user_id": "user_123",
  "transaction_id": "txn_456",
  "amount": 250000,
  "currency": "USD",
  "ip_address": "192.168.1.5",
  "device_id": "device_789",
  "location": "Lagos, Nigeria"
}
```

**Response:**

```json
{
  "transaction_id": "txn_456",
  "risk_score": 41,
  "risk_level": "MEDIUM",
  "reasons": [
    "High transaction amount (>100k)",
    "Foreign currency transaction (USD)",
    "AI recommends REVIEW: Large USD transaction from Lagos warrants manual review"
  ],
  "ai_triggered": true,
  "ai_confidence": 0.85,
  "ai_recommendation": "REVIEW",
  "ai_fraud_probability": 0.45,
  "ai_summary": "Large USD transaction from Lagos warrants manual review",
  "message": "Transaction analyzed successfully"
}
```

---

### Face Service — Health Check

```http
GET http://localhost:8082/health
```

**Response:**

```json
{ "status": "healthy", "service": "asguard-face", "version": "1.0.0" }
```

### Face Service — Extract Embedding

Extracts a 128-dimension facial embedding from a base64-encoded image.

```http
POST http://localhost:8082/v1/analyze
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

**Request Body:**

```json
{
  "image": "<base64-encoded-image>",
  "quality_checks": true
}
```

**Response:**

```json
{
  "success": true,
  "face_detected": true,
  "embedding": [0.023, -0.14, ...],
  "quality_score": 1.0,
  "sharpness": 0.82,
  "brightness": 142.5,
  "face_size_ratio": 0.34,
  "warnings": [],
  "processing_time_ms": 312
}
```

### Face Service — Compare Faces

Compares a probe image against a stored 128D reference embedding.

```http
POST http://localhost:8082/v1/compare
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

**Request Body:**

```json
{
  "probe_image": "<base64-encoded-image>",
  "reference_embedding": [0.023, -0.14, ...],
  "threshold": 0.6
}
```

**Response:**

```json
{
  "success": true,
  "match": true,
  "confidence": 0.78,
  "distance": 0.131,
  "threshold_used": 0.6,
  "probe_quality": 1.0,
  "processing_time_ms": 287
}
```

---

## Project Layout

```text
asguard/
├── README.md                      # This file
├── ARCHITECTURE.md                # Deep-dive code walkthrough
├── CONTRIBUTING.md                # Contribution guidelines
├── docker-compose.yml             # Multi-service orchestration (backend + face)
├── models/                        # dlib model files (not committed to git)
│   ├── shape_predictor_5_face_landmarks.dat
│   ├── dlib_face_recognition_resnet_model_v1.dat
│   └── mmod_human_face_detector.dat
│
├── backend/                       # Transaction risk scoring service
│   ├── main.go                    # Entry point, Gin router setup
│   ├── .env                       # Local secrets (git-ignored)
│   ├── Dockerfile                 # Backend container build
│   ├── middleware/
│   │   └── apikey.go              # x-api-key validation middleware
│   ├── routes/
│   │   └── routes.go              # HTTP route definitions
│   └── services/
│       ├── ai_service.go          # Groq LLM integration
│       └── risk_engine.go         # Core rule-based scoring logic
│
└── asguard-face/                  # Biometric face recognition service
    ├── main.go                    # Entry point, face recognizer, all routes
    ├── Dockerfile                 # Multi-stage build (builder + slim runtime)
    └── go.mod / go.sum
```

---

## Testing & CI

### Running Tests Locally

```bash
# Backend
cd backend
go test ./... -v

# Face service
cd asguard-face
go test ./... -v
```

### Linting & Formatting

```bash
gofmt -w .
go vet ./...
```

### CI Pipeline

The repository includes a GitHub Actions workflow (`.github/workflows/ci.yml`) that runs on every push and Pull Request to `main`. It verifies:

- Go formatting (`gofmt`)
- Static analysis (`go vet`)
- Test suite (`go test`)

---

## Contributing

We strongly welcome community contributions! Please read our [CONTRIBUTING.md](CONTRIBUTING.md) for detailed instructions on the development workflow, branching strategies, and testing guidelines.

**Quick Checklist:**

- Open an Issue first to discuss large changes.
- Branch off `main` using the format `feat/feature-name` or `fix/bug-name`.
- Ensure all tests pass (`go test ./...`).
- Submit a Pull Request with a clear description.

---

## License

This project is licensed under the [APACHE 2.0 License](LICENSE.txt).
