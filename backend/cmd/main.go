package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/config"
	"logdownloader/internal/handler"
	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
	"logdownloader/internal/notify"
	"logdownloader/internal/scheduler"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"
	"logdownloader/web"

	"github.com/google/uuid"
)

// version — build paytida `-ldflags "-X main.version=..."` bilan beriladi.
var version = "dev"

func main() {
	cfg := config.Load()

	s, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
	}
	// Oldingi ishga tushishdan qolgan uzilgan joblar va orphan fayllarni tozalash.
	if interrupted, orphans := s.RecoverJobs(); interrupted > 0 || orphans > 0 {
		log.Printf("Recovered jobs: %d interrupted marked failed, %d orphan export file(s) removed", interrupted, orphans)
	}

	secret := cfg.JWTSecret
	if secret == "" {
		secret, err = auth.LoadOrCreateSecret(filepath.Join(cfg.DataDir, "jwt_secret"))
		if err != nil {
			log.Fatalf("failed to load/create jwt secret: %v", err)
		}
	}
	auth.SetSecret(secret)

	// Create default admin user if no users exist
	if len(s.GetUsers()) == 0 {
		hash, _ := auth.HashPassword("admin")
		s.SaveUser(model.User{
			ID:       uuid.NewString(),
			Username: "admin",
			Password: hash,
			Role:     model.RoleAdmin,
		})
		log.Println("Created default admin user (admin/admin)")
	}

	notifier := notify.New(s)
	w := worker.New(s, int64(cfg.ExportChunkMB)*1024*1024, notifier)
	w.Start()
	if cfg.ExportChunkMB > 0 {
		log.Printf("Export auto-chunking: %d MB per part inside .tar.gz; max concurrent exports: %d", cfg.ExportChunkMB, s.MaxConcurrent())
	} else {
		log.Printf("Export auto-chunking disabled (export_chunk_mb=0); max concurrent exports: %d", s.MaxConcurrent())
	}

	sched := scheduler.New(s, w)
	sched.Start()

	h := handler.New(s, w, notifier, sched, version)
	h.SetExportChunkMB(cfg.ExportChunkMB)

	// Fon ishlari: vaqti chiqqan share'lar (har 1 daqiqa) va retention (har 10 daqiqa).
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if n := s.CleanupExpiredShares(); n > 0 {
				log.Printf("Cleaned up %d expired share(s)", n)
			}
		}
	}()
	go func() {
		run := func() {
			st := s.GetSettings()
			quota := int64(st.Retention.QuotaGB * 1024 * 1024 * 1024)
			if n := s.PruneExports(st.Retention.Days, quota); n > 0 {
				log.Printf("Retention: removed %d export job(s) (days=%d quota=%.1fGB)", n, st.Retention.Days, st.Retention.QuotaGB)
			}
		}
		run()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Serve embedded frontend
	distFS, err := fs.Sub(web.StaticFiles, "dist")
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		f, err := distFS.Open(path[1:])
		if err != nil {
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(rw, r)
	})

	// Middleware zanjiri: metrics → CORS → auth. User har so'rovda store'dan
	// qayta o'qiladi — o'chirilgan user tokeni darhol yaroqsiz.
	wrapped := metrics.HTTPMiddleware(corsMiddleware(auth.Middleware(mux, s.GetUser)))

	listenAddr := cfg.Host + ":" + cfg.Port
	if cfg.Host == "" {
		fmt.Printf("LogDownloader %s listening on %s (all interfaces)\n", version, listenAddr)
	} else {
		fmt.Printf("LogDownloader %s listening on http://%s:%s\n", version, cfg.Host, cfg.Port)
	}
	log.Fatal(http.ListenAndServe(listenAddr, wrapped))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
