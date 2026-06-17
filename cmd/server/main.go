// Command server runs the Zerops backup → S3 exporter dashboard.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/config"
	"recipe-s3-exporter/internal/crypto"
	"recipe-s3-exporter/internal/db"
	"recipe-s3-exporter/internal/scheduler"
	"recipe-s3-exporter/internal/web"
	"recipe-s3-exporter/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cipher, err := crypto.New(cfg.MasterKey)
	if err != nil {
		return err
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Connection pool for application queries.
	store, err := db.Connect(rootCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(rootCtx); err != nil {
		return err
	}
	// Clean up any runs orphaned by a previous crash/restart.
	if err := store.MarkStaleRunsFailed(rootCtx); err != nil {
		log.Printf("warn: could not reset stale runs: %v", err)
	}

	// Separate database/sql handle for the session store (pgx stdlib driver).
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	authSvc := auth.New(sqlDB, store, cfg.SecureCookies)
	if err := authSvc.BootstrapAdmin(rootCtx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Printf("warn: admin bootstrap: %v", err)
	} else if cfg.AdminEmail != "" {
		log.Printf("admin bootstrap checked for %s", cfg.AdminEmail)
	}

	// Worker pool.
	pool := worker.NewPool(store, cipher, cfg.ZeropsAPI, cfg.Workers)
	pool.Start(rootCtx, cfg.Workers)

	// Scheduler.
	sched := scheduler.New(store, pool)
	if err := sched.Start(rootCtx); err != nil {
		log.Printf("warn: scheduler start: %v", err)
	}
	defer sched.Stop()

	// HTTP server.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           web.NewServer(cfg, store, cipher, authSvc, pool, sched).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on http://0.0.0.0:%s (%d workers)", cfg.Port, cfg.Workers)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server error: %v", err)
			stop()
		}
	}()

	<-rootCtx.Done()
	log.Println("shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	return nil
}
