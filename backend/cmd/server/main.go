// Command server is the Origin Proof API: origin prediction, plan search and
// precedent verification for CBP substantial-transformation cases.
//
// Endpoints so far:
//
//	GET /health  liveness check (not /healthz: Cloud Run reserves paths ending in "z")
package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	logger.Info("listening", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
