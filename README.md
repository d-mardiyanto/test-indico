# INDICO OTT Integration Service — NETPLAY

Take-home implementation: a Go (Gin) backend and a React (Vite) frontend that
integrate the **NETPLAY** OTT partner into INDICO's subscribe / activation
flow.

- **Backend**: `backend/` — Gin, in-memory storage, provider abstraction.
- **Frontend**: `frontend/` — Vite + React + React Router, mobile-first.
- **Architecture note**: see [`ARCHITECTURE.md`](./ARCHITECTURE.md).

## End-to-end flow

1. External purchase platform (out of scope) calls `POST /api/subscribe`.
2. Backend calls NETPLAY `/subscribe`, normalizes the response, persists a
   local record keyed by a 6-char `activationCode`, and returns an
   **SMS-style message** containing an activation link of the form
   `<FRONTEND_BASE_URL>/activation/<code>`.
3. The user opens the link → React activation page loads current status via
   `GET /api/subscription-status` and offers an **Activate** CTA.
4. Clicking Activate calls `POST /api/activate`, which calls NETPLAY
   `/activate`, updates local state, and returns the normalized record.

## Endpoints

| Method | Path                           | Purpose                                    |
| ------ | ------------------------------ | ------------------------------------------ |
| POST   | `/api/subscribe`               | Post-purchase hook (returns SMS + link)    |
| POST   | `/api/activate`                | Activate via 6-char code                   |
| GET    | `/api/subscription-status`     | Get current record (`?activationCode=...`) |
| GET    | `/api/providers`               | List registered partners                   |
| GET    | `/healthz`                     | Liveness check                             |

The `subscription-status` endpoint accepts `&refresh=true` to also re-sync
from the partner before responding.

## Running locally

There are **two ways** to run the project locally:

1. **Native** — run the Go backend and the Vite dev server directly on your
   machine. Best for active development & hot reload.
2. **Docker Compose** — one command, no Go/Node toolchain required.

---

### Option 1 — Native (Go + Node)

**Prerequisites**

- Go **1.25+**
- Node **20+** & npm

**1. Backend**

```bash
cd backend
cp .env.example .env   # optional — defaults work for local
go run ./cmd
```

The server listens on `:8080` and is pre-configured to talk to the
assignment's NETPLAY base URL.

**2. Frontend** (in a second terminal)

```bash
cd frontend
cp .env.example .env   # optional — defaults work for local
npm install
npm run dev
```

Open <http://localhost:5173>. The Home page is a small **demo simulator**
(stand-in for the external purchase platform) that triggers
`POST /api/subscribe` so you can grab an activation link without a real
upstream caller. In production this page would not exist.

**3. Running tests**

```bash
cd backend
go test ./... -count=1
```

---

### Option 2 — Docker Compose (one-command boot)

**Prerequisites**

- Docker **24+** with the Compose plugin (`docker compose ...`).

A `docker-compose.yml` at the repo root builds both images and wires the
env vars so the activation link → CORS → API base URL chain works out of
the box.

```bash
docker compose up --build
# backend  → http://localhost:8080
# frontend → http://localhost:5173
```

Tear down:

```bash
docker compose down
```

What you get:

- **Backend image** — multi-stage build from `golang:1.25-alpine` into a
  distroless `nonroot` static binary (`backend/Dockerfile`). Exposes `:8080`.
- **Frontend image** — Vite build served by `nginx:1.27-alpine` with SPA
  fallback configured in `frontend/nginx.conf`, so deep links like
  `/activation/ABC123` survive a hard refresh. Exposes `:80`, mapped to
  host `:5173`.

To change the API URL for a non-local deployment, override the build arg
(Vite inlines `VITE_*` values at build time):

```bash
docker compose build \
  --build-arg VITE_API_BASE_URL=https://api.example.com frontend
```

Run the backend test suite inside Docker (no local Go required):

```bash
docker run --rm -v "$PWD/backend":/src -w /src golang:1.25-alpine \
  go test ./... -count=1
```

## Triggering a failed activation (for testing)

The live NETPLAY sandbox returns success for the assignment's tokens, so to
exercise the **failure** branches you need to point the backend at a mock
partner. Failure can surface in two ways:

1. **Body-level failure** — partner returns `2xx` but with a non-success
   `subscriptionStatus` (`failed`, `activation_failed`, `expired`) or with
   `activationStatus != "success"`. Mapped by `provider.NormalizeStatus`
   (see `backend/internal/provider/netplay.go`).
2. **HTTP-level failure** — partner returns an error status, mapped by
   `mapNetplayError`:
   - `401 / 403` → `ErrUnauthorized` → **401** to client
   - `404` → `ErrNotFound` → **404**
   - `5xx` → `ErrUnavailable` → **502**
   - other `4xx` → `ErrBadResponse` → **400**
   - context deadline → `ErrTimeout` → **504**

### Bundled mock partner: `cmd/mockpartner`

A ready-made stand-in lives at `backend/cmd/mockpartner`. Pick a `MODE` and
run it; the backend talks to it once you flip `NETPLAY_BASE_URL`.

| `MODE`    | Effect on `/activate`                         | Final status            |
| --------- | --------------------------------------------- | ----------------------- |
| `active`  | `subscriptionStatus=active`                   | `active`                |
| `pending` | `subscriptionStatus=pending_activation`       | `pending_activation`    |
| `failed`  | `subscriptionStatus=activation_failed`        | `failed`                |
| `expired` | `subscriptionStatus=expired`                  | `expired`               |
| `unknown` | `subscriptionStatus=weird-value`              | `unknown`               |
| `http401` | returns `401 Unauthorized`                    | `ErrUnauthorized` → 401 |
| `http500` | returns `500 Internal Server Error`           | `ErrUnavailable` → 502  |
| `timeout` | sleeps 10s (set `HTTP_TIMEOUT_MS=500`)        | `ErrTimeout` → 504      |

