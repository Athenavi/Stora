package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Athenavi/Stora/pkg/config"
	"github.com/Athenavi/Stora/pkg/database"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Server] Stora backend starting...")

	// Load configuration
	cfg := config.Load()
	if cfg.SecretKey == "" {
		log.Fatal("[Server] SECRET_KEY is required. Set it in .env or environment.")
	}

	// Connect to PostgreSQL
	db, err := database.Connect(cfg.PostgresDSN(), 25, 5)
	if err != nil {
		log.Fatalf("[Server] Database connection failed: %v", err)
	}
	defer database.Close()

	// Connect to Redis (optional, warn on failure)
	redisAddr := cfg.RedisAddr()
	if _, err := database.ConnectRedis(redisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
		log.Printf("[Server] Redis not available (non-fatal): %v", err)
	} else {
		defer database.CloseRedis()
	}

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		// Check DB connectivity
		if err := db.Ping(); err != nil {
			http.Error(w, `{"status":"unhealthy","database":"down"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"healthy","database":"ok","version":"0.1.0"}`)
	})
	r.Get("/api/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"alive"}`)
	})

	// API v2 placeholder
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"message":"Stora API v2","version":"0.1.0-go"}`)
		})
	})

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("[Server] Listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Server] Listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[Server] Shutdown error: %v", err)
	}
	log.Println("[Server] Stopped")
}
