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
	"github.com/Athenavi/Stora/internal/adminui"
	"github.com/Athenavi/Stora/internal/api/v2/wopi"
	"github.com/Athenavi/Stora/pkg/cache"
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

	// Initialize vault encryption key from config or secret key
	vaultKey := cfg.VaultEncryptionKey
	if vaultKey == "" {
		vaultKey = cfg.SecretKey // fallback to main secret
	}
	fileapi.SetEncryptionKey(vaultKey)

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
		`CREATE TABLE IF NOT EXISTS transcription_tasks (
			id BIGSERIAL PRIMARY KEY,
			file_id BIGINT NOT NULL REFERENCES file_items(id) ON DELETE CASCADE,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			content TEXT,
			error_msg TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS vaults (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			password_hash VARCHAR(128) DEFAULT '',
			description TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vaults_user_id ON vaults(user_id)`,
		`CREATE TABLE IF NOT EXISTS vault_items (
			id BIGSERIAL PRIMARY KEY,
			vault_id BIGINT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			filename VARCHAR(512) DEFAULT '',
			file_size BIGINT DEFAULT 0,
			mime_type VARCHAR(128) DEFAULT '',
			content_type VARCHAR(64) DEFAULT 'file',
			content TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
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

	// Initialize speed limiter (KB/s, 0 = unlimited)
	speedLimiter := middleware.NewSpeedLimiter(cfg.UploadSpeedLimit, cfg.DownloadSpeedLimit)

	// Rate limiters for auth endpoints
	loginLimiter := middleware.NewRateLimiter(10, 1*time.Minute)    // 10 login attempts/min per IP
	registerLimiter := middleware.NewRateLimiter(3, 1*time.Minute)  // 3 registrations/min per IP

	// Initialize storage driver
	store := storage.NewLocalDriver(cfg.StorageObjectsDir, "/files")

	// Initialize auth API handlers
	authHandler := authapi.NewHandler(db, jwtManager)

	// Initialize file API handlers
	var pathCache *cache.PathCache
	if database.RedisClient != nil {
		pathCache = cache.NewPathCache(database.RedisClient, 10*time.Second)
	}
	fileHandler := fileapi.NewHandler(db, store, cfg.TempFolder, pathCache, speedLimiter)
	uploadHandler := fileapi.NewUploadHandler(db, store, cfg.TempFolder)
	vaultHandler := fileapi.NewVaultHandler(db)
	transcodeHandler := fileapi.NewTranscodeHandler(db, store)
	transcribeHandler := fileapi.NewTranscribeHandler(db)
	versionHandler := fileapi.NewVersionHandler(db)
	batchHandler := fileapi.NewBatchHandler(db, store)
	trashHandler := fileapi.NewTrashHandler(db)

	// Initialize offline download handler
	offlineDownloadHandler := fileapi.NewOfflineDownloadHandler(db, store, cfg.TempFolder)

	// Initialize webhook handler
	webhookHandler := fileapi.NewWebhookHandler(db)

	// Initialize share handler
	shareHandler := shareapi.NewHandler(db, store)
	shareapi.StartShareCleanup(db)

	// Initialize team handler
	teamHandler := shareapi.NewTeamHandler(db)

	// Initialize admin handler
	adminHandler := adminapi.NewHandler(db)

	// Initialize admin UI handler (Go templates, lightweight management pages)
	adminUIHandler, err := adminui.NewHandler(db, cfg)
	if err != nil {
		log.Printf("[Server] Admin UI init warning: %v", err)
	}

	// Initialize WOPI handler (Collabora Online integration)
	wopiHandler := wopi.NewHandler(db, store)

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
			r.With(loginLimiter.HTTPMiddleware).Post("/login", authHandler.Login)
			r.With(registerLimiter.HTTPMiddleware).Post("/register", authHandler.Register)
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
			r.Get("/auth/sessions", authHandler.ListSessions)
			r.Delete("/auth/sessions/{id}", authHandler.RevokeSession)

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
				var usagePercent float64
				if total > 0 {
					usagePercent = float64(used) / float64(total) * 100
				}
				utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
					"max_storage":    total,
					"used_storage":   used,
					"usage_percent":  usagePercent,
				})
			})

			// Files
			r.Get("/files", fileHandler.ListFiles)
			r.Get("/files/{id}", fileHandler.GetFile)
			r.Post("/files/upload", fileHandler.UploadFile)
			r.Post("/files/upload/init", uploadHandler.InitUpload)
			r.Put("/files/upload/{uploadId}/chunk/{index}", uploadHandler.UploadChunk)
			r.Post("/files/upload/chunk", uploadHandler.UploadChunk) // frontend sends POST with FormData
			r.Get("/files/upload/{uploadId}/status", uploadHandler.UploadStatus)
			r.Post("/files/upload/{uploadId}/complete", uploadHandler.CompleteUpload)
			r.Post("/files/upload/complete", uploadHandler.CompleteUpload) // frontend sends POST with JSON body
			r.Delete("/files/upload/{uploadId}", uploadHandler.CancelUpload)
			r.Patch("/files/{id}", fileHandler.UpdateFile)
			r.Put("/files/{id}/content", fileHandler.UpdateFileContent)
			r.Delete("/files/{id}", fileHandler.DeleteFile)
			r.Put("/files/{id}/rename", fileHandler.RenameFile)
			r.Put("/files/{id}/favorite", fileHandler.ToggleFavorite)
			r.Put("/files/{id}/move", fileHandler.MoveFile)
			r.Get("/files/{id}/download", fileHandler.DownloadFile)
			r.Get("/files/download/{id}", fileHandler.DownloadFile) // alias for frontend
			r.Get("/files/preview/{id}/{filename}", fileHandler.PreviewFile)
			r.Get("/files/search", fileHandler.Search)

			// File comments
			r.Get("/files/{id}/comments", fileHandler.ListComments)
			r.Post("/files/{id}/comments", fileHandler.AddComment)
			r.Delete("/files/comments/{commentId}", fileHandler.DeleteComment)

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
			r.Post("/files/transcode/{id}", transcodeHandler.StartTranscode)
			r.Get("/files/transcode/{id}/tasks", transcodeHandler.ListTranscodeTasks)

			// Transcription (AI subtitles)
			r.Post("/files/transcribe/{id}", transcribeHandler.StartTranscription)
			r.Get("/files/transcribe/{id}/status", transcribeHandler.GetTranscriptionStatus)
			r.Get("/files/transcribe/{id}/subtitle", transcribeHandler.GetSubtitleFile)

			// Versions
			r.Get("/files/{id}/versions", versionHandler.ListVersions)

			// Offline download
			r.Post("/files/offline-download", offlineDownloadHandler.CreateDownloadTask)
			r.Get("/files/offline-download", offlineDownloadHandler.ListDownloadTasks)

			// Webhooks
			r.Get("/webhooks", webhookHandler.ListWebhooks)
			r.Post("/webhooks", webhookHandler.CreateWebhook)
			r.Delete("/webhooks/{id}", webhookHandler.DeleteWebhook)

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
			r.Put("/files/shares/{id}", shareHandler.UpdateShareLink)
			r.Delete("/files/shares/{id}", shareHandler.DeleteShareLink)
			r.Post("/files/shares/share-with-user", shareHandler.ShareWithUser)
			// Legacy paths
			r.Get("/share/links", shareHandler.ListShareLinks)
			r.Post("/share/links", shareHandler.CreateShareLink)
			r.Delete("/share/links/{id}", shareHandler.DeleteShareLink)

			// Teams
			r.Get("/teams", teamHandler.ListTeams)
			r.Post("/teams", teamHandler.CreateTeam)
			r.Delete("/teams/{teamId}", teamHandler.DeleteTeam)
			r.Get("/teams/{teamId}/members", teamHandler.ListMembers)
			r.Post("/teams/{teamId}/members", teamHandler.AddMember)
			r.Delete("/teams/{teamId}/members/{memberId}", teamHandler.RemoveMember)
			r.Get("/teams/{teamId}/folders", teamHandler.ListTeamFolders)
			r.Post("/teams/{teamId}/folders", teamHandler.AddTeamFolder)
			r.Delete("/teams/{teamId}/folders/{folderId}", teamHandler.RemoveTeamFolder)
			r.Get("/users/search", teamHandler.SearchUsers)

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

				// Maintenance mode
				r.Get("/admin/maintenance", adminHandler.GetMaintenanceStatus)
				r.Put("/admin/maintenance", adminHandler.SetMaintenanceMode)

				// Notifications (admin: create system-wide)
				r.Post("/notifications", adminHandler.CreateNotification)

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
		r.Post("/share/{code}/upload", shareHandler.ShareFileUpload)
		r.Post("/share/{code}/save", shareHandler.SaveToMyDrive)
		r.Get("/share/{code}/qrcode", shareHandler.ShareQRCode)
		r.Get("/files/shares/access/{code}", shareHandler.VerifySharePassword)

		// File preview (optional auth for <img> tags)
		r.With(middleware.OptionalAuthMiddleware(jwtManager)).Get("/files/preview/{id}/{filename}", fileHandler.PreviewFile)

		// Maintenance public status
		r.Get("/system/maintenance/public-status", func(w http.ResponseWriter, r *http.Request) {
			var enabled bool
			var message string
			db.QueryRow(`SELECT setting_value = 'true' FROM system_settings WHERE setting_key = 'maintenance_mode'`).Scan(&enabled)
			db.QueryRow(`SELECT COALESCE(setting_value, '') FROM system_settings WHERE setting_key = 'maintenance_message'`).Scan(&message)
			utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"maintenance_mode": enabled,
				"message":          message,
			})
		})
	})

	// Admin UI — Go html/template management pages (no JS framework)
	// Accessible at /admin/ui/ by authenticated admin users
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(jwtManager))
		r.Use(middleware.RequireAdmin)
		if adminUIHandler != nil {
			r.Get("/admin/ui", adminUIHandler.Dashboard)
			r.Get("/admin/ui/", adminUIHandler.Dashboard)
			r.Get("/admin/ui/migrate", adminUIHandler.MigratePage)
		}
	})

	// WOPI routes (public — Collabora Online calls server-to-server)
	r.Route("/api/v2/wopi", func(r chi.Router) {
		r.Get("/access/{fileId}", wopiHandler.GetWopiAccess)
		r.Get("/files/{fileId}", wopiHandler.CheckFileInfo)
		r.Get("/files/{fileId}/contents", wopiHandler.GetFile)
		r.Post("/files/{fileId}/contents", wopiHandler.PutFile)
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
