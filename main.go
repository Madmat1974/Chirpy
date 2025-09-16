package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {

	const filepathRoot = "."

	mux := http.NewServeMux()

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	fs := http.FileServer(http.Dir(filepathRoot))
	mux.HandleFunc("/healthz", healthHandler)
	mux.Handle("/app/", http.StripPrefix("/app", fs))
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})

	fmt.Printf("Starting server on %s", server.Addr)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Server failed to start: %v", err)
	}

}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
