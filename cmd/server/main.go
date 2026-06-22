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

	authapi "github.com/Athenavi/Stora/internal/api/v2/auth"
	fileapi "github.com/Athenavi/Stora/internal/api/v2/files"
	shareapi "github.com/Athenavi/Stora/internal/api/v2/share"
	adminapi "github.com/Athenavi/Stora/internal/api/v2/admin"
	mobileapi "github.com/Athenavi/Stora/internal/api/v3/mobile"
	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/config"
	"github.com/Athenavi/Stora/pkg/database"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/Athenavi/Stora/pkg/utils/cleanup"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
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

	// Auto-migrate: add missing columns that Go code references
	migrations := []string{
		`ALTER TABLE file_items ADD COLUMN IF NOT EXISTS description TEXT`,
		`ALTER TABLE upload_tasks ADD COLUMN IF NOT EXISTS upload_id VARCHAR(255)`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			log.Printf("[Server] Migration warning (%s): %v", m[:60], err)
		} else {
			log.Printf("[Server] Migration OK: %s", m[:60])
		}
	}

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Timeout(60 * time.Second))
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

	// Initialize JWT manager
	jwtManager := auth.NewJWTManager(cfg.SecretKey, cfg.JWTExpiration, cfg.JWTRefreshExpiration)

	// Initialize storage driver
	store := storage.NewLocalDriver(cfg.StorageObjectsDir, "/files")

	// Initialize auth API handlers
	authHandler := authapi.NewHandler(db, jwtManager)

	// Initialize file API handlers
	fileHandler := fileapi.NewHandler(db, store, cfg.TempFolder)
	uploadHandler := fileapi.NewUploadHandler(db, store, cfg.TempFolder)
	vaultHandler := fileapi.NewVaultHandler(db)
	transcodeHandler := fileapi.NewTranscodeHandler(db)
	versionHandler := fileapi.NewVersionHandler(db)
	batchHandler := fileapi.NewBatchHandler(db, store)
	trashHandler := fileapi.NewTrashHandler(db)

	// Initialize share handler
	shareHandler := shareapi.NewHandler(db, store)

	// Initialize admin handler
	adminHandler := adminapi.NewHandler(db)

	// Setup cleanup notifier and scheduler
	notifier := cleanup.NewCleanupNotifier()
	notifier.Register(&cleanup.TempDirCleaner{})
	notifier.Register(&cleanup.UploadChunkCleaner{DB: db})
	notifier.Register(&cleanup.OrphanFileItemCleaner{DB: db, ExpireHours: cfg.UploadExpireHours})

	scheduler := cleanup.NewCleanupScheduler(db, notifier, cfg.UploadExpireHours)
	scheduler.Start()
	defer scheduler.Stop()

	// API v2 routes
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"message":"Stora API v2","version":"0.1.0-go"}`)
		})

		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
			r.Post("/send-code", authHandler.SendCode)
			r.Post("/login-with-code", authHandler.LoginWithCode)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(jwtManager))

			// Auth (authenticated)
			r.Post("/auth/logout", authHandler.Logout)
			r.Get("/auth/me", authHandler.Me)

			// User quota
			r.Get("/users/me/quota", func(w http.ResponseWriter, r *http.Request) {
				userID, ok := middleware.GetUserID(r.Context())
				if !ok {
					utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
				var total, used int64
				err := db.QueryRow(
					`SELECT total_storage, used_storage FROM users WHERE id = $1`, userID,
				).Scan(&total, &used)
				if err != nil {
					utils.WriteError(w, http.StatusNotFound, "user not found")
					return
				}
				utils.WriteJSON(w, http.StatusOK, map[string]int64{
					"total_storage": total,
					"used_storage":  used,
				})
			})

			// Files
			r.Get("/files", fileHandler.ListFiles)
			r.Get("/files/{id}", fileHandler.GetFile)
			r.Post("/files/upload", fileHandler.UploadFile)
			r.Post("/files/upload/init", uploadHandler.InitUpload)
			r.Put("/files/upload/{uploadId}/chunk/{index}", uploadHandler.UploadChunk)
			r.Get("/files/upload/{uploadId}/status", uploadHandler.UploadStatus)
			r.Post("/files/upload/{uploadId}/complete", uploadHandler.CompleteUpload)
			r.Delete("/files/upload/{uploadId}", uploadHandler.CancelUpload)
			r.Patch("/files/{id}", fileHandler.UpdateFile)
			r.Delete("/files/{id}", fileHandler.DeleteFile)
			r.Put("/files/{id}/rename", fileHandler.RenameFile)
			r.Put("/files/{id}/favorite", fileHandler.ToggleFavorite)
			r.Put("/files/{id}/move", fileHandler.MoveFile)
			r.Get("/files/{id}/download", fileHandler.DownloadFile)
			r.Get("/files/download/{id}", fileHandler.DownloadFile) // alias for frontend
			r.Get("/files/preview/{id}/{filename}", fileHandler.PreviewFile)
			r.Get("/files/search", fileHandler.Search)

			// Folders
			r.Get("/files/folders/tree", fileHandler.ListFolders)
			r.Get("/files/folders/by-path", fileHandler.GetFolderChildrenByPath)
			r.Post("/files/folders/by-path", fileHandler.CreateFolderByPath)
			r.Get("/files/folders/{id}/children", fileHandler.GetFolderChildren)
			r.Post("/files/folders", fileHandler.CreateFolder)
			r.Delete("/files/folders/{id}", fileHandler.DeleteFolder)

			// Tags
			r.Get("/files/tags", fileHandler.ListTags)
			r.Post("/files/tags", fileHandler.CreateTag)
			r.Patch("/files/tags/{id}", fileHandler.UpdateTag)
			r.Delete("/files/tags/{id}", fileHandler.DeleteTag)

			// Vault
			r.Get("/vaults", vaultHandler.ListVaults)
			r.Post("/vaults", vaultHandler.CreateVault)
			r.Post("/vaults/{vaultId}/verify-password", vaultHandler.VerifyVaultPassword)
			r.Delete("/vaults/{vaultId}", vaultHandler.DeleteVault)
			r.Get("/vaults/{vaultId}/items", vaultHandler.ListVaultItems)
			r.Post("/vaults/{vaultId}/items/upload", vaultHandler.UploadVaultItem)
			r.Get("/vaults/{vaultId}/items/{itemId}", vaultHandler.DownloadVaultItem)
			r.Delete("/vaults/{vaultId}/items/{itemId}", vaultHandler.DeleteVaultItem)

			// Transcoding
			r.Post("/files/{id}/transcode", transcodeHandler.StartTranscode)

			// Versions
			r.Get("/files/{id}/versions", versionHandler.ListVersions)

			// Batch operations
			r.Post("/files/batch/delete", batchHandler.BatchDelete)
			r.Post("/files/batch/move", batchHandler.BatchMove)
			r.Post("/files/download/batch", batchHandler.BatchDownload)

			// Trash — frontend calls /files/trash/* paths
			r.Get("/files/trash", trashHandler.ListTrash)
			r.Post("/files/trash/{id}/restore", trashHandler.RestoreFile)
			r.Post("/files/trash/{id}/destroy", trashHandler.DestroyFile)
			r.Post("/files/trash/batch-destroy", trashHandler.BatchDestroy)
			r.Post("/files/trash/batch-restore", trashHandler.BatchRestore)
			r.Post("/files/trash/clear", trashHandler.ClearTrash)
			// Legacy paths
			r.Get("/trash", trashHandler.ListTrash)
			r.Post("/trash/{id}/restore", trashHandler.RestoreFile)
			r.Post("/trash/empty", trashHandler.EmptyTrash)

			// Share links — frontend calls /files/shares/*
			r.Get("/files/shares", shareHandler.ListShareLinks)
			r.Post("/files/shares", shareHandler.CreateShareLink)
			r.Delete("/files/shares/{id}", shareHandler.DeleteShareLink)
			// Legacy paths
			r.Get("/share/links", shareHandler.ListShareLinks)
			r.Post("/share/links", shareHandler.CreateShareLink)
			r.Delete("/share/links/{id}", shareHandler.DeleteShareLink)

			// Notifications
			r.Get("/notifications", adminHandler.ListNotifications)
			r.Put("/notifications/{id}/read", adminHandler.MarkRead)

			// 2FA
			r.Post("/auth/2fa/setup", adminHandler.Setup2FA)
			r.Post("/auth/2fa/disable", adminHandler.Disable2FA)

			// Admin (requires superuser)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)

				// Dashboard stats
				r.Get("/admin/dashboard", adminHandler.DashboardStats)

				// User management
				r.Get("/admin/users", adminHandler.ListUsers)
				r.Put("/admin/users/{id}", adminHandler.UpdateUser)

				// Roles
				r.Get("/admin/roles", adminHandler.ListRoles)
				r.Post("/admin/roles", adminHandler.CreateRole)

				// Settings
				r.Get("/admin/settings", adminHandler.ListSettings)
				r.Put("/admin/settings", adminHandler.UpdateSetting)

				// Audit logs
				r.Get("/admin/audit-logs", adminHandler.ListAuditLogs)

				// Sensitive words
				r.Get("/admin/sensitive-words", adminHandler.ListSensitiveWords)
			})
		})

		// Public share access (no auth required)
		r.Get("/share/{token}", shareHandler.AccessShareLink)
		r.Get("/share/{code}/info", shareHandler.GetShareInfo)
		r.Get("/share/{code}/download", shareHandler.ShareFileDownload)
		r.Get("/files/shares/access/{code}", shareHandler.VerifySharePassword)

		// File preview (optional auth for <img> tags)
		r.With(middleware.OptionalAuthMiddleware(jwtManager)).Get("/files/preview/{id}/{filename}", fileHandler.PreviewFile)

		// Maintenance public status
		r.Get("/system/maintenance/public-status", func(w http.ResponseWriter, r *http.Request) {
			utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"maintenance_mode": false,
				"message":          "",
			})
		})
	})

	// API v3 (mobile)
	mobileAuth := mobileapi.NewAuthHandler(db, jwtManager)
	r.Route("/api/v3", func(r chi.Router) {
		r.Post("/auth/login", mobileAuth.Login)
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
