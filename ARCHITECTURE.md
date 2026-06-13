# Architecture Note

A short tour of how the integration is structured and the trade-offs taken.

## Layered backend

```
cmd/main.go            wires env → registry → service → router
└── internal/
    ├── handler/        Gin handlers; HTTP↔JSON only. Error→status mapping.
    ├── service/        SubscriptionService: business flow, code gen, persistence.
    ├── provider/       Provider interface + Registry + typed errors.
    │   └── netplay.go  NETPLAY adapter: normalizes wire DTOs → internal DTOs.
    ├── client/         Raw HTTP clients (NETPLAY). Knows the wire format only.
    ├── storage/        In-memory store, indexed by activationCode + token.
    └── model/          Canonical Subscription record + status constants.
```

Each layer depends only on the layers below it.

## Provider abstraction

`provider.Provider` exposes three methods (`Subscribe`, `Activate`, `Status`)
that take and return **normalized** DTOs. NETPLAY's wire DTOs live in
`internal/client/netplay_client.go` and never leak past `internal/provider/
netplay.go`, which is the single point where wire fields are mapped to:

- Internal status vocabulary (`pending_activation | active | failed |
  expired | unknown`).
- Typed errors (`ErrTimeout`, `ErrUnavailable`, `ErrUnauthorized`,
  `ErrNotFound`, `ErrBadResponse`).
- Time fields parsed from RFC3339 to `*time.Time`.

A `Registry` maps `provider name → Provider`. Adding a new partner is:

1. Create `internal/client/<partner>_client.go` (wire client).
2. Create `internal/provider/<partner>.go` implementing `Provider`.
3. Register it in `cmd/main.go`.

No changes are needed in handlers, service, or storage.

## Subscribe / Activate flow

1. **Handler** validates JSON, calls `service.Subscribe(...)`.
2. **Service** looks up the provider by name, generates a UUID
   `Idempotency-Key` and a 6-char alphanumeric `activationCode`
   (`crypto/rand`), calls `provider.Subscribe`, persists the normalized
   record, and returns the SMS-style payload.
3. **Activation page** later loads `subscription-status`, then calls
   `POST /api/activate {activationCode}`.
4. **Service.Activate** looks up the local record, short-circuits if
   already `active` (idempotent), otherwise calls `provider.Activate`
   with the stored `activationToken`, updates the record, and returns it.

## Error mapping

Provider-level errors are mapped to HTTP status in the handler layer:

| Provider error          | HTTP status | Code                   |
| ----------------------- | ----------- | ---------------------- |
| `ErrTimeout`            | 504         | `provider_timeout`     |
| `ErrUnavailable` / 5xx  | 502         | `provider_unavailable` |
| `ErrUnauthorized`       | 502         | `provider_unauthorized`|
| `ErrNotFound`           | 404         | `provider_not_found`   |
| `ErrBadResponse`        | 502         | `provider_bad_response`|
| `service.ErrNotFound`   | 404         | `not_found`            |
| `service.ErrInvalid…`   | 400         | `invalid_request`      |
| `service.ErrUnknownProvider` | 400    | `unknown_provider`     |
| `service.ErrNotActivatable`  | 409    | `not_activatable`      |

Transport / `context.DeadlineExceeded` are squashed to `ErrTimeout` at the
provider boundary so upper layers don't depend on `net/http` internals.

## Timeouts & cancellation

- The HTTP client used by `client.NetplayClient` has a per-request timeout
  from `HTTP_TIMEOUT_MS` (default 5s).
- All partner calls accept `context.Context`; the handler passes
  `c.Request.Context()`, so client disconnect propagates downstream.

## Persistence

In-memory `MemoryStorage` with `sync.RWMutex`. Two indexes:

- `byCode map[string]*Subscription` — primary key for the user flow.
- `codeByToken map[string]string` — secondary index for partner-token
  lookups (used when refreshing from `/subscription-status`).

`Save` always overwrites; `Get*` return copies, so callers cannot mutate
internal state by mistake.

## Frontend

- `App.jsx` is a `BrowserRouter` with two routes: `/` (Home demo) and
  `/activation/:code` (real activation page).
- `src/api.js` centralizes fetch logic and reads `VITE_API_BASE_URL`.
- `Activation.jsx` is a small state machine:
  `loading → ready → activating → (active | error)` with a Retry button on
  error.
- Styling is plain CSS, mobile-first, dark gradient theme. No UI library
  pulled in to keep the bundle small.

## Trade-offs

- **In-memory > file**: simpler, less I/O surface area, easy to test. Lost
  on restart — acceptable because the partner is the source of truth.
- **No retry policy**: simplifies happy/error paths, and the UI exposes a
  Retry button. A bounded backoff would be a small addition.
- **Single SubscriptionService** instead of separate use-case classes:
  fewer files, still clean because the file stays under ~300 LOC.
- **Activation code, not token, in the public URL**: the partner's token is
  treated as a secret stored server-side; the URL exposes only an opaque
  6-char handle to the user.
- **Home page exists**: not part of the production flow, but lets reviewers
  exercise subscribe → activate end-to-end without curl. Clearly labeled.
