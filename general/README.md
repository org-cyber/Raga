# General Validation Engine

![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg) ![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791.svg) ![CEL](https://img.shields.io/badge/Rules-CEL_Engine-orange.svg) ![Port](https://img.shields.io/badge/port-8083-blueviolet.svg)

The **General Validation Engine** is a standalone, programmable rule engine that lets you define, store, and evaluate custom logic against arbitrary JSON inputs in real time. It uses **Google's Common Expression Language (CEL)** to evaluate rules and **PostgreSQL** as its persistent rule store — with every validation decision fully audited.

It is purpose-built as the fraud-agnostic sibling to the `backend` service: where the backend scores financial transactions with hardcoded weights, the General Engine lets you write any logic you need, for any domain, without touching code.

---

## Table of Contents

- [What Problem Does This Solve?](#what-problem-does-this-solve)
- [System Architecture](#system-architecture)
- [Request Flow](#request-flow)
- [Database Schema](#database-schema)
  - [Rules Table](#rules-table)
  - [Decisions Table](#decisions-table)
  - [Indexes](#indexes)
- [Rule System Deep Dive](#rule-system-deep-dive)
  - [Rule Fields](#rule-fields)
  - [Actions Explained](#actions-explained)
  - [Writing CEL Conditions](#writing-cel-conditions)
  - [Rule Evaluation Order](#rule-evaluation-order)
  - [Score Accumulation & Thresholds](#score-accumulation--thresholds)
- [Code Structure](#code-structure)
- [Setup & Running Locally](#setup--running-locally)
  - [Prerequisites](#prerequisites)
  - [1. Start the Database](#1-start-the-database)
  - [2. Run the Engine](#2-run-the-engine)
- [API Reference](#api-reference)
  - [Health Check](#health-check)
  - [Validate Input](#validate-input-post-v1validate)
  - [List Rules](#list-rules-get-v1rules)
  - [Create Rule](#create-rule-post-v1rules)
  - [Get Rule](#get-rule-get-v1rulesid)
  - [Update Rule](#update-rule-put-v1rulesid)
  - [Delete Rule](#delete-rule-delete-v1rulesid)
- [SQL Queries Reference](#sql-queries-reference)
- [Regenerating DB Code (sqlc)](#regenerating-db-code-sqlc)
- [Adding New Rules: Walkthrough](#adding-new-rules-walkthrough)

---

## What Problem Does This Solve?

Traditional fraud logic is hardcoded. When business rules change (and they always do), engineers must modify code, review, test, redeploy — a cycle that takes hours or days.

The General Engine decouples logic from code:

```
Business Analyst writes a rule  →  POST /v1/rules
Engine stores it in PostgreSQL  →  immediately active
Next request evaluated against it  →  zero redeploy
```

**Use case examples:**

| Domain             | Context          | Example Rule Condition                                  |
| ------------------ | ---------------- | ------------------------------------------------------- |
| Login security     | `user_login`     | `input.failed_attempts > 5`                             |
| API rate limiting  | `api_request`    | `input.requests_per_minute > 100`                       |
| Content moderation | `content_submit` | `input.spam_score > 0.8`                                |
| Payment fraud      | `payment`        | `input.amount > 50000 && input.country == "XX"`         |
| KYC checks         | `kyc_verify`     | `input.document_type == "passport" && input.age >= 18`  |

---

## System Architecture

```mermaid
flowchart TD
    Client["Client App / Service"] -->|"POST /v1/validate\n{context, input:{}}"| Server

    subgraph General["General Validation Engine (Port 8083)"]
        Server["HTTP Server\n(net/http)"]
        Server --> RuleEngine["Rule Engine\n(engine.go)"]
        RuleEngine --> DB1[("PostgreSQL\nFetch active rules\nfor this context")]
        DB1 --> Evaluator["CEL Evaluator\n(evaluator.go)"]
        Evaluator -->|"rule matches?"| DecisionTree["Decision Logic\nallow / block / challenge\n/ score / flag"]
        DecisionTree --> DB2[("PostgreSQL\nWrite decision\nto audit log")]
    end

    Server -->|"ValidationResult"| Client

    Admin["Admin / Dashboard"] -->|"CRUD /v1/rules"| Server
    Server <-->|"Read/Write Rules"| DB1
```

---

## Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant S as HTTP Server
    participant RE as Rule Engine
    participant DB as PostgreSQL
    participant CEL as CEL Evaluator

    C->>S: POST /v1/validate {context:"user_login", input:{...}}
    S->>S: Validate required fields
    S->>RE: Validate(ctx, request)
    RE->>DB: ListRulesByContext(tenant, "user_login")
    DB-->>RE: [Rule1(priority:100), Rule2(priority:50), Rule3(priority:0)]

    loop For each rule, highest priority first
        RE->>CEL: CompileRule(rule.condition)
        CEL-->>RE: cel.Program
        RE->>CEL: Evaluate(program, input)
        CEL-->>RE: matched: true/false

        alt matched && action == "block"
            RE-->>S: Decision="block", stop evaluation
        else matched && action == "score"
            RE->>RE: Accumulate score
        else matched && action == "allow"
            RE-->>S: Decision="allow", stop evaluation
        end
    end

    RE->>DB: CreateDecision (audit log)
    RE-->>S: ValidationResult
    S-->>C: 200 OK {decision, score, reason, rules_matched}
```

---

## Database Schema

The engine uses two PostgreSQL tables. The schema is in [`schema.sql`](schema.sql) and is automatically applied when you start the database via Docker.

### Rules Table

```sql
CREATE TABLE rules (
    id        UUID    PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID    NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    name      TEXT    NOT NULL,
    context   TEXT    NOT NULL,       -- e.g. 'user_login', 'payment'
    condition TEXT    NOT NULL,       -- CEL expression: 'input.amount > 1000'
    action    TEXT    NOT NULL CHECK (action IN ('allow','block','challenge','flag','score')),
    score     INTEGER,                -- points to add (only for action='score')
    priority  INTEGER NOT NULL DEFAULT 0,   -- higher = evaluated first
    enabled   BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);
```

**Entity Diagram:**

```mermaid
erDiagram
    RULES {
        UUID id PK
        UUID tenant_id
        TEXT name
        TEXT context
        TEXT condition
        TEXT action
        INTEGER score
        INTEGER priority
        BOOLEAN enabled
        TIMESTAMP created_at
        TIMESTAMP updated_at
    }

    DECISIONS {
        UUID id PK
        UUID tenant_id
        TEXT context
        JSONB input
        UUID[] matched_rules
        TEXT decision
        INTEGER final_score
        TEXT reason
        TIMESTAMP processed_at
        INTEGER processing_time_ms
        TEXT engine_version
    }

    RULES ||--o{ DECISIONS : "matched by"
```

### Decisions Table

Every validation call — whether it results in allow, block, or challenge — is written to the `decisions` table. This creates a full audit trail.

```sql
CREATE TABLE decisions (
    id                UUID      PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID      NOT NULL,
    context           TEXT      NOT NULL,
    input             JSONB     NOT NULL,   -- the full input payload
    matched_rules     UUID[],               -- which rules fired
    decision          TEXT      NOT NULL CHECK (decision IN ('allow','block','challenge')),
    final_score       INTEGER,
    reason            TEXT,
    processed_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processing_time_ms INTEGER  NOT NULL,
    engine_version    TEXT      NOT NULL DEFAULT '1.0.0'
);
```

### Indexes

```sql
-- Fast rule lookup: tenant + context + enabled + priority (the hot path)
CREATE INDEX idx_rules_lookup ON rules(tenant_id, context, enabled, priority DESC);

-- Fast audit queries: recent decisions per tenant+context
CREATE INDEX idx_decisions_recent ON decisions(tenant_id, context, processed_at DESC);

-- JSONB GIN index: search decisions by input field values
CREATE INDEX idx_decisions_input ON decisions USING gin(input);
```

---

## Rule System Deep Dive

### Rule Fields

| Field       | Type      | Required | Description                                                                 |
| ----------- | --------- | -------- | --------------------------------------------------------------------------- |
| `name`      | `string`  | ✅        | Unique human-readable identifier within a tenant. e.g. `"block-tor-ips"`    |
| `context`   | `string`  | ✅        | The scenario this rule applies to. e.g. `"user_login"`, `"payment"`         |
| `condition` | `string`  | ✅        | A **CEL expression** evaluated against `input`. e.g. `"input.score > 80"`  |
| `action`    | `string`  | ✅        | What to do when matched: `allow`, `block`, `challenge`, `flag`, or `score`  |
| `score`     | `integer` | ❌        | Points to add when `action == "score"`. Ignored otherwise.                  |
| `priority`  | `integer` | ❌        | Evaluation order. Higher number = evaluated **first**. Default: `0`          |
| `enabled`   | `boolean` | ❌        | `true` = rule is active. `false` = soft-disabled. Default: `true`           |

### Actions Explained

```mermaid
flowchart LR
    Match["Rule Matched"] --> A{Action}
    A -->|allow| AL["✅ ALLOW\nStop evaluation immediately\nReturn allow decision"]
    A -->|block| BL["🚫 BLOCK\nStop evaluation immediately\nReturn block decision"]
    A -->|challenge| CH["⚠️ CHALLENGE\nContinue evaluation\nMinimum outcome is challenge"]
    A -->|score| SC["📊 SCORE\nAdd 'score' points to total\nContinue evaluation"]
    A -->|flag| FL["🚩 FLAG\nLog + continue evaluation\nNo decision change"]
```

| Action      | Stops evaluation? | Effect                                                                          |
| ----------- | :---------------: | ------------------------------------------------------------------------------- |
| `allow`     | ✅ Yes             | Immediately returns `allow`. No further rules checked.                          |
| `block`     | ✅ Yes             | Immediately returns `block`. No further rules checked.                          |
| `challenge` | ❌ No              | Marks result as at-least `challenge`. Evaluation continues.                     |
| `score`     | ❌ No              | Adds the rule's `score` value to a running total. Evaluation continues.         |
| `flag`      | ❌ No              | Logged in `rules_matched`. No decision change. Useful for monitoring.           |

### Writing CEL Conditions

All conditions are **Google CEL (Common Expression Language)** expressions. The available variable is `input` — a map containing whatever JSON you send in the `input` field of the validation request.

**Basic examples:**

```cel
# Check a numeric field
input.amount > 50000

# Check a string field
input.country == "NG"

# Check a boolean
input.is_vpn == true

# Combine conditions
input.failed_attempts > 3 && input.country != "US"

# String contains
input.email.contains("@tempmail.com")

# Check presence of a field
has(input.device_id)
```

**Advanced examples:**

```cel
# Block very high amounts from specific countries
input.amount > 1000000 && input.country in ["XX", "YY", "ZZ"]

# Score if amount is large AND currency is foreign
input.amount > 100000 && input.currency != "USD"

# Challenge if login is from a new device at unusual hour
input.new_device == true && input.hour_of_day < 6

# Flag if multiple failed attempts (for monitoring)
input.failed_attempts >= 2
```

> **Tip:** CEL is type-safe. If your `input.amount` is sent as a string `"5000"` but your condition uses `> 5000` (integer), the type check will fail. Always send the correct JSON types.

### Rule Evaluation Order

Rules are fetched from the database **sorted by `priority DESC`** — highest priority first. Within the same priority, order is not guaranteed.

```mermaid
flowchart TD
    Start([Start Validation]) --> R1["Rule: priority=100\nblock-vpn-users"]
    R1 -->|no match| R2["Rule: priority=50\nflag-large-amounts"]
    R2 -->|match → flag| R3["Rule: priority=10\nscore-foreign-currency"]
    R3 -->|match → +30 pts| R4["Rule: priority=0\nchallenge-new-device"]
    R4 -->|match → challenge| ScoreCheck{"Total score\n≥ 100?"}
    ScoreCheck -->|yes| BlockResult["Decision: BLOCK"]
    ScoreCheck -->|no, ≥50?| ChallengeResult["Decision: CHALLENGE"]
    ScoreCheck -->|no| AllowResult["Decision: ALLOW"]
```

### Score Accumulation & Thresholds

When rules with `action=score` match, their `score` values are added together. After all rules are evaluated:

| Cumulative Score | Auto-Decision       |
| :--------------: | ------------------- |
| `>= 100`         | `block`             |
| `>= 50`          | `challenge` (if currently `allow`) |
| `< 50`           | unchanged (default `allow`)        |

These thresholds are currently hardcoded in `engine.go`. You can adjust them to fit your risk tolerance.

---

## Code Structure

```text
general/
├── cmd/
│   └── server/
│       └── main.go         # HTTP server, all route handlers, CRUD for rules
│
├── internal/
│   ├── db/                 # Auto-generated by sqlc (DO NOT EDIT MANUALLY)
│   │   ├── db.go           # DBTX interface (pgx connection wrapper)
│   │   ├── models.go       # Go structs: Rule, Decision
│   │   ├── queries.go      # Querier interface
│   │   └── queries.sql.go  # All SQL query implementations
│   │
│   └── engine/
│       ├── engine.go       # RuleEngine, types (RuleRequest/Response), RuleToResponse,
│       │                   # RuleToResponse helper, Validate() method
│       └── evaluator.go    # CEL environment setup, CompileRule, Evaluate
│
├── schema.sql              # PostgreSQL DDL (tables + indexes)
├── queries.sql             # Named SQL queries (sqlc source)
├── sqlc.yaml               # sqlc code-generation config
├── docker-compose.yaml     # Postgres-only compose (for the engine's DB)
└── go.mod                  # Module: 'general'
```

**Key dependency flow:**

```mermaid
flowchart LR
    main["cmd/server/main.go\n(HTTP handlers)"]
    engine["internal/engine/engine.go\n(RuleEngine · types)"]
    evaluator["internal/engine/evaluator.go\n(CEL env)"]
    db["internal/db/\n(sqlc generated)"]
    pg[("PostgreSQL")]

    main --> engine
    main --> db
    engine --> evaluator
    engine --> db
    db --> pg
```

---

## Setup & Running Locally

### Prerequisites

| Tool        | Version  | Purpose                                  |
| ----------- | -------- | ---------------------------------------- |
| Go          | ≥ 1.21   | Build and run the engine                 |
| Docker      | any      | Run PostgreSQL (recommended)             |
| sqlc        | ≥ 1.25   | Regenerate DB code from SQL (if needed, see below) |

### 1. Start the Database

The `docker-compose.yaml` inside `general/` spins up a PostgreSQL 16 instance and automatically applies `schema.sql` on first start:

```bash
cd asguard/general
docker compose up -d
```

This starts Postgres on **port 5433** (not 5432, to avoid conflicts) with:

| Setting  | Value            |
| -------- | ---------------- |
| User     | `asguard`        |
| Password | `devpassword`    |
| Database | `general_engine` |

Verify it's running:

```bash
docker compose ps
# Should show: postgres   running   0.0.0.0:5433->5432/tcp
```

Connect with psql to inspect (optional):

```bash
psql "postgres://asguard:devpassword@localhost:5433/general_engine"
\dt   -- should show: rules, decisions
```

### 2. Run the Engine

```bash
cd asguard/general
go mod download
go run ./cmd/server/
```

Expected output:

```
2026/03/12 15:00:00 General Validation Engine starting on port 8083
```

The engine is now ready at `http://localhost:8083`.

---

## API Reference

### Health Check

```http
GET http://localhost:8083/health
```

**Response:**

```json
{
  "status": "ok",
  "engine": "general-validation"
}
```

---

### Validate Input — `POST /v1/validate`

The core endpoint. Evaluates all active rules for the given `context` against the provided `input`.

**Request:**

```http
POST http://localhost:8083/v1/validate
Content-Type: application/json
```

```json
{
  "context": "user_login",
  "input": {
    "failed_attempts": 7,
    "ip_address": "1.2.3.4",
    "is_vpn": true,
    "country": "NG"
  }
}
```

| Field     | Type   | Required | Description                                                        |
| --------- | ------ | :------: | ------------------------------------------------------------------ |
| `context` | string | ✅        | Which set of rules to evaluate. Must match rules in the database.   |
| `input`   | object | ✅        | Arbitrary JSON object. Values are accessed in CEL as `input.field`. |

**Response:**

```json
{
  "decision": "block",
  "score": 0,
  "reason": "Rule 'block-vpn-users' blocked: input.is_vpn == true",
  "rules_matched": ["block-vpn-users"],
  "processing_time_ms": 3
}
```

| Field                | Type     | Description                                                     |
| -------------------- | -------- | --------------------------------------------------------------- |
| `decision`           | string   | Final outcome: `allow`, `block`, or `challenge`                 |
| `score`              | integer  | Cumulative score from all matched `score`-action rules          |
| `reason`             | string   | Human-readable explanation of why the decision was made         |
| `rules_matched`      | []string | Names of all rules that matched (in evaluation order)           |
| `processing_time_ms` | integer  | Total time to evaluate all rules and write the audit log        |

---

### List Rules — `GET /v1/rules`

Returns all rules for the (currently hardcoded) tenant. Supports optional context filtering.

```http
GET http://localhost:8083/v1/rules
GET http://localhost:8083/v1/rules?context=user_login
```

**Response:**

```json
[
  {
    "id": "a1b2c3d4-...",
    "name": "block-vpn-users",
    "context": "user_login",
    "condition": "input.is_vpn == true",
    "action": "block",
    "score": null,
    "priority": 100,
    "enabled": true,
    "created_at": "2026-03-12T14:00:00Z",
    "updated_at": "2026-03-12T14:00:00Z"
  }
]
```

---

### Create Rule — `POST /v1/rules`

Creates a new rule. The rule becomes **effective immediately** on the next validation call.

```http
POST http://localhost:8083/v1/rules
Content-Type: application/json
```

```json
{
  "name": "score-large-amounts",
  "context": "payment",
  "condition": "input.amount > 100000",
  "action": "score",
  "score": 40,
  "priority": 50,
  "enabled": true
}
```

**Allowed `action` values:** `allow` · `block` · `challenge` · `flag` · `score`

**Response:** `201 Created` — the created rule object (same shape as list response).

---

### Get Rule — `GET /v1/rules/{id}`

```http
GET http://localhost:8083/v1/rules/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

**Response:** Single rule object, or `404 Not Found`.

---

### Update Rule — `PUT /v1/rules/{id}`

Updates all fields of an existing rule. The `context` field **can** be updated via this endpoint (the underlying SQL includes it in the `SET` clause).

```http
PUT http://localhost:8083/v1/rules/a1b2c3d4-e5f6-7890-abcd-ef1234567890
Content-Type: application/json
```

```json
{
  "name": "score-large-amounts",
  "context": "payment",
  "condition": "input.amount > 50000",
  "action": "score",
  "score": 60,
  "priority": 50,
  "enabled": true
}
```

**Response:** Updated rule object, or `500` if not found.

---

### Delete Rule — `DELETE /v1/rules/{id}`

Hard-deletes the rule. It will no longer be evaluated on future requests.

```http
DELETE http://localhost:8083/v1/rules/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

**Response:** `204 No Content` on success, `404 Not Found` if the ID doesn't exist.

---

## SQL Queries Reference

The file [`queries.sql`](queries.sql) defines all named SQL operations. These are compiled to Go by `sqlc`.

| Query Name             | Type     | Description                                                         |
| ---------------------- | -------- | ------------------------------------------------------------------- |
| `CreateRule`           | `:one`   | Insert a new rule, return it with generated ID and timestamps       |
| `GetRule`              | `:one`   | Fetch a single rule by `id` and `tenant_id`                         |
| `ListRulesByContext`   | `:many`  | **Hot path.** All enabled rules for a context, sorted by priority DESC |
| `ListAllRules`         | `:many`  | All rules (including disabled) for a tenant, sorted by context+priority |
| `ListRules`            | `:many`  | All rules with optional context filter (`$2` is a nullable text param) |
| `UpdateRule`           | `:one`   | Update name, context, condition, action, score, priority, enabled   |
| `DeleteRule`           | `:exec`  | Hard delete by `id` + `tenant_id`                                   |
| `CreateDecision`       | `:one`   | Write an audit log entry, return it with generated ID               |
| `ListDecisions`        | `:many`  | Paginated audit log for a tenant+context                            |

---

## Regenerating DB Code (sqlc)

If you modify `schema.sql` or `queries.sql`, regenerate the Go code:

```bash
# Install sqlc (if not already installed)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Run from the general/ directory
cd asguard/general
sqlc generate
```

> **Warning:** `internal/db/` is fully generated. Never edit those files manually — your changes will be overwritten on the next `sqlc generate`.

---

## Adding New Rules: Walkthrough

Here's an example of adding a login brute-force protection rule from scratch:

**Step 1 — Design your rule logic:**

> "If a user has had more than 5 failed login attempts in the current session, block them."

**Step 2 — Create the rule via API:**

```bash
curl -X POST http://localhost:8083/v1/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "block-brute-force",
    "context": "user_login",
    "condition": "input.failed_attempts > 5",
    "action": "block",
    "priority": 100,
    "enabled": true
  }'
```

**Step 3 — Test it with a validation call:**

```bash
# Should return "allow" (4 attempts, under threshold)
curl -X POST http://localhost:8083/v1/validate \
  -H "Content-Type: application/json" \
  -d '{"context":"user_login","input":{"failed_attempts":4}}'

# Should return "block" (6 attempts, over threshold)
curl -X POST http://localhost:8083/v1/validate \
  -H "Content-Type: application/json" \
  -d '{"context":"user_login","input":{"failed_attempts":6}}'
```

**Step 4 — Adjust priority or score as needed:**

```bash
# Add a warning-level score rule at lower priority (runs AFTER block rules)
curl -X POST http://localhost:8083/v1/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "score-suspicious-attempts",
    "context": "user_login",
    "condition": "input.failed_attempts >= 2",
    "action": "score",
    "score": 25,
    "priority": 50,
    "enabled": true
  }'
```

**Step 5 — Disable a rule without deleting it:**

```bash
curl -X PUT http://localhost:8083/v1/rules/<RULE_ID> \
  -H "Content-Type: application/json" \
  -d '{
    "name": "score-suspicious-attempts",
    "context": "user_login",
    "condition": "input.failed_attempts >= 2",
    "action": "score",
    "score": 25,
    "priority": 50,
    "enabled": false
  }'
```

The rule is now soft-disabled and will be skipped on future evaluations.

---

> For the full Asguard platform overview, see the [root README](../README.md) and [ARCHITECTURE.md](../ARCHITECTURE.md).
