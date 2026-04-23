package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/pkg/api"
	"backend/pkg/config"
	"backend/pkg/convert"
)

func main() {
	configPath := flag.String("config", "", "Path to config.yaml file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[main] Failed to load config: %v", err)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("[main] Starting Apica Search Engine on port %s", cfg.Port)
	log.Printf("[main] Data directory: %s", cfg.DataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[main] Converting Parquet files to JSON...")
	if err := convert.Run(); err != nil {
		log.Fatalf("[main] Failed to convert Parquet files: %v", err)
	}

	// build handler — loads + indexes all records at startup
	handler, err := api.New(ctx, cfg.DataDir, cfg.NumWorkers)
	if err != nil {
		log.Fatalf("[main] Failed to initialize: %v", err)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// start server in background
	go func() {
		log.Printf("[main] Server listening on http://localhost:%s", cfg.Port)
		log.Printf("[main] Endpoints:")
		log.Printf("[main]   GET  /health")
		log.Printf("[main]   GET  /api/search?q=<query>&limit=20&offset=0")
		log.Printf("[main]   GET  /api/stats")
		log.Printf("[main]   POST /api/reload")
		log.Printf("[main]   POST /api/upload (form-data: file=<parquet_file>)")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] Server error: %v", err)
		}
	}()

	// wait for shutdown signal
	<-sigCh
	signal.Stop(sigCh)
	cancel()

	log.Println("[main] Shutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("[main] Forced shutdown: %v", err)
	}
	log.Println("[main] Server stopped")
}