**Run natively** (in a second terminal):

```bash
cd backend
MODE=failed go run ./cmd/mockpartner            # listens on :9999
```

Then point the backend at it (`backend/.env`) and restart `go run ./cmd`:

```
NETPLAY_BASE_URL=http://localhost:9999
```

**Or via Docker Compose** — a `mock` profile is wired up:

```bash
MOCK_MODE=failed docker compose --profile mock up --build mockpartner
# In another shell, restart the backend pointed at the mock service:
docker compose run --rm -e NETPLAY_BASE_URL=http://mockpartner:9999 \
  --service-ports backend
```

Run a normal Subscribe → Activate flow from the UI; the resulting
subscription will land in `subscriptionStatus: "failed"` (or whatever
`MODE` you chose) and the activation page will show its **error** state.

### Re-activation guard

The service short-circuits when the local record is already `active`
(`subscription_service.go`: returns the existing record without calling the
partner). To force a re-call, restart the backend (in-memory storage is
wiped) and run a fresh Subscribe before Activate.

## Environment variables

### Backend

| Variable             | Default                                                                  | Purpose                                       |
| -------------------- | ------------------------------------------------------------------------ | --------------------------------------------- |
| `PORT`               | `8080`                                                                   | HTTP listen port                              |
| `NETPLAY_BASE_URL`   | `https://ctazh5lrhe.execute-api.ap-southeast-3.amazonaws.com/dev/api`    | NETPLAY API base URL                          |
| `FRONTEND_BASE_URL`  | `http://localhost:5173`                                                  | Base URL embedded in activation links + CORS  |
| `HTTP_TIMEOUT_MS`    | `5000`                                                                   | Per-request timeout for partner HTTP calls    |

### Frontend

| Variable             | Default                  | Purpose                |
| -------------------- | ------------------------ | ---------------------- |
| `VITE_API_BASE_URL`  | `http://localhost:8080`  | Backend API base URL   |

A starter `frontend/.env` is included.

## Local persistence

**Choice: in-memory map** (`backend/internal/storage/memory.go`), guarded by
a `sync.RWMutex`, indexed by both `activationCode` and `activationToken`.

**Why**: the assignment allows file or in-memory storage and prioritizes
clarity over completeness. In-memory keeps the test surface small and the
data lifecycle (per-process) predictable for an 8-hour scope. The storage
type sits behind a small set of methods (`Save`, `GetByCode`, `GetByToken`,
`List`), so swapping in a JSON-file or SQLite backend later is a localized
change.

**Trade-off**: state is lost on restart. For a take-home this is acceptable
because the partner API itself remains the source of truth (we can always
re-fetch via `refresh=true`).

## Assumptions

- Each subscribe creates a *new* local record, even for repeating user IDs;
  duplicate prevention/idempotency at the INDICO layer was out of scope.
- The 6-char activation code is treated as effectively unique for this
  exercise (no collision-retry loop). With a 62-char alphabet and 6 chars
  collision probability is negligible for the test data set.
- The `Idempotency-Key` for partner subscribe is generated per call (UUID v4);
  retry semantics across calls are not built (see "What would be improved").
- The NETPLAY API contract from the brief is assumed authoritative; any
  fields we don't recognize fall through into `LastMessage` / are ignored.
- The Home page is a developer aid, not a deliverable user surface.

## Known limitations

- No persistence across restarts (see Local persistence).
- No retry policy for transient partner errors (5xx / timeout). The error is
  surfaced and the user can press **Retry** in the UI.
- No structured logging — relies on Gin's default request logger.
- No rate limiting / auth on our own API endpoints. Treat as internal.
- Frontend visual fidelity to the Figma is approximate (color, layout, copy)
  — the focus was the activation UX state machine.
- Tests cover provider normalization, error mapping, and service flows.
  Handler-level HTTP tests were skipped to fit the time budget.

## AI usage

This implementation was developed with the help of an AI pair-programming
assistant. Specifically, AI was used to:

- Scaffold the provider abstraction (interface, registry, typed errors) and
  the normalization layer between NETPLAY's wire format and our internal
  contract.
- Generate the `httptest`-based provider tests and the service-layer fake
  provider tests.
- Draft this README and the architecture note.
- Suggest the activation page state machine (`loading | ready | activating
  | active | error`) and the corresponding CSS theme.

Every file was reviewed and adjusted by hand: the module structure,
HTTP/error mapping (timeout → 504, 5xx → 502, etc.), idempotent re-activation,
status refresh semantics, and CORS wiring were validated against the
assignment brief and against the live NETPLAY API (end-to-end smoke test
during development).

## What would be improved with more time

- **Persistence**: swap in-memory for JSON file or SQLite via the same
  storage interface; add restart-safe replay.
- **Retry policy**: bounded exponential backoff for partner timeouts /
  5xx, with idempotency-key reuse on retry.
- **Tests**: handler-level Gin tests (httptest.NewRecorder), table-driven
  status mapping, contract tests against a recorded NETPLAY fixture.
- **Structured logging**: switch Gin default logger for slog with request
  IDs propagated through the service layer.
- **Tighter Figma fidelity** and a polished status timeline (e.g. show
  partner activation step-by-step).
- **Provider auth abstraction**: a `ProviderAuth` interface so partners that
  need OAuth/HMAC plug in without leaking auth logic into clients.
- **OpenAPI** spec generated from the handler layer + a Postman collection.
- **Docker** images for backend & frontend, plus a `docker-compose.yml` for
  one-command local boot.
- **CI**: GitHub Actions running `go test ./...` and `npm run build`.
