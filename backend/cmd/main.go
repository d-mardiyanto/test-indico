// Command server is the HTTP entry point for the INDICO OTT Integration
// Service. It wires together the provider registry, in-memory storage,
// subscription service, and Gin router.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"backend/internal/handler"
	"backend/internal/provider"
	"backend/internal/service"
	"backend/internal/storage"
)

func main() {
	cfg := loadConfig()

	// Provider registry: add more partners here as they are integrated.
	netplay := provider.NewNetplayProvider(cfg.NetplayBaseURL, cfg.HTTPTimeout)
	registry := provider.NewRegistry(netplay)

	// In-memory store (see README for rationale).
	store := storage.NewMemoryStorage()

	svc := service.NewSubscriptionService(service.Config{
		Registry:        registry,
		Storage:         store,
		FrontendBaseURL: cfg.FrontendBaseURL,
	})

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	handler.New(svc).Register(r)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (frontend=%s, netplay=%s)", srv.Addr, cfg.FrontendBaseURL, cfg.NetplayBaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
}

// ---- Config -----------------------------------------------------------------

type config struct {
	Port            string
	NetplayBaseURL  string
	FrontendBaseURL string
	HTTPTimeout     time.Duration
	AllowedOrigins  []string
}

func loadConfig() config {
	return config{
		Port:            getenv("PORT", "8080"),
		NetplayBaseURL:  getenv("NETPLAY_BASE_URL", "https://ctazh5lrhe.execute-api.ap-southeast-3.amazonaws.com/dev/api"),
		FrontendBaseURL: getenv("FRONTEND_BASE_URL", "http://localhost:5173"),
		HTTPTimeout:     getenvDuration("HTTP_TIMEOUT_MS", 5000),
		AllowedOrigins:  []string{getenv("FRONTEND_BASE_URL", "http://localhost:5173")},
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvDuration(k string, defMS int) time.Duration {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Millisecond
		}
	}
	return time.Duration(defMS) * time.Millisecond
}
