# Asguard Backend — Architecture, Code Walkthrough & Change Log

> **Last updated:** 2026-02-18  
> This document explains how the project works, how every part communicates, what bugs were fixed, and what improvements were made — with code snippets throughout.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Folder Structure](#2-folder-structure)
3. [How a Request Flows Through the System](#3-how-a-request-flows-through-the-system)
4. [File-by-File Breakdown](#4-file-by-file-breakdown)
   - [main.go](#41-maingo--the-entry-point)
   - [middleware/apikey.go](#42-middlewareapikeygo--the-security-gate)
   - [routes/routes.go](#43-routesroutesgo--the-http-layer)
   - [services/risk_engine.go](#44-servicesrisk_enginego--the-scoring-brain)
   - [services/ai_service.go](#45-servicesai_servicego--the-groq-ai-integration)
5. [Bugs Found and Fixed](#5-bugs-found-and-fixed)
   - [Bug 1 — 400 Bad Request on /analyze](#bug-1--400-bad-request-on-analyze)
   - [Bug 2 — Location field silently dropped](#bug-2--location-field-silently-dropped)
   - [Bug 3 — Scoring weights exceeded 100%](#bug-3--scoring-weights-exceeded-100)
   - [Bug 4 — Deprecated Groq model](#bug-4--deprecated-groq-model)
   - [Bug 5 — AI errors were silently swallowed](#bug-5--ai-errors-were-silently-swallowed)
6. [Improvements Made](#6-improvements-made)
   - [Tiered Amount Scoring](#tiered-amount-scoring)
   - [AI Gate Lowered to 40](#ai-gate-lowered-to-40)
   - [AI Can Influence Final Risk Level](#ai-can-influence-final-risk-level)
   - [Structured Prompting with System + User Roles](#structured-prompting-with-system--user-roles)
   - [Markdown Fence Stripping](#markdown-fence-stripping)
   - [Full AI Fields Exposed in API Response](#full-ai-fields-exposed-in-api-response)
7. [How to Test the API](#7-how-to-test-the-api)

---

## 1. Project Overview

**Asguard** is a fraud detection backend API built in Go. When a financial transaction comes in, the system:

1. Validates the request and checks the caller's API key
2. Runs a **rule-based scoring engine** that assigns a risk score (0–100)
3. If the score is high enough (≥ 40), calls **Groq AI** (an LLM) for a second opinion
4. Returns a structured JSON response with the score, risk level, reasons, and AI analysis

The stack is:

- **Go** — language
- **Gin** — HTTP web framework
- **Groq API** — LLM provider (using `llama-3.3-70b-versatile`)
- **godotenv** — loads `.env` secrets at startup

---

## 2. Folder Structure

```
backend/
├── main.go                    ← Entry point. Starts the server.
├── .env                       ← Secret keys (never commit this to git)
├── middleware/
│   └── apikey.go              ← API key authentication middleware
├── routes/
│   └── routes.go              ← HTTP route definitions and request handling
└── services/
    ├── risk_engine.go         ← Rule-based scoring logic + AI gate
    └── ai_service.go          ← Groq AI HTTP client
```

---

## 3. How a Request Flows Through the System

Here is the full journey of a single `POST /analyze` request:

```
Client (Postman / Frontend)
        │
        │  POST /analyze
        │  Header: x-api-key: supersecret123
        │  Body: { "user_id": ..., "amount": 250000, ... }
        ▼
┌─────────────────────────────┐
│       main.go               │  ← Starts Gin, loads .env, registers routes
└────────────┬────────────────┘
             │
             ▼
┌─────────────────────────────┐
│  middleware/apikey.go       │  ← Checks x-api-key header
│  APIKeyAuth()               │    If wrong/missing → 401 Unauthorized
└────────────┬────────────────┘
             │  (key is valid, continue)
             ▼
┌─────────────────────────────┐
│  routes/routes.go           │  ← Parses + validates JSON body
│  AnalyzeTransaction()       │    If missing required fields → 400 Bad Request
└────────────┬────────────────┘
             │  (request is valid)
             ▼
┌─────────────────────────────┐
│  services/risk_engine.go    │  ← Runs 5 weighted rules → score 0–100
│  CalculateRisk()            │    Determines risk level: LOW / MEDIUM / HIGH
└────────────┬────────────────┘
             │  (if score >= 40)
             ▼
┌─────────────────────────────┐
│  services/ai_service.go     │  ← Calls Groq API with transaction details
│  AnalyzeTransaction()       │    Gets back: fraud_probability, action, reasoning
└────────────┬────────────────┘
             │
             ▼
        JSON Response
        { risk_score, risk_level, ai_recommendation, ... }
```

---

## 4. File-by-File Breakdown

### 4.1 `main.go` — The Entry Point

This is where the application boots. It does three things:

```go
func main() {
    // 1. Load .env file so os.Getenv("GROQ_API_KEY") works everywhere
    if err := godotenv.Load(); err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }

    // 2. Create the Gin HTTP router
    router := gin.Default()

    // 3. Register all routes (health check + /analyze)
    routes.RegisterRoutes(router)

    // 4. Start listening on port 8081
    router.Run(":8081")
}
```

**Key point:** `godotenv.Load()` must run before anything else, because `ai_service.go` reads `GROQ_API_KEY` from the environment. If this fails, the whole app crashes intentionally — you don't want to run without secrets.

---

### 4.2 `middleware/apikey.go` — The Security Gate

Every protected route passes through this middleware before the handler runs. It reads the `x-api-key` header and compares it to the value stored in `.env`.

```go
func APIKeyAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        apikey := c.GetHeader("x-api-key")         // read from request header
        expectedKey := os.Getenv("ASGUARD_API_KEY") // read from .env

        if apikey == "" || apikey != expectedKey {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorised"})
            c.Abort() // ← stops the request here, handler never runs
            return
        }

        c.Next() // ← key is valid, pass through to the route handler
    }
}
```

**How it connects:** In `routes.go`, this middleware is attached to a route group:

```go
protected := router.Group("/")
protected.Use(middleware.APIKeyAuth()) // ← applied to all routes in this group
protected.POST("/analyze", AnalyzeTransaction)
```

So the middleware runs **before** `AnalyzeTransaction`. If the key is wrong, the request never reaches the handler.

---

### 4.3 `routes/routes.go` — The HTTP Layer

This file has two jobs:

1. **Define the shape of incoming requests** via `TransactionRequest`
2. **Handle the request** — validate it, call the service, return the response

#### The Request Struct

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

The `binding:"required"` tag tells Gin to reject the request with a 400 if that field is missing from the JSON body. Fields without it are optional.

#### The Handler

```go
func AnalyzeTransaction(c *gin.Context) {
    var req TransactionRequest

    // Try to parse the JSON body into the struct
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request payload"})
        return
    }

    // Pass the validated data to the service layer
    riskResult := services.CalculateRisk(services.TransactionData{
        UserID:        req.UserID,
        TransactionID: req.TransactionID,
        Amount:        req.Amount,
        Currency:      req.Currency,
        IPAddress:     req.IPAddress,
        DeviceID:      req.DeviceID,
        Location:      req.Location,
    })

    // Return the full result to the caller
    c.JSON(200, gin.H{
        "transaction_id":       req.TransactionID,
        "risk_score":           riskResult.Score,
        "risk_level":           riskResult.Level,
        "reasons":              riskResult.Reasons,
        "ai_triggered":         riskResult.AITriggered,
        "ai_confidence":        riskResult.AIConfidence,
        "ai_recommendation":    riskResult.AIRecommendation,
        "ai_fraud_probability": riskResult.AIFraudProbability,
        "ai_summary":           riskResult.AISummary,
        "message":              "Transaction analyzed successfully",
    })
}
```

**Key point:** The route layer does NOT do any business logic. It only handles HTTP concerns (parsing, responding). All the actual risk logic lives in `services/`.

---

### 4.4 `services/risk_engine.go` — The Scoring Brain

This is the core of the system. It takes a `TransactionData` struct and returns a `RiskResult`.

#### The Data Types

```go
// Input — what the route layer sends us
type TransactionData struct {
    UserID        string
    TransactionID string
    Amount        float64
    Currency      string
    IPAddress     string
    DeviceID      string
    Location      string
}

// Output — what we return to the route layer
type RiskResult struct {
    Score              int      // 0–100 integer
    Level              string   // "LOW", "MEDIUM", or "HIGH"
    Reasons            []string // human-readable list of why the score is what it is
    AITriggered        bool     // was the AI called?
    AIConfidence       float64  // how confident was the AI (0.0–1.0)
    AISummary          string   // AI's one-sentence reasoning
    AIRecommendation   string   // "APPROVE", "REVIEW", or "BLOCK"
    AIFraudProbability float64  // AI's estimated fraud probability (0.0–1.0)
}
```

#### The Scoring Rules

Each rule produces a risk value between `0.0` and `1.0`. Each rule has a weight. **All weights sum to exactly 1.0**, so the final score is a true percentage.

```go
// Rule 1: Amount (35% of total score) — tiered, not binary
switch {
case tx.Amount > 500000:
    amountRisk = 1.0   // full risk
case tx.Amount > 100000:
    amountRisk = 0.6   // high risk
case tx.Amount > 50000:
    amountRisk = 0.3   // moderate risk
}

// Rule 2: Currency (20%) — non-NGN = foreign = riskier
if tx.Currency != "NGN" {
    currencyRisk = 1.0
}

// Rule 3: Device ID (15%) — missing device = anonymous sender
if tx.DeviceID == "" {
    deviceRisk = 1.0
}

// Rule 4: IP Address (15%) — missing IP = untraceable
if tx.IPAddress == "" {
    ipRisk = 1.0
}

// Rule 5: Location (15%) — missing location = unverifiable origin
if tx.Location == "" {
    locationRisk = 1.0
}

// Final weighted score (0.35 + 0.20 + 0.15 + 0.15 + 0.15 = 1.0)
score := (amountRisk * 0.35) +
         (currencyRisk * 0.20) +
         (deviceRisk * 0.15) +
         (ipRisk * 0.15) +
         (locationRisk * 0.15)

finalScore := int(score * 100) // e.g. 0.55 → 55
```

#### The AI Gate

After the rule-based score is calculated, the engine decides whether to call the AI:

```go
if finalScore >= 40 {
    // Score is MEDIUM or HIGH — get AI's second opinion
    aiTriggered = true
    log.Printf("[AI GATE] Score=%d for txn=%s — calling Groq AI...", finalScore, tx.TransactionID)

    result, err := AnalyzeTransaction(tx, finalScore)
    if err != nil {
        // AI failed — escalate to HIGH for safety, don't silently ignore
        log.Printf("[AI ERROR] txn=%s: %v", tx.TransactionID, err)
        level = "HIGH"
        reasons = append(reasons, "AI analysis unavailable — escalated to HIGH for manual review")
    } else {
        // AI succeeded — let it influence the final level
        switch result.RecommendedAction {
        case "BLOCK":
            level = "HIGH"
        case "REVIEW":
            if level == "LOW" { level = "MEDIUM" } // AI can upgrade, never downgrade
        case "APPROVE":
            // note the AI's opinion but keep rule-based level
        }
    }
}
```

---

### 4.5 `services/ai_service.go` — The Groq AI Integration

This file is responsible for sending transaction data to Groq's API and parsing the response.

#### The Prompt Strategy

The AI is given two messages — a **system prompt** (its role and output rules) and a **user prompt** (the actual transaction data):

```go
systemPrompt := `You are a financial fraud detection AI for a Nigerian fintech platform.
You MUST respond with ONLY a valid JSON object — no markdown, no explanation outside the JSON.
The JSON must follow this exact schema:
{
  "fraud_probability": <float between 0.0 and 1.0>,
  "recommended_action": <"APPROVE" | "REVIEW" | "BLOCK">,
  "reasoning": <one concise sentence explaining your decision>,
  "confidence": <float between 0.0 and 1.0>
}`

userPrompt := fmt.Sprintf(`Assess this transaction for fraud risk:
Transaction ID : %s
Amount         : %.2f %s
Location       : %s
Device ID      : %s
IP Address     : %s
Baseline Score : %d/100
Respond with JSON only.`, tx.TransactionID, tx.Amount, tx.Currency, ...)
```

#### The HTTP Call

```go
body := groqRequest{
    Model:       "llama-3.3-70b-versatile", // current active Groq model
    Temperature: 0.1,  // low = more deterministic output (important for JSON)
    MaxTokens:   256,  // we only need a small JSON blob
    Messages: []groqMessage{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: userPrompt},
    },
}

client := &http.Client{Timeout: 10 * time.Second}
resp, err := client.Do(httpReq)
```

#### Parsing the Response Safely

LLMs sometimes wrap their JSON in markdown code fences (` ```json ... ``` `). The code strips these before parsing:

````go
rawContent = strings.TrimPrefix(rawContent, "```json")
rawContent = strings.TrimPrefix(rawContent, "```")
rawContent = strings.TrimSuffix(rawContent, "```")
rawContent = strings.TrimSpace(rawContent)

var aiResult AIResult
json.Unmarshal([]byte(rawContent), &aiResult)
````

It also validates the action field is one of the expected values:

```go
switch aiResult.RecommendedAction {
case "APPROVE", "REVIEW", "BLOCK":
    // valid — continue
default:
    return AIResult{}, fmt.Errorf("AI returned unexpected action: %q", aiResult.RecommendedAction)
}
```

---

## 5. Bugs Found and Fixed

### Bug 1 — 400 Bad Request on `/analyze`

**Problem:** The `TransactionRequest` struct had `Timestamp` marked as `binding:"required"`:

```go
// BEFORE (broken)
Timestamp string `json:"timestamp" binding:"required"`
```

The test JSON payload did not include a `timestamp` field. Gin's `ShouldBindJSON` failed validation and returned 400 immediately — before any business logic ran.

**Fix:** Made `Timestamp` optional by removing the `binding:"required"` tag:

```go
// AFTER (fixed)
Timestamp string `json:"timestamp"` // optional
```

---

### Bug 2 — Location field silently dropped

**Problem:** The `TransactionRequest` struct had no `Location` field at all. Even though the client sent `"location": "Lagos, Nigeria"`, Gin silently ignored it. The `risk_engine.go` then always saw `tx.Location == ""` and always added a location risk penalty.

**Fix:** Added `Location` to both the request struct and the mapping to `TransactionData`:

```go
// In TransactionRequest struct
Location string `json:"location"` // added

// In the handler, passing it through
riskResult := services.CalculateRisk(services.TransactionData{
    ...
    Location: req.Location, // added
})
```

---

### Bug 3 — Scoring weights exceeded 100%

**Problem:** The original weights added up to **1.2 (120%)**, not 1.0:

```go
// BEFORE (broken — sums to 1.2)
amountWeight   := 0.4
currencyWeight := 0.2
deviceWeight   := 0.2
ipWeight       := 0.2
locationWeight := 0.2
// Total: 0.4 + 0.2 + 0.2 + 0.2 + 0.2 = 1.2
```

This meant the maximum possible score was 120, not 100. The score was not a true percentage.

**Fix:** Rebalanced weights to sum to exactly 1.0:

```go
// AFTER (fixed — sums to 1.0)
// Amount: 35%, Currency: 20%, Device: 15%, IP: 15%, Location: 15%
score := (amountRisk * 0.35) +
         (currencyRisk * 0.20) +
         (deviceRisk * 0.15) +
         (ipRisk * 0.15) +
         (locationRisk * 0.15)
// Total: 0.35 + 0.20 + 0.15 + 0.15 + 0.15 = 1.0 ✓
```

---

### Bug 4 — Deprecated Groq model

**Problem:** The original code used `mixtral-8x7b-32768`, which Groq has deprecated. Calls to it return an API error, which was silently swallowed.

```go
// BEFORE (broken — model no longer exists on Groq)
Model: "mixtral-8x7b-32768",
```

**Fix:** Updated to Groq's current recommended model:

```go
// AFTER (fixed)
Model: "llama-3.3-70b-versatile",
```

---

### Bug 5 — AI errors were silently swallowed

**Problem:** When the AI call failed (wrong model, network error, bad key), the original code did this:

```go
// BEFORE — error is caught but there's no logging
result, err := AnalyzeTransaction(tx, finalScore)
if err == nil {
    aiResult = result
} else {
    level = "HIGH"
    reasons = append(reasons, "AI unavailable: transaction blocked pending manual review")
    aiResult.Confidence = 0
}
```

There was no `log.Printf` anywhere. You had no way to know _why_ the AI wasn't working.

**Fix:** Added explicit logging at every stage:

```go
// AFTER — every outcome is logged
log.Printf("[AI GATE] Score=%d for txn=%s — calling Groq AI...", finalScore, tx.TransactionID)

result, err := AnalyzeTransaction(tx, finalScore)
if err != nil {
    log.Printf("[AI ERROR] txn=%s: %v", tx.TransactionID, err) // ← tells you exactly what went wrong
    level = "HIGH"
    reasons = append(reasons, "AI analysis unavailable — escalated to HIGH for manual review")
} else {
    log.Printf("[AI OK] txn=%s confidence=%.2f action=%s", tx.TransactionID, result.Confidence, result.RecommendedAction)
    aiResult = result
}
```

---

## 6. Improvements Made

### Tiered Amount Scoring

**Before:** Amount was binary — either risky or not:

```go
// BEFORE — only one threshold
if tx.Amount > 100000 {
    amountRisk = 1.0
}
```

**After:** Three tiers that reflect real-world risk gradation:

```go
// AFTER — graduated risk
switch {
case tx.Amount > 500000:
    amountRisk = 1.0   // extreme
case tx.Amount > 100000:
    amountRisk = 0.6   // high
case tx.Amount > 50000:
    amountRisk = 0.3   // moderate
}
```

---

### AI Gate Lowered to 40

**Before:** AI was only called at score ≥ 50 (HIGH territory). This meant MEDIUM-risk transactions never got AI analysis.

**After:** AI is called at score ≥ 40, which covers all MEDIUM and HIGH transactions:

```go
if finalScore >= 40 { // was >= 50
    // call AI
}
```

---

### AI Can Influence Final Risk Level

**Before:** The AI result was stored but never actually changed the `level` variable. It was purely informational.

**After:** The AI's `recommended_action` can upgrade the risk level:

```go
switch result.RecommendedAction {
case "BLOCK":
    level = "HIGH"                    // AI overrides to HIGH
case "REVIEW":
    if level == "LOW" {
        level = "MEDIUM"              // AI upgrades LOW → MEDIUM
    }
case "APPROVE":
    // AI agrees it's safe, but we don't downgrade the rule-based level
}
```

---

### Structured Prompting with System + User Roles

**Before:** A single combined prompt was sent as a `user` message. This gives the model less context about its role.

**After:** Split into a `system` message (role + output format) and a `user` message (transaction data). This produces more reliable, consistent JSON output:

```go
Messages: []groqMessage{
    {Role: "system", Content: systemPrompt}, // role definition
    {Role: "user",   Content: userPrompt},   // transaction data
},
```

---

### Markdown Fence Stripping

LLMs sometimes wrap their JSON output in markdown code fences even when told not to. Added defensive stripping:

````go
rawContent = strings.TrimPrefix(rawContent, "```json")
rawContent = strings.TrimPrefix(rawContent, "```")
rawContent = strings.TrimSuffix(rawContent, "```")
rawContent = strings.TrimSpace(rawContent)
````

---

### Full AI Fields Exposed in API Response

**Before:** The response only returned `ai_confidence`. The AI's recommendation and fraud probability were computed but never sent back to the caller.

**After:** All AI fields are returned:

```go
c.JSON(200, gin.H{
    "transaction_id":       req.TransactionID,
    "risk_score":           riskResult.Score,
    "risk_level":           riskResult.Level,
    "reasons":              riskResult.Reasons,
    "ai_triggered":         riskResult.AITriggered,         // was AI called?
    "ai_confidence":        riskResult.AIConfidence,        // 0.0–1.0
    "ai_recommendation":    riskResult.AIRecommendation,    // APPROVE/REVIEW/BLOCK
    "ai_fraud_probability": riskResult.AIFraudProbability,  // 0.0–1.0
    "ai_summary":           riskResult.AISummary,           // one-sentence reasoning
    "message":              "Transaction analyzed successfully",
})
```

---

## 7. How to Test the API

### Step 1 — Start the server

```bash
cd backend
go run main.go
```

### Step 2 — Health check (no API key needed)

```
GET http://localhost:8081/health
```

Expected response:

```json
{ "status": "asguard health running" }
```

### Step 3 — Analyze a transaction

```
POST http://localhost:8081/analyze
Header: x-api-key: supersecret123
Content-Type: application/json
```

Body:

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

Expected response (with AI triggered):

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

### Score Breakdown for the Test Payload

| Rule       | Value         | Risk | Weight | Contribution |
| ---------- | ------------- | ---- | ------ | ------------ |
| Amount     | 250,000 USD   | 0.6  | 35%    | 21 pts       |
| Currency   | USD (foreign) | 1.0  | 20%    | 20 pts       |
| Device ID  | present       | 0.0  | 15%    | 0 pts        |
| IP Address | present       | 0.0  | 15%    | 0 pts        |
| Location   | present       | 0.0  | 15%    | 0 pts        |
| **Total**  |               |      |        | **41 pts**   |

Score = 41 → **MEDIUM** → AI is triggered (≥ 40 threshold)
