package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"logdownloader/internal/auth"
	"logdownloader/internal/config"
	"logdownloader/internal/handler"
	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"
	"logdownloader/web"

	"github.com/google/uuid"
)

func main() {
	cfg := config.Load()

	s, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
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

	w := worker.New(s)
	h := handler.New(s, w)

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

	// Auth middleware wraps CORS
	wrapped := corsMiddleware(auth.Middleware(mux))

	listenAddr := cfg.Host + ":" + cfg.Port
	if cfg.Host == "" {
		fmt.Printf("LogDownloader listening on %s (all interfaces)\n", listenAddr)
	} else {
		fmt.Printf("LogDownloader listening on http://%s:%s\n", cfg.Host, cfg.Port)
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
