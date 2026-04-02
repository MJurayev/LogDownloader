package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"logdownloader/internal/config"
	"logdownloader/internal/handler"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"
	"logdownloader/web"
)

func main() {
	cfg := config.Load()

	s, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
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
		// Try serving the file; if not found, serve index.html (SPA fallback)
		path := r.URL.Path
		f, err := distFS.Open(path[1:]) // strip leading /
		if err != nil {
			// SPA fallback
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(rw, r)
	})

	// CORS middleware for development
	wrapped := corsMiddleware(mux)

	fmt.Printf("LogDownloader started on http://localhost:%s\n", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, wrapped))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
