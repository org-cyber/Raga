# Asguard — Architecture, Code Walkthrough & Change Log

> **Last updated:** 2026-03-03
> This document explains how every part of the Asguard platform works, how services communicate, what was built and why, and all notable changes made throughout development — with code snippets throughout.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [System Architecture](#2-system-architecture)
3. [Folder Structure](#3-folder-structure)
4. [Backend Service — Deep Dive](#4-backend-service--deep-dive)
   - [main.go](#41-maingo)
   - [middleware/apikey.go](#42-middlewareapikeygo)
   - [routes/routes.go](#43-routesroutesgo)
   - [services/risk_engine.go](#44-servicesrisk_enginego)
   - [services/ai_service.go](#45-servicesai_servicego)
5. [Face Service — Deep Dive](#5-face-service--deep-dive)
   - [Overview & Startup](#51-overview--startup)
   - [CORS Middleware](#52-cors-middleware-new-2026-03-03)
   - [Auth Middleware](#53-auth-middleware)
   - [Request ID Middleware](#54-request-id-middleware)
   - [Routes](#55-routes)
   - [handleAnalyze — Embedding Extraction](#56-handleanalyze--embedding-extraction)
   - [handleCompare — Face Similarity](#57-handlecompare--face-similarity)
   - [Image Quality Scoring](#58-image-quality-scoring)
6. [Dockerfile — Multi-Stage Build](#6-dockerfile--multi-stage-build-new-2026-03-03)
7. [Docker Compose — Multi-Service Orchestration](#7-docker-compose--multi-service-orchestration)
8. [Backend — Bugs Fixed & Improvements](#8-backend--bugs-fixed--improvements)
9. [Face Service — Changes on 2026-03-03](#9-face-service--changes-on-2026-03-03)
10. [How to Test Both APIs](#10-how-to-test-both-apis)

---

## 1. Project Overview

**Asguard** is a fraud detection and identity verification platform built in Go. It operates as two independent microservices:

| Service        | Port | Responsibility                                             |
| -------------- | ---- | ---------------------------------------------------------- |
| `backend`      | 8081 | Transaction risk scoring (rule-based + Groq LLM)           |
| `asguard-face` | 8082 | Biometric face recognition (dlib embeddings via `go-face`) |

The two services are orchestrated together by `docker-compose.yml` at the project root and communicate on a shared Docker bridge network (`asguard-network`).

### Tech Stack

| Layer          | Backend                 | Face Service                      |
| -------------- | ----------------------- | --------------------------------- |
| Language       | Go 1.25                 | Go 1.25                           |
| HTTP Framework | Gin                     | Gin                               |
| Auth           | x-api-key middleware    | Bearer token middleware           |
| CORS           | —                       | `gin-contrib/cors`                |
| AI / ML        | Groq API (LLM)          | dlib via `go-face` (CNN)          |
| Tracing        | —                       | UUID per-request (`X-Request-ID`) |
| Container      | Single-stage Dockerfile | **Multi-stage Dockerfile**        |

---

## 2. System Architecture

### Platform-Level Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        Client Applications                        │
│              (Web Frontend / Mobile SDK / Other Services)         │
└───────┬───────────────────────────────────────┬──────────────────┘
        │                                       │
        │  POST /analyze                        │  POST /v1/analyze
        │  x-api-key: <key>                     │  POST /v1/compare
        │                                       │  Authorization: Bearer <key>
        ▼                                       ▼
┌───────────────────────┐             ┌──────────────────────────────┐
│   backend             │             │   asguard-face               │
│   Port 8081           │             │   Port 8082                  │
│                       │             │                              │
│  ┌─────────────────┐  │             │  ┌────────────────────────┐  │
│  │ APIKeyAuth      │  │             │  │ CORS Middleware         │  │
│  │ Middleware      │  │             │  │ (gin-contrib/cors)      │  │
│  └────────┬────────┘  │             │  └────────────┬───────────┘  │
│           │           │             │               │              │
│  ┌────────▼────────┐  │             │  ┌────────────▼───────────┐  │
│  │ Risk Engine     │  │             │  │ Auth Middleware         │  │
│  │ (Rule-based)    │  │             │  │ (Bearer Token)         │  │
│  └────────┬────────┘  │             │  └────────────┬───────────┘  │
│           │           │             │               │              │
│  ┌────────▼────────┐  │             │  ┌────────────▼───────────┐  │
│  │  AI Gate        │  │             │  │ Request ID Middleware   │  │
│  │  (Score >= 40)  │  │             │  │ (UUID per request)     │  │
│  └────────┬────────┘  │             │  └────────────┬───────────┘  │
│           │           │             │               │              │
└───────────┼───────────┘             │  ┌────────────▼───────────┐  │
            │                        │  │ go-face Recognizer      │  │
            ▼                        │  │ (dlib, 128D embeddings)  │  │
     Groq API (LLM)                  │  └────────────────────────┘  │
     llama-3.3-70b                   └──────────────────────────────┘
```

### Docker Network Topology

```
  docker-compose.yml
  ┌──────────────────────────────────────────┐
  │  asguard-network (bridge)                │
  │                                          │
  │   ┌──────────────┐  ┌─────────────────┐  │
  │   │  backend     │  │  asguard-face   │  │
  │   │  :8081       │  │  :8082          │  │
  │   └──────────────┘  └─────────────────┘  │
  │                           │              │
  │                    volume: ./models      │
  │                           (read-only)   │
  └──────────────────────────────────────────┘
```

---

## 3. Folder Structure

```
asguard/
├── README.md
├── ARCHITECTURE.md                 ← This file
├── CONTRIBUTING.md
├── docker-compose.yml              ← Orchestrates backend + asguard-face
├── models/                         ← dlib model files (shared volume, not committed)
│   ├── shape_predictor_5_face_landmarks.dat
│   ├── dlib_face_recognition_resnet_model_v1.dat
│   └── mmod_human_face_detector.dat
│
├── backend/                        ← Transaction risk service
│   ├── main.go
│   ├── Dockerfile
│   ├── middleware/
│   │   └── apikey.go
│   ├── routes/
│   │   └── routes.go
│   └── services/
│       ├── ai_service.go
│       └── risk_engine.go
│
└── asguard-face/                   ← Face recognition microservice
    ├── main.go                     ← All-in-one: server, handlers, middleware, helpers
    ├── Dockerfile                  ← Multi-stage build
    ├── go.mod
    └── go.sum
```

---

## 4. Backend Service — Deep Dive

### 4.1 `main.go`

The backend entry point does three things:

```go
func main() {
    // 1. Load .env so os.Getenv("GROQ_API_KEY") works everywhere
    if err := godotenv.Load(); err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }

    // 2. Create Gin router
    router := gin.Default()

    // 3. Register routes (health check + /analyze)
    routes.RegisterRoutes(router)

    // 4. Listen on port 8081
    router.Run(":8081")
}
```

`godotenv.Load()` must run first — `ai_service.go` reads `GROQ_API_KEY` at call time. A missing key causes an intentional fatal crash rather than a silent failure.

---

### 4.2 `middleware/apikey.go`

Every protected route passes through this gate. It reads the `x-api-key` header and compares against the value stored in `.env`:

```go
func APIKeyAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        apikey := c.GetHeader("x-api-key")
        expectedKey := os.Getenv("ASGUARD_API_KEY")

        if apikey == "" || apikey != expectedKey {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorised"})
            c.Abort() // stops the chain — handler never runs
            return
        }
        c.Next()
    }
}
```

**How it connects:** In `routes.go`, the middleware wraps a route group so all endpoints under it are protected automatically:

```go
protected := router.Group("/")
protected.Use(middleware.APIKeyAuth())
protected.POST("/analyze", AnalyzeTransaction)
```

---

### 4.3 `routes/routes.go`

Defines the shape of the incoming request and handles parsing/response:

```go
type TransactionRequest struct {
    UserID        string  `json:"user_id"        binding:"required"`
    TransactionID string  `json:"transaction_id" binding:"required"`
    Amount        float64 `json:"amount"         binding:"required"`
    Currency      string  `json:"currency"       binding:"required"`
    IPAddress     string  `json:"ip_address"     binding:"required"`
    DeviceID      string  `json:"device_id"      binding:"required"`
    Location      string  `json:"location"`   // optional
    Timestamp     string  `json:"timestamp"`  // optional
}
```

`binding:"required"` causes Gin to reject with HTTP 400 if a field is absent. Fields without it are silently optional.

The handler itself does **no business logic** — it only marshals HTTP concerns and delegates to `services/`:

```go
func AnalyzeTransaction(c *gin.Context) {
    var req TransactionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request payload"})
        return
    }
    riskResult := services.CalculateRisk(services.TransactionData{ ... })
    c.JSON(200, gin.H{ "risk_score": riskResult.Score, ... })
}
```

---

### 4.4 `services/risk_engine.go`

The scoring core. It produces a `RiskResult` from a `TransactionData` input.

#### Scoring Rules

Each rule produces a risk value from `0.0` to `1.0`. Weights sum exactly to **1.0**, so the final score is a true 0–100 percentage:

```go
// Rule 1: Amount (35%) — tiered, not binary
switch {
case tx.Amount > 500000:
    amountRisk = 1.0   // extreme
case tx.Amount > 100000:
    amountRisk = 0.6   // high
case tx.Amount > 50000:
    amountRisk = 0.3   // moderate
}

// Rule 2: Currency (20%) — non-NGN = riskier foreign transaction
if tx.Currency != "NGN" { currencyRisk = 1.0 }

// Rule 3: Device ID (15%) — missing = anonymous sender
if tx.DeviceID == "" { deviceRisk = 1.0 }

// Rule 4: IP Address (15%) — missing = untraceable
if tx.IPAddress == "" { ipRisk = 1.0 }

// Rule 5: Location (15%) — missing = unverifiable origin
if tx.Location == "" { locationRisk = 1.0 }

// Final weighted score (0.35 + 0.20 + 0.15 + 0.15 + 0.15 = 1.0)
score := (amountRisk * 0.35) + (currencyRisk * 0.20) +
         (deviceRisk * 0.15) + (ipRisk * 0.15) + (locationRisk * 0.15)
finalScore := int(score * 100)
```

#### AI Gate

```go
if finalScore >= 40 {
    aiTriggered = true
    log.Printf("[AI GATE] Score=%d for txn=%s — calling Groq AI...", finalScore, tx.TransactionID)

    result, err := AnalyzeTransaction(tx, finalScore)
    if err != nil {
        log.Printf("[AI ERROR] txn=%s: %v", tx.TransactionID, err)
        level = "HIGH"
        reasons = append(reasons, "AI analysis unavailable — escalated to HIGH for manual review")
    } else {
        switch result.RecommendedAction {
        case "BLOCK":
            level = "HIGH"
        case "REVIEW":
            if level == "LOW" { level = "MEDIUM" } // AI can upgrade, never downgrade
        }
    }
}
```

---

### 4.5 `services/ai_service.go`

Responsible for calling Groq's API and parsing the structured response.

#### Prompt Strategy

Two-role prompting ensures reliable, deterministic JSON output from the LLM:

```go
systemPrompt := `You are a financial fraud detection AI for a Nigerian fintech platform.
You MUST respond with ONLY a valid JSON object — no markdown, no explanation outside the JSON.
The JSON must follow this exact schema:
{
  "fraud_probability": <float between 0.0 and 1.0>,
  "recommended_action": <"APPROVE" | "REVIEW" | "BLOCK">,
  "reasoning": <one concise sentence>,
  "confidence": <float between 0.0 and 1.0>
}`

userPrompt := fmt.Sprintf(`Assess this transaction for fraud risk:
Transaction ID : %s
Amount         : %.2f %s
Location       : %s
Baseline Score : %d/100
Respond with JSON only.`, tx.TransactionID, tx.Amount, tx.Currency, tx.Location, score)
```

#### Markdown Fence Stripping

LLMs sometimes wrap JSON in code fences even when instructed not to. The parser defensively strips them:

````go
rawContent = strings.TrimPrefix(rawContent, "```json")
rawContent = strings.TrimPrefix(rawContent, "```")
rawContent = strings.TrimSuffix(rawContent, "```")
rawContent = strings.TrimSpace(rawContent)
````

---

## 5. Face Service — Deep Dive

The face service is a self-contained Go application in `asguard-face/main.go`. It uses a single global `face.Recognizer` instance (loaded once at startup) shared across all request handlers.

### 5.1 Overview & Startup

```go
var recognizer *face.Recognizer  // Global, initialized once
var apiKeys map[string]bool       // Parsed from API_KEYS env var

func main() {
    modelsPath := getEnv("MODELS_PATH", "./models")
    apiKeyList := getEnv("API_KEYS", "dev-key-123")

    // Parse comma-separated API keys into a lookup map
    apiKeys = make(map[string]bool)
    for _, key := range strings.Split(apiKeyList, ",") {
        apiKeys[strings.TrimSpace(key)] = true
    }

    // Load dlib models — this is expensive, done once at startup
    var err error
    recognizer, err = face.NewRecognizer(modelsPath)
    if err != nil {
        log.Fatalf("Failed to load face models from %s: %v", modelsPath, err)
    }
    defer recognizer.Close()

    log.Printf("Loaded face models from %s", modelsPath)

    gin.SetMode(gin.ReleaseMode)
    r := gin.New()
    r.Use(gin.Recovery())
    r.Use(requestIDMiddleware())
    r.Use(cors.New(...))      // CORS — added 2026-03-03
    r.Use(authMiddleware())

    r.POST("/v1/analyze", handleAnalyze)
    r.POST("/v1/compare", handleCompare)
    r.GET("/health", handleHealth)

    port := getEnv("PORT", "8082")
    r.Run(":" + port)
}
```

**Why load models once?** Loading dlib CNN models (especially the ResNet face recognizer) takes several seconds and significant memory. Loading them once at boot and reusing the global `recognizer` across all requests is the standard pattern — it would be a critical performance bug to initialize it per-request.

---

### 5.2 CORS Middleware _(New — 2026-03-03)_

**Why it was added:** Without CORS headers, browsers block cross-origin requests to the face service. Any frontend or web-based SDK calling the service directly would receive a CORS error.

The middleware is registered before `authMiddleware` so that `OPTIONS` preflight requests from browsers can succeed even before auth is checked:

```go
r.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"*"},  // Allow all origins — set to your frontend URL in production
    AllowMethods:     []string{"GET", "POST", "OPTIONS"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
    ExposeHeaders:    []string{"Content-Length"},
    AllowCredentials: true,
}))
```

| Config Key         | Value                    | Reason                                                                            |
| ------------------ | ------------------------ | --------------------------------------------------------------------------------- |
| `AllowOrigins`     | `["*"]`                  | Permissive during development. Restrict to specific domains in production.        |
| `AllowMethods`     | `GET, POST, OPTIONS`     | Covers health check, data endpoints, and browser preflights.                      |
| `AllowHeaders`     | includes `Authorization` | Critical — without this, `Authorization: Bearer <key>` is blocked by the browser. |
| `AllowCredentials` | `true`                   | Required if cookies or auth headers are sent cross-origin.                        |

> ⚠️ **Production Note:** Replace `AllowOrigins: ["*"]` with your specific frontend domain (e.g., `["https://app.yourdomain.com"]`) to prevent unauthorized cross-origin access.

---

### 5.3 Auth Middleware

Validates the `Authorization: Bearer <token>` header against the parsed `apiKeys` map. The `/health` route is explicitly exempt:

```go
func authMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.Request.URL.Path == "/health" {
            c.Next()
            return
        }

        key := c.GetHeader("Authorization")
        key = strings.TrimPrefix(key, "Bearer ") // strip prefix if present

        if !apiKeys[key] {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "success": false,
                "error":   "Invalid or missing API key",
            })
            return
        }
        c.Next()
    }
}
```

**Multiple key support:** `API_KEYS=key-1,key-2,key-3` — all keys are valid simultaneously. This enables key rotation without downtime (add a new key, deploy, retire the old key).

---

### 5.4 Request ID Middleware

Every request gets a unique UUID injected into the Gin context and returned as a response header:

```go
func requestIDMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestID := uuid.New().String()
        c.Set("request_id", requestID)        // available to handlers via c.GetString("request_id")
        c.Header("X-Request-ID", requestID)   // returned to caller for tracing
        c.Next()
    }
}
```

This enables log correlation across distributed systems. Every log line inside a handler prefixes with `[requestID]`, making it trivial to trace a single request through the logs even under high concurrency.

---

### 5.5 Routes

```
GET  /health         → handleHealth      (no auth)
POST /v1/analyze     → handleAnalyze     (auth required)
POST /v1/compare     → handleCompare     (auth required)
```

---

### 5.6 `handleAnalyze` — Embedding Extraction

Accepts a base64 image, decodes it, runs dlib face detection, and returns a 128-dimension embedding vector:

```go
func handleAnalyze(c *gin.Context) {
    start := time.Now()
    requestID := c.GetString("request_id")

    var req AnalyzeRequest
    if err := c.ShouldBindJSON(&req); err != nil { /* 400 */ return }

    // 1. Decode base64 → raw image bytes
    imgBytes, err := decodeBase64ToBytes(req.Image)

    // 2. Optionally decode to image.Image for quality checks
    var img image.Image
    if req.QualityChecks {
        img, _, err = image.Decode(bytes.NewReader(imgBytes))
    }

    // 3. Run dlib face detection on raw bytes
    faces, err := recognizer.Recognize(imgBytes)

    // 4. Guard: exactly one face required
    if len(faces) == 0 { /* "No face detected" */ return }
    if len(faces) > 1  { /* "Multiple faces detected" */ return }

    // 5. Extract 128D descriptor
    embedding := faces[0].Descriptor[:]

    // 6. Optional quality scoring
    if req.QualityChecks && img != nil {
        qualityScore, sharpness, brightness, faceSize, warnings =
            checkQuality(img, faces[0].Rectangle)
    }

    c.JSON(http.StatusOK, AnalyzeResponse{
        Success:          true,
        FaceDetected:     true,
        Embedding:        embedding,
        QualityScore:     qualityScore,
        Sharpness:        sharpness,
        Brightness:       brightness,
        FaceSizeRatio:    faceSize,
        Warnings:         warnings,
        ProcessingTimeMs: time.Since(start).Milliseconds(),
    })
}
```

**Key design choice — raw bytes vs file path:** `recognizer.Recognize(imgBytes)` accepts raw JPEG/PNG bytes directly. This avoids writing temporary files to disk, which would be slow and require cleanup. The recognizer decodes the image internally using the `libjpeg62-turbo` library.

---

### 5.7 `handleCompare` — Face Similarity

Takes a probe image and a pre-computed 128D reference embedding. Extracts the probe's embedding, computes Euclidean distance, and returns a match decision:

```go
func handleCompare(c *gin.Context) {
    // Validate reference embedding is exactly 128 dimensions
    if len(req.ReferenceEmbedding) != 128 { /* error */ return }

    // Extract probe embedding (same pipeline as handleAnalyze)
    faces, err := recognizer.Recognize(imgBytes)
    // ... single-face guards ...

    // Euclidean distance comparison
    var refArray, probeArray [128]float32
    copy(refArray[:], req.ReferenceEmbedding)
    probeArray = faces[0].Descriptor

    threshold := float32(0.6)  // default; caller can override
    if req.Threshold != nil { threshold = *req.Threshold }

    match, confidence, distance := compareEmbeddings(refArray, probeArray, threshold)
}
```

#### The Distance Formula

```go
func compareEmbeddings(ref, probe [128]float32, threshold float32) (match bool, confidence, distance float32) {
    var sum float32
    for i := 0; i < 128; i++ {
        diff := ref[i] - probe[i]
        sum += diff * diff
    }
    distance = float32(math.Sqrt(float64(sum)))  // Euclidean distance

    // Normalize to [0, 1] confidence score
    // 0 distance   → 1.0 confidence (perfect match)
    // threshold    → 0.0 confidence (at the boundary)
    // > threshold  → 0.0 confidence (no match)
    if distance >= threshold {
        confidence = 0
    } else {
        confidence = 1 - (distance / threshold)
    }
    match = distance < threshold
    return
}
```

**About the threshold:** The default of `0.6` is the standard dlib recommendation for its ResNet face recognition model. A smaller threshold (e.g., `0.4`) is more strict (fewer false positives, more false negatives). A larger threshold is more permissive. Callers can override it per-request.

---

### 5.8 Image Quality Scoring

When `quality_checks: true` is set, the service evaluates the image before accepting the embedding:

```go
func checkQuality(img image.Image, faceRect image.Rectangle) (score, sharpness, brightness, faceSize float32, warnings []string) {

    // Face size ratio: face area / total image area
    faceSize = float32(faceArea) / float32(imgArea)
    if faceSize < 0.15 { warnings = append(warnings, "face_too_small") }

    // Brightness: average luminance across all pixels
    brightness = computeAverageBrightness(img)
    if brightness < 60  { warnings = append(warnings, "too_dark") }
    if brightness > 200 { warnings = append(warnings, "too_bright") }

    // Sharpness: Laplacian variance (gradient magnitude sampled every 4px)
    sharpness = calculateSharpness(img)
    if sharpness < 100 { warnings = append(warnings, "too_blurry") }

    // Overall score: 1.0 if clean, 0.7 if any warnings
    score = 1.0
    if len(warnings) > 0 { score = 0.7 }
    return
}
```

| Warning          | Trigger            | Meaning                                                        |
| ---------------- | ------------------ | -------------------------------------------------------------- |
| `face_too_small` | `faceSize < 0.15`  | Face occupies less than 15% of the image — too far from camera |
| `too_dark`       | `brightness < 60`  | Poor lighting — recognition accuracy degraded                  |
| `too_bright`     | `brightness > 200` | Overexposed — facial features washed out                       |
| `too_blurry`     | `sharpness < 100`  | Motion blur or out-of-focus — embeddings unreliable            |

The sharpness calculation uses a **Sobel gradient approximation** sampled at every 4th pixel for performance — a full pixel iteration on a high-resolution image would be unacceptably slow:

```go
// Sample every 4th pixel for performance
for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y += 4 {
    for x := bounds.Min.X + 1; x < bounds.Max.X-1; x += 4 {
        gx := grayDiff(img, x+1, y, x-1, y)   // horizontal gradient
        gy := grayDiff(img, x, y+1, x, y-1)   // vertical gradient
        mag := math.Sqrt(gx*gx + gy*gy)
        sum += mag; sumSq += mag*mag; count++
    }
}
variance := (sumSq / float64(count)) - (mean * mean)
normalized := variance / 1000.0  // cap at 1.0
```

---

## 6. Dockerfile — Multi-Stage Build _(New — 2026-03-03)_

The `asguard-face/Dockerfile` was redesigned as a **two-stage build**. This is one of the most important Docker optimizations available for CGO-dependent Go applications.

```dockerfile
# ---- Builder stage ----
FROM golang:1.25-bookworm AS builder

# Install C/C++ BUILD dependencies (includes headers, static libs, etc.)
RUN apt-get update && apt-get install -y \
    libdlib-dev \
    libatlas-base-dev \
    libjpeg62-turbo-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download          # Download dependencies first (layer cache)
COPY main.go .
RUN CGO_ENABLED=1 GOOS=linux go build -o face-service main.go


# ---- Runtime stage ----
FROM debian:bookworm-slim    # Minimal base — no Go toolchain, no dev headers

# Install RUNTIME ONLY libraries (no -dev packages, no headers)
RUN apt-get update && apt-get install -y \
    libdlib19.1 \            # dlib shared library (runtime only)
    libopenblas0 \           # BLAS math (dlib dependency)
    liblapack3 \             # LAPACK linear algebra (dlib dependency)
    libjpeg62-turbo \        # JPEG decoding (runtime only)
    ca-certificates \        # TLS root certificates
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/face-service /app/face-service   # Copy only the binary

CMD ["/app/face-service"]
```

### Why Multi-Stage?

|                        | Single Stage                    | Multi-Stage                    |
| ---------------------- | ------------------------------- | ------------------------------ |
| Base image             | `golang:1.25-bookworm` (~900MB) | `debian:bookworm-slim` (~80MB) |
| Dev tools in runtime   | Yes (gcc, make, Go toolchain)   | **No**                         |
| `-dev` header packages | Yes                             | **No**                         |
| Final image size       | ~2.5GB                          | **~300MB**                     |
| Attack surface         | Large                           | **Minimal**                    |

**Stage 1 (builder):** Installs all C development headers and the full Go toolchain. Compiles the binary with `CGO_ENABLED=1` so it links against the native dlib shared library.

**Stage 2 (runtime):** Starts fresh from a slim Debian base. Installs only the runtime `.so` shared libraries that the compiled binary needs at execution time. Copies just the compiled binary from Stage 1. The result is a production-grade minimal image.

---

## 7. Docker Compose — Multi-Service Orchestration

The root `docker-compose.yml` brings up the entire platform:

```yaml
version: "3.8"
services:
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    ports:
      - "8081:8081"
    environment:
      - ASGUARD_API_KEY=devsecret
      - PORT=8081

  asguard-face:
    build: ./asguard-face # Uses the multi-stage Dockerfile
    ports:
      - "8082:8082"
    environment:
      - PORT=8082
      - MODELS_PATH=/app/models
      - API_KEYS=${FACE_API_KEYS:-dev-key-123} # Falls back to dev-key-123
    volumes:
      - ./models:/app/models:ro # Mount models as read-only
    networks:
      - asguard-network

networks:
  asguard-network:
    driver: bridge
```

**Key design decisions:**

- `./models:/app/models:ro` — The dlib model files (~100MB each) are mounted from the host, not baked into the image. This keeps the image lean and allows model updates without a full Docker rebuild.
- `API_KEYS=${FACE_API_KEYS:-dev-key-123}` — Docker Compose reads `FACE_API_KEYS` from the host environment (or a `.env` file). The `:-dev-key-123` syntax provides a safe dev fallback so the service always starts even without an `.env` file.
- Both services share `asguard-network`, enabling them to call each other by service name (e.g., `http://asguard-face:8082`) if needed in the future.

---

## 8. Backend — Bugs Fixed & Improvements

### Bug 1 — 400 Bad Request on `/analyze`

`Timestamp` was marked `binding:"required"` but clients didn't send it. **Fix:** Made it optional.

### Bug 2 — Location field silently dropped

`Location` wasn't in `TransactionRequest`, so Gin discarded it. The risk engine always penalized missing location. **Fix:** Added `Location` to both the struct and the `TransactionData` mapping.

### Bug 3 — Scoring weights exceeded 100%

Weights summed to `1.2`, so max score was 120, not 100. **Fix:** Rebalanced to `0.35 + 0.20 + 0.15 + 0.15 + 0.15 = 1.0`.

### Bug 4 — Deprecated Groq model

`mixtral-8x7b-32768` was removed by Groq. **Fix:** Updated to `llama-3.3-70b-versatile`.

### Bug 5 — AI errors silently swallowed

AI failures produced no log output and the caller received a generic response. **Fix:** Added `log.Printf` at every stage: `[AI GATE]`, `[AI ERROR]`, `[AI OK]`.

### Improvement — Tiered Amount Scoring

Amount was binary (`> 100k` = full risk). **Fix:** Added three tiers: `> 50k` = 0.3, `> 100k` = 0.6, `> 500k` = 1.0.

### Improvement — AI Gate at 40 (was 50)

AI was only triggered at MEDIUM/HIGH boundary (50). Lowered to 40 so borderline MEDIUM transactions also get AI analysis.

### Improvement — AI Influences Final Risk Level

AI recommendation now actively upgrades the risk level (`BLOCK` → `HIGH`, `REVIEW` → upgrade `LOW` to `MEDIUM`). Previously it was stored but never applied.

---

## 9. Face Service — Changes on 2026-03-03

### CORS Middleware Added

**Problem:** Any web browser calling `/v1/analyze` or `/v1/compare` directly would receive a CORS policy error because the server returned no `Access-Control-Allow-Origin` header.

**Solution:** Integrated `github.com/gin-contrib/cors` and registered it as the first middleware in the chain (before auth), so `OPTIONS` preflight requests succeed:

```go
import "github.com/gin-contrib/cors"

r.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"*"},
    AllowMethods:     []string{"GET", "POST", "OPTIONS"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
    ExposeHeaders:    []string{"Content-Length"},
    AllowCredentials: true,
}))
```

**Why `Authorization` must be in `AllowHeaders`:** The browser's CORS preflight check (`OPTIONS`) includes a list of the headers the actual request will send. If `Authorization` is not in the server's `Access-Control-Allow-Headers` response, the browser aborts the request before it reaches the server — the `Bearer` token is never sent, and clients receive a CORS error instead of a 401.

### Dockerfile Converted to Multi-Stage Build

**Problem:** The original single-stage Dockerfile used `golang:1.25-bookworm` as its runtime base, bundling the entire Go compiler, build tools, and `-dev` header packages (~2.5GB) into the production image.

**Solution:** Separated build and runtime into two stages. Final production image is `debian:bookworm-slim` containing only the compiled binary and its runtime `.so` dependencies (~300MB reduction).

---

## 10. How to Test Both APIs

### Start Everything

```bash
# From the project root
docker compose up --build
```

### Test Backend

```bash
# Health check (no key needed)
curl http://localhost:8081/health

# Analyze a transaction
curl -X POST http://localhost:8081/analyze \
  -H "x-api-key: devsecret" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "u1", "transaction_id": "t1",
    "amount": 250000, "currency": "USD",
    "ip_address": "1.2.3.4", "device_id": "d1",
    "location": "Lagos, Nigeria"
  }'
```

### Test Face Service

```bash
# Health check (no key needed)
curl http://localhost:8082/health

# Extract embedding from an image
curl -X POST http://localhost:8082/v1/analyze \
  -H "Authorization: Bearer dev-key-123" \
  -H "Content-Type: application/json" \
  -d '{
    "image": "<base64-encoded-jpeg>",
    "quality_checks": true
  }'

# Compare probe image to reference embedding
curl -X POST http://localhost:8082/v1/compare \
  -H "Authorization: Bearer dev-key-123" \
  -H "Content-Type: application/json" \
  -d '{
    "probe_image": "<base64-encoded-jpeg>",
    "reference_embedding": [0.023, -0.14, ...],
    "threshold": 0.6
  }'
```

### Score Breakdown Reference (Backend)

| Rule       | Example Value | Risk | Weight | Contribution                       |
| ---------- | ------------- | ---- | ------ | ---------------------------------- |
| Amount     | 250,000 USD   | 0.6  | 35%    | 21 pts                             |
| Currency   | USD (foreign) | 1.0  | 20%    | 20 pts                             |
| Device ID  | present       | 0.0  | 15%    | 0 pts                              |
| IP Address | present       | 0.0  | 15%    | 0 pts                              |
| Location   | present       | 0.0  | 15%    | 0 pts                              |
| **Total**  |               |      |        | **41 pts → MEDIUM → AI triggered** |
