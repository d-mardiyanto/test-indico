// Command mockpartner is a tiny stand-in for the NETPLAY partner API. It
// implements the three endpoints our client talks to (/subscribe, /activate,
// /subscription-status) and lets you select which terminal subscription
// status the partner should report by setting the MODE env var.
//
// This is a developer tool: point NETPLAY_BASE_URL at this server to
// exercise the failure / pending / expired / unknown branches end-to-end
// without depending on the live sandbox.
//
// Usage:
//
//	MODE=failed go run ./cmd/mockpartner            # default port :9999
//	MODE=expired PORT=9000 go run ./cmd/mockpartner
//
// Modes:
//
//	active   - /activate returns subscriptionStatus=active   (happy path)
//	pending  - /activate returns subscriptionStatus=pending  (stays pending)
//	failed   - /activate returns subscriptionStatus=activation_failed
//	expired  - /activate returns subscriptionStatus=expired
//	unknown  - /activate returns subscriptionStatus=weird-value (→ unknown)
//	http401  - /activate returns 401 Unauthorized
//	http500  - /activate returns 500 Internal Server Error
//	timeout  - /activate sleeps so the client hits its deadline
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type modeConfig struct {
	subscriptionStatus string // value echoed in subscriptionStatus
	activationStatus   string // value echoed in activationStatus
	httpStatus         int    // non-zero overrides 200
	delay              time.Duration
}

var modes = map[string]modeConfig{
	"active":  {subscriptionStatus: "active", activationStatus: "success"},
	"pending": {subscriptionStatus: "pending_activation", activationStatus: "success"},
	"failed":  {subscriptionStatus: "activation_failed", activationStatus: "failed"},
	"expired": {subscriptionStatus: "expired", activationStatus: "success"},
	"unknown": {subscriptionStatus: "weird-value", activationStatus: "success"},
	"http401": {httpStatus: http.StatusUnauthorized},
	"http500": {httpStatus: http.StatusInternalServerError},
	"timeout": {delay: 10 * time.Second, subscriptionStatus: "active", activationStatus: "success"},
}

func main() {
	modeName := getenv("MODE", "active")
	cfg, ok := modes[modeName]
	if !ok {
		log.Fatalf("unknown MODE=%q (valid: active|pending|failed|expired|unknown|http401|http500|timeout)", modeName)
	}
	port := getenv("PORT", "9999")

	mux := http.NewServeMux()

	// Subscribe always returns a pending_activation token; the interesting
	// behavior is on /activate. This keeps the demo flow short.
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[mockpartner] POST /subscribe idempotency-key=%q", r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{`+
			`"subscriptionRequestId":"SUBREQ-MOCK",`+
			`"activationToken":"tok-mock",`+
			`"status":"pending_activation",`+
			`"message":"mock subscribe ok"`+
			`}`)
	})

	mux.HandleFunc("/activate", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[mockpartner] POST /activate mode=%s", modeName)
		if cfg.delay > 0 {
			time.Sleep(cfg.delay)
		}
		if cfg.httpStatus != 0 {
			http.Error(w, http.StatusText(cfg.httpStatus), cfg.httpStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{`+
			`"provider":"NETPLAY",`+
			`"userId":"u-mock",`+
			`"activationStatus":%q,`+
			`"subscriptionStatus":%q,`+
			`"plan":"PREMIUM_30D",`+
			`"externalReferenceId":"EXT-MOCK",`+
			`"activatedAt":"2026-05-17T16:29:33Z",`+
			`"message":"mock activate (%s)"`+
			`}`, cfg.activationStatus, cfg.subscriptionStatus, modeName)
	})

	mux.HandleFunc("/subscription-status", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[mockpartner] GET /subscription-status mode=%s token=%q", modeName, r.URL.Query().Get("activationToken"))
		if cfg.httpStatus != 0 {
			http.Error(w, http.StatusText(cfg.httpStatus), cfg.httpStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		status := cfg.subscriptionStatus
		if status == "" {
			status = "pending_activation"
		}
		_, _ = fmt.Fprintf(w, `{`+
			`"subscriptionRequestId":"SUBREQ-MOCK",`+
			`"userId":"u-mock",`+
			`"provider":"NETPLAY",`+
			`"plan":"PREMIUM_30D",`+
			`"subscriptionStatus":%q,`+
			`"activatedAt":"2026-05-17T16:29:33Z",`+
			`"tokenExpiresAt":"2026-05-20T16:29:04Z",`+
			`"externalReferenceId":"EXT-MOCK"`+
			`}`, status)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "ok mode=%s\n", modeName)
	})

	log.Printf("mockpartner listening on :%s (mode=%s)", port, modeName)
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("invalid PORT=%q", port)
	}
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("mockpartner: %v", err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
