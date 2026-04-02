package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"logdownloader/internal/handler"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"
)

func main() {
	dataDir := filepath.Join(".", "data")
	if d := os.Getenv("DATA_DIR"); d != "" {
		dataDir = d
	}

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	s, err := store.New(dataDir)
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
	}

	w := worker.New(s)
	h := handler.New(s, w)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Serve frontend static files
	frontendDir := filepath.Join("..", "frontend", "build")
	if _, err := os.Stat(frontendDir); err == nil {
		mux.Handle("/", http.FileServer(http.Dir(frontendDir)))
	}

	// CORS middleware for development
	wrapped := corsMiddleware(mux)

	fmt.Printf("Server started on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, wrapped))
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
