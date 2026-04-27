package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"project-helper/internal/ai"
	"project-helper/internal/analyzer"
	"project-helper/internal/config"
	"project-helper/internal/db"
	"project-helper/internal/httpapi"
	"project-helper/internal/repo"
	"project-helper/internal/store"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	logFile, err := config.SetupLogging(cfg.LogPath)
	if err != nil {
		log.Fatalf("setup logging: %v", err)
	}
	defer logFile.Close()
	log.Printf("logs writing to %s", cfg.LogPath)

	sqlDB, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.Migrate(sqlDB); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	st := store.New(sqlDB)
	if err := st.RecoverInterruptedRuns(context.Background()); err != nil {
		log.Fatalf("recover interrupted runs: %v", err)
	}
	repoSvc := repo.NewService(cfg.ReposDir)
	deepseek := ai.NewDeepSeekClient(cfg.DeepSeek)
	an := analyzer.New(st, repoSvc, deepseek, cfg.ReportsDir)

	router := httpapi.NewRouter(cfg, st, an, deepseek)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("project-helper listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
