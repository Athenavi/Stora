package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Athenavi/Stora/internal/adminui"
	adminapi "github.com/Athenavi/Stora/internal/api/v2/admin"
	authapi "github.com/Athenavi/Stora/internal/api/v2/auth"
	e2eeapi "github.com/Athenavi/Stora/internal/api/v2/e2ee"
	fileapi "github.com/Athenavi/Stora/internal/api/v2/files"
	shareapi "github.com/Athenavi/Stora/internal/api/v2/share"
	"github.com/Athenavi/Stora/internal/api/v2/wopi"
	mobileapi "github.com/Athenavi/Stora/internal/api/v3/mobile"
	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/internal/webdav"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/cache"
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
	if err := fileapi.SetEncryptionKey(vaultKey); err != nil {
		log.Fatalf("[Server] vault encryption key invalid: %v", err)
	}

	// Trust a forwarding header only when the operator explicitly opted in.
	middleware.SetTrustedIPHeader(cfg.TrustedIPHeader)

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

	// Router
	//
	// Note: chi's RealIP is deliberately NOT used. It rewrites RemoteAddr from the
	// client-supplied X-Forwarded-For, which would make per-IP rate limits, IP
	// allow-lists and audit logs spoofable. Behind a reverse proxy, set
	// TRUSTED_IP_HEADER to the header the proxy overwrites itself (e.g. X-Real-IP).
	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	// Bulk transfers legitimately outlive a JSON request: they get the server's
	// full WriteTimeout budget while everything else fails fast.
	r.Use(timeoutByPath(transferPaths, 60*time.Second, 600*time.Second))
	// CORS: only the configured origins may call the API from a browser.
	// Wildcard + AllowCredentials is both invalid per the CORS spec and unsafe.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins(),
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
	loginLimiter := middleware.NewRateLimiter(10, 1*time.Minute)   // 10 login attempts/min per IP
	registerLimiter := middleware.NewRateLimiter(3, 1*time.Minute) // 3 registrations/min per IP

	// Initialize the object storage driver.
	//
	// Both drivers expose the same content-addressed key layout (`<hash[:2]>/<hash[2:]>`),
	// so switching backends is a configuration change: the file_path values stored
	// in file_items stay valid and objects are a plain copy apart.
	var store storage.Driver
	switch strings.ToLower(cfg.StorageDriver) {
	case "s3":
		presignTTL := time.Duration(0)
		if cfg.S3PresignDownload {
			presignTTL = 15 * time.Minute
		}
		s3Driver, err := storage.NewS3Driver(context.Background(), storage.S3Options{
			Endpoint:       cfg.S3Endpoint,
			Region:         cfg.S3Region,
			Bucket:         cfg.S3Bucket,
			AccessKey:      cfg.S3AccessKey,
			SecretKey:      cfg.S3SecretKey,
			ForcePathStyle: cfg.S3ForcePathStyle,
			PresignTTL:     presignTTL,
		})
		if err != nil {
			log.Fatalf("[Server] object storage init failed: %v", err)
		}
		store = s3Driver
		log.Printf("[Server] object storage: s3 (bucket=%s endpoint=%s pathStyle=%v)",
			cfg.S3Bucket, cfg.S3Endpoint, cfg.S3ForcePathStyle)
	default:
		store = storage.NewLocalDriver(cfg.StorageObjectsDir, "/files")
		log.Printf("[Server] object storage: local (root=%s)", cfg.StorageObjectsDir)
	}

	// Initialize auth API handlers
	authHandler := authapi.NewHandler(db, jwtManager, cfg.CookieSecure)

	// Initialize file API handlers
	var pathCache *cache.PathCache
	if database.RedisClient != nil {
		pathCache = cache.NewPathCache(database.RedisClient, 10*time.Second)
	}
	fileHandler := fileapi.NewHandler(db, store, cfg.TempFolder, cfg.VaultDir(), pathCache, speedLimiter)
	uploadHandler := fileapi.NewUploadHandler(db, store, cfg.TempFolder)
	vaultHandler := fileapi.NewVaultHandler(db, cfg.VaultDir())
	transcodeHandler := fileapi.NewTranscodeHandler(db, store)
	transcribeHandler := fileapi.NewTranscribeHandler(db)
	versionHandler := fileapi.NewVersionHandler(db, store)
	batchHandler := fileapi.NewBatchHandler(db, store)
	trashHandler := fileapi.NewTrashHandler(db)
	blockHandler := fileapi.NewBlockHandler(db, store, cfg.TempFolder)
	sessionHandler := fileapi.NewUploadSessionHandler(db, store, cfg.TempFolder)

	// Thumbnails are rendered in the background so an upload never waits on image
	// decoding or ffmpeg.
	thumbnailHandler := fileapi.NewThumbnailHandler(db, store)
	go thumbnailHandler.Run(context.Background())

	// Zero-knowledge private spaces: the server stores ciphertext and has no
	// decrypt path at all (see docs/design-e2ee.md).
	e2eeHandler := e2eeapi.NewHandler(db, store)

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
	wopiHandler := wopi.NewHandler(db, store, jwtManager)

	// Setup cleanup notifier and scheduler
	notifier := cleanup.NewCleanupNotifier()
	notifier.Register(&cleanup.TempDirCleaner{})
	notifier.Register(&cleanup.UploadChunkCleaner{DB: db})

	scheduler := cleanup.NewCleanupScheduler(db, notifier, cfg.UploadExpireHours, cfg.TempFolder)
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
			r.Patch("/auth/me", authHandler.UpdateMe)
			r.Post("/auth/change-password", authHandler.ChangePassword)
			r.Get("/auth/sessions", authHandler.ListSessions)
			r.Delete("/auth/sessions/{id}", authHandler.RevokeSession)

			// Credentials for Basic-auth-only clients (WebDAV), individually revocable.
			r.Get("/auth/app-passwords", authHandler.ListAppPasswords)
			r.Post("/auth/app-passwords", authHandler.CreateAppPassword)
			r.Delete("/auth/app-passwords/{id}", authHandler.DeleteAppPassword)

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
					"max_storage":   total,
					"used_storage":  used,
					"usage_percent": usagePercent,
				})
			})
			// Recalculate storage quota from actual file sizes
			r.Post("/users/me/quota/recalculate", func(w http.ResponseWriter, r *http.Request) {
				userID, ok := middleware.GetUserID(r.Context())
				if !ok {
					utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
				var total, actual int64
				err := db.QueryRow(`SELECT total_storage FROM users WHERE id = $1`, userID).Scan(&total)
				if err != nil {
					utils.WriteError(w, http.StatusNotFound, "user not found")
					return
				}
				db.QueryRow(
					`SELECT COALESCE(SUM(file_size), 0) FROM (
						SELECT file_size FROM file_items WHERE user_id = $1 AND deleted_at IS NULL
						UNION ALL
						SELECT file_size FROM vault_items WHERE user_id = $1
					) AS all_files`,
					userID,
				).Scan(&actual)
				db.Exec(`UPDATE users SET used_storage = $1 WHERE id = $2`, actual, userID)
				var usagePercent float64
				if total > 0 {
					usagePercent = float64(actual) / float64(total) * 100
				}
				utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
					"max_storage":   total,
					"used_storage":  actual,
					"usage_percent": usagePercent,
				})
			})

			// Files
			r.Get("/files", fileHandler.ListFiles)
			r.Get("/files/{id}", fileHandler.GetFile)
			r.Post("/files/upload", fileHandler.UploadFile)
			r.Post("/files/upload/init", uploadHandler.InitUpload)
			r.Get("/files/upload/check", uploadHandler.CheckUpload)
			r.Put("/files/upload/{uploadId}/chunk/{index}", uploadHandler.UploadChunk)
			r.Post("/files/upload/chunk", uploadHandler.UploadChunk) // frontend sends POST with FormData
			r.Get("/files/upload/{uploadId}/status", uploadHandler.UploadStatus)
			r.Post("/files/upload/{uploadId}/complete", uploadHandler.CompleteUpload)
			r.Post("/files/upload/complete", uploadHandler.CompleteUpload) // frontend sends POST with JSON body
			r.Delete("/files/upload/{uploadId}", uploadHandler.CancelUpload)
			r.Patch("/files/{id}", fileHandler.UpdateFile)
			r.Post("/files/{id}/to-vault", fileHandler.CopyFileToVault)
			r.Put("/files/{id}/content", fileHandler.UpdateFileContent)
			r.Delete("/files/{id}", fileHandler.DeleteFile)
			r.Put("/files/{id}/rename", fileHandler.RenameFile)
			r.Put("/files/{id}/favorite", fileHandler.ToggleFavorite)
			r.Put("/files/{id}/move", fileHandler.MoveFile)
			r.Get("/files/{id}/download", fileHandler.DownloadFile)
			r.Get("/files/download/{id}", fileHandler.DownloadFile) // alias for frontend
			r.Get("/files/preview/{id}/{filename}", fileHandler.PreviewFile)
			r.Get("/files/{id}/thumbnail", thumbnailHandler.Get)
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
			r.Patch("/files/folders/{id}", fileHandler.UpdateFolder)
			r.Delete("/files/folders/{id}", fileHandler.DeleteFolder)

			// Tags
			r.Get("/files/tags", fileHandler.ListTags)
			r.Post("/files/tags", fileHandler.CreateTag)
			r.Patch("/files/tags/{id}", fileHandler.UpdateTag)
			r.Delete("/files/tags/{id}", fileHandler.DeleteTag)
			r.Get("/files/{id}/tags", fileHandler.ListFileTags)
			r.Post("/files/{id}/tags", fileHandler.AssignFileTags)
			r.Delete("/files/{id}/tags/{tagId}", fileHandler.RemoveFileTag)

			// Vault
			r.Get("/vaults", vaultHandler.ListVaults)
			r.Post("/vaults", vaultHandler.CreateVault)
			r.Post("/vaults/{vaultId}/verify-password", vaultHandler.VerifyVaultPassword)
			r.Delete("/vaults/{vaultId}", vaultHandler.DeleteVault)
			r.Get("/vaults/{vaultId}/items", vaultHandler.ListVaultItems)
			r.Get("/vaults/{vaultId}/items/{itemId}", vaultHandler.DownloadVaultItem)
			r.Delete("/vaults/{vaultId}/items/{itemId}", vaultHandler.DeleteVaultItem)

			// E2EE private spaces — zero knowledge. The server stores wrapped keys
			// and ciphertext only; it never derives thumbnails, OCR or search
			// entries for these items, because it cannot read them.
			r.Route("/e2ee", func(r chi.Router) {
				r.Get("/vaults", e2eeHandler.ListVaults)
				r.Post("/vaults", e2eeHandler.CreateVault)
				r.Delete("/vaults/{vaultId}", e2eeHandler.DeleteVault)
				r.Get("/vaults/{vaultId}/items", e2eeHandler.ListItems)
				r.Post("/vaults/{vaultId}/items", e2eeHandler.UploadItem)
				r.Get("/items/{itemId}/content", e2eeHandler.DownloadItem)
				r.Delete("/items/{itemId}", e2eeHandler.DeleteItem)
			})

			// Transcoding
			r.Post("/files/transcode/{id}", transcodeHandler.StartTranscode)
			r.Get("/files/transcode/{id}/tasks", transcodeHandler.ListTranscodeTasks)

			// Transcription (AI subtitles)
			r.Post("/files/transcribe/{id}", transcribeHandler.StartTranscription)
			r.Get("/files/transcribe/{id}/status", transcribeHandler.GetTranscriptionStatus)
			r.Get("/files/transcribe/{id}/subtitle", transcribeHandler.GetSubtitleFile)

			// Versions
			r.Get("/files/{id}/versions", versionHandler.ListVersions)
			r.Post("/files/{id}/versions/{versionId}/restore", versionHandler.RestoreVersion)

			// Offline download
			r.Post("/files/offline-download", offlineDownloadHandler.CreateDownloadTask)
			r.Get("/files/offline-download", offlineDownloadHandler.ListDownloadTasks)
			r.Get("/files/offline-download/{id}", offlineDownloadHandler.GetDownloadTask)
			r.Post("/files/offline-download/{id}/retry", offlineDownloadHandler.RetryDownloadTask)

			// Webhooks
			r.Get("/webhooks", webhookHandler.ListWebhooks)
			r.Post("/webhooks", webhookHandler.CreateWebhook)
			r.Delete("/webhooks/{id}", webhookHandler.DeleteWebhook)

			// Batch operations
			r.Post("/files/batch/delete", batchHandler.BatchDelete)
			r.Post("/files/batch/move", batchHandler.BatchMove)
			r.Post("/files/batch/copy", batchHandler.BatchCopy)
			r.Post("/files/batch/tags", batchHandler.BatchAssignTags)
			r.Post("/files/batch/category", batchHandler.BatchSetCategory)
			r.Post("/files/download/batch", batchHandler.BatchDownload)

			// Trash — frontend calls /files/trash/* paths
			r.Get("/files/trash", trashHandler.ListTrash)
			r.Post("/files/trash/{id}/restore", trashHandler.RestoreFile)
			r.Post("/files/trash/{id}/destroy", trashHandler.DestroyFile)
			r.Post("/files/trash/batch-destroy", trashHandler.BatchDestroy)
			r.Post("/files/trash/batch-restore", trashHandler.BatchRestore)
			r.Post("/files/trash/clear", trashHandler.ClearTrash)

			// Block storage (.Stora sync engine)
			r.Post("/blocks/upload", blockHandler.UploadBlock)
			r.Get("/blocks/{hash}", blockHandler.DownloadBlock)
			r.Get("/files/{id}/manifest", blockHandler.GetFileManifest)
			r.Get("/files/{id}/snapshots", blockHandler.ListSnapshots)
			r.Get("/sync/changes", blockHandler.SyncChanges)
			r.Post("/sync/upload", blockHandler.SyncUpload)
			r.Post("/sync/upload/complete", blockHandler.SyncUploadComplete)
			// 块级增量落地：客户端只传变化的块，服务端按清单拼装并写 file_blocks
			r.Post("/sync/upload/blocks", blockHandler.SyncUploadBlocks)
			r.Post("/sync/upload/session", sessionHandler.CreateSession)
			r.Put("/sync/upload/session/{sessionId}", sessionHandler.UploadFragment)
			r.Get("/sync/upload/session/{sessionId}", sessionHandler.GetSessionStatus)
			r.Post("/sync/upload/assign", blockHandler.SyncUploadAssign)
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

			// WOPI editor token — authenticated. Collabora then calls /api/v2/wopi/*
			// server-to-server carrying this token, so those endpoints stay public
			// but validate the token themselves.
			r.Get("/wopi/access/{fileId}", wopiHandler.GetWopiAccess)
		})

		// Public share access (no auth required)
		r.Get("/share/{token}", shareHandler.AccessShareLink)
		r.Get("/share/{code}/info", shareHandler.GetShareInfo)
		r.Get("/share/{code}/download", shareHandler.ShareFileDownload)
		r.Post("/share/{code}/upload", shareHandler.ShareFileUpload)
		r.Post("/share/{code}/save", shareHandler.SaveToMyDrive)
		r.Get("/share/{code}/qrcode", shareHandler.ShareQRCode)
		r.Get("/share/{code}/folder/{folderId}", shareHandler.ShareFolderChildren)
		r.Get("/files/shares/access/{code}", shareHandler.VerifySharePassword)

		// Note: /files/preview/{id}/{filename} is registered once, inside the
		// authenticated group above — anonymous requests must not read files by id.

		// Maintenance public status
		r.Get("/system/maintenance/public-status", func(w http.ResponseWriter, r *http.Request) {
			var enabled bool
			var message string
			db.QueryRow(`SELECT setting_value = 'true' FROM system_settings WHERE setting_key = 'maintenance_mode'`).Scan(&enabled)
			db.QueryRow(`SELECT COALESCE(setting_value, '') FROM system_settings WHERE setting_key = 'maintenance_message'`).Scan(&message)
			utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"enabled": enabled,
				"message": message,
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

	// WOPI routes — Collabora Online calls these server-to-server. Every handler
	// validates the access_token (a Stora JWT minted by /api/v2/wopi/access).
	r.Route("/api/v2/wopi", func(r chi.Router) {
		r.Get("/files/{fileId}", wopiHandler.CheckFileInfo)
		r.Get("/files/{fileId}/contents", wopiHandler.GetFile)
		r.Post("/files/{fileId}/contents", wopiHandler.PutFile)
	})

	// API v3 (mobile)
	mobileAuth := mobileapi.NewAuthHandler(db, jwtManager)
	r.Route("/api/v3", func(r chi.Router) {
		r.Post("/auth/login", mobileAuth.Login)
	})

	// WebDAV — mounted outside /api/v2 because its clients authenticate with
	// HTTP Basic (an application-specific password), not with the API middleware.
	// The prefix must be stripped: webdav.Handler treats r.URL.Path as the file path.
	davHandler := webdav.New(db, store)
	r.Mount("/dav", http.StripPrefix("/dav", davHandler))

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  600 * time.Second,
		WriteTimeout: 600 * time.Second,
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

// transferPaths are the route prefixes that carry (potentially large) request or
// response bodies. A single 60s deadline across the whole API used to fire in
// the middle of a large upload or a rate-limited download.
var transferPaths = []string{
	"/api/v2/files/upload",
	"/api/v2/files/download",
	"/api/v2/files/preview",
	"/api/v2/files/blocks",
	"/api/v2/files/offline-download",
	"/api/v2/sync",
	"/api/v2/share",
	"/api/v2/wopi",
}

// timeoutByPath applies long to transfer prefixes and short to everything else.
func timeoutByPath(prefixes []string, short, long time.Duration) func(http.Handler) http.Handler {
	shortMW := chimw.Timeout(short)
	longMW := chimw.Timeout(long)
	return func(next http.Handler) http.Handler {
		shortH := shortMW(next)
		longH := longMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range prefixes {
				if strings.HasPrefix(r.URL.Path, p) {
					longH.ServeHTTP(w, r)
					return
				}
			}
			shortH.ServeHTTP(w, r)
		})
	}
}
