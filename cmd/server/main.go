package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	personalserver "personal-server"
	"personal-server/internal/auth"
	"personal-server/internal/config"
	"personal-server/internal/db"
	"personal-server/internal/docker"
	"personal-server/internal/files"
	"personal-server/internal/media"
	"personal-server/internal/monitor"
	"personal-server/internal/mounts"
	"personal-server/internal/server"
	"personal-server/internal/settings"
	"personal-server/internal/share"
	"personal-server/internal/terminal"
	"personal-server/internal/wargame"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to YAML configuration file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	conn, err := db.Open(cfg.Storage.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = conn.Close() }()

	authSvc, err := auth.NewService(conn, cfg)
	if err != nil {
		log.Fatalf("init auth service: %v", err)
	}

	jail := files.NewJail(cfg.Storage.Roots)
	mountSvc, err := mounts.NewService(conn, jail, cfg.Storage.Roots)
	if err != nil {
		log.Fatalf("init mounts service: %v", err)
	}
	started := time.Now()
	cacheDir := filepath.Join(filepath.Dir(cfg.Storage.DatabasePath), "cache")
	filesHandler := files.NewHandler(jail, cfg.Storage.TempUploadDir)
	mediaHandler := media.NewHandler(conn, jail, cacheDir)
	shareHandler, err := share.NewHandler(conn, jail, cfg.Server.PublicBaseURL, cfg.Server.CookieSecure)
	if err != nil {
		log.Fatalf("init share handler: %v", err)
	}
	wgSvc, err := wargame.NewService(conn, jail, cfg.Wargame)
	if err != nil {
		log.Fatalf("init wargame service: %v", err)
	}
	monitorSvc := monitor.NewService(jail, started)
	terminalHandler := terminal.NewHandler(wgSvc)
	dockerSvc, err := docker.NewService(cfg.Storage.StacksDir)
	if err != nil {
		log.Fatalf("init docker service: %v", err)
	}
	brandingDir := filepath.Join(filepath.Dir(cfg.Storage.DatabasePath), "branding")
	settingsSvc, err := settings.NewService(conn, brandingDir)
	if err != nil {
		log.Fatalf("init settings service: %v", err)
	}

	webFS, err := personalserver.WebDistFS()
	if err != nil {
		log.Fatalf("load embedded web dist: %v", err)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	wgSvc.StartBackground(sigCtx)
	mediaHandler.StartBackground(sigCtx)
	monitorSvc.StartBackground(sigCtx)

	deps := server.Dependencies{
		Config:   cfg,
		Auth:     authSvc,
		Jail:     jail,
		Files:    filesHandler,
		Media:    mediaHandler,
		Share:    shareHandler,
		Mounts:   mountSvc,
		Wargame:  wgSvc,
		Monitor:  monitorSvc,
		Terminal: terminalHandler,
		Docker:   dockerSvc,
		Settings: settingsSvc,
		WebDist:  webFS,
		Started:  started,
	}

	pubSrv := &http.Server{
		Addr:              cfg.Server.PublicAddr,
		Handler:           server.NewPublicRouter(deps),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var privSrv *http.Server
	if cfg.Server.PrivateAddr != "" && cfg.Server.PrivateAddr != cfg.Server.PublicAddr {
		privSrv = &http.Server{
			Addr:              cfg.Server.PrivateAddr,
			Handler:           server.NewPrivateRouter(deps),
			ReadHeaderTimeout: 10 * time.Second,
		}
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("[Public Listener]  http://%s (NPM / External tier)", cfg.Server.PublicAddr)
		if err := pubSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if privSrv != nil {
		go func() {
			log.Printf("[Private Listener] http://%s (Tailscale / Admin tier)", cfg.Server.PrivateAddr)
			if err := privSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	select {
	case srvErr := <-errCh:
		log.Fatalf("server error: %v", srvErr)
	case <-sigCtx.Done():
		log.Println("shutting down servers...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = pubSrv.Shutdown(shutdownCtx)
	if privSrv != nil {
		_ = privSrv.Shutdown(shutdownCtx)
	}
}
