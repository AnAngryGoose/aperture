// Command hub runs the aperture web server. In v0.1 it also embeds the
// local metrics collector and docker client, so a single `aperture-hub`
// process is all that's needed to monitor the machine it runs on.
//
// When remote agents land, this binary stays the central hub; remote hosts
// will register via the same hub.MetricSource interface but over a network
// transport (cmd/agent will be the long-running process on those hosts).
package main

import (
	"context"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aperture/aperture/internal/alerts"
	"github.com/aperture/aperture/internal/api"
	"github.com/aperture/aperture/internal/collector"
	"github.com/aperture/aperture/internal/compose"
	"github.com/aperture/aperture/internal/dockerctl"
	"github.com/aperture/aperture/internal/hub"
	"github.com/aperture/aperture/internal/store"
)

// Version identifies the running binary. Bump alongside changelog entries.
// Surfaced via /api/system/info and the layout footer.
const Version = "0.4.0-alpha.2"

func main() {
	var (
		listenAddr = flag.String("listen", envOr("APERTURE_LISTEN", ":8080"), "HTTP listen address")
		dbPath     = flag.String("db", envOr("APERTURE_DB", "aperture.db"), "SQLite database path")
		interval   = flag.Duration("interval", parseDurEnv("APERTURE_INTERVAL", 5*time.Second), "metric sample interval")
		retain     = flag.Duration("retain", parseDurEnv("APERTURE_RETAIN", 14*24*time.Hour), "metric retention; 0 = forever")
		diskPath   = flag.String("disk-path", envOr("APERTURE_DISK_PATH", "/"), "filesystem root to report disk usage for")
		webDir     = flag.String("web-dir", envOr("APERTURE_WEB_DIR", ""), "directory of built SvelteKit assets to serve at /; empty = API-only")
	)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	startedAt := time.Now().UTC()
	log.Info("aperture hub starting", "version", Version, "db", *dbPath, "listen", *listenAddr)

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	h := hub.New(hub.Config{Store: st, Logger: log, Retain: *retain})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ev, err := alerts.New(ctx, st, log)
	if err != nil {
		log.Error("init alert evaluator", "err", err)
		os.Exit(1)
	}
	h.SetEvaluator(ev)

	notif := alerts.NewNotifier(st, log)
	ev.SetNotifier(notif)

	go func() {
		if err := h.Run(ctx); err != nil {
			log.Error("hub run", "err", err)
		}
	}()

	// Prune expired sessions in the background.
	go api.PruneSessions(ctx, st)

	// Register the local machine as a metric source.
	local := collector.NewLocal(*interval)
	local.DiskPath = *diskPath
	hostID, err := h.RegisterSource(ctx, local)
	if err != nil {
		log.Error("register local source", "err", err)
		os.Exit(1)
	}

	// Attach the local docker socket as the docker/terminal provider for this host.
	if dc, err := dockerctl.New(hostID); err != nil {
		log.Warn("docker unavailable", "err", err)
	} else if err := dc.Ping(ctx); err != nil {
		log.Warn("docker ping failed; container endpoints will return errors", "err", err)
		_ = dc.Close()
	} else {
		h.RegisterDocker(hostID, dc)
		h.RegisterTerminal(hostID, hub.NewLocalTerminalProvider(dc))
		log.Info("docker provider registered", "host_id", hostID)

		// Attach the local compose provider (requires docker to be available).
		if lc, err := compose.NewLocal(); err != nil {
			log.Warn("compose not available", "err", err)
		} else {
			h.RegisterCompose(hostID, lc)
			log.Info("compose provider registered", "host_id", hostID)
		}
	}

	agentH := hub.NewAgentHandler(h, st, log)

	var webFS fs.FS
	if *webDir != "" {
		if _, err := os.Stat(*webDir); err != nil {
			log.Warn("web-dir not found, serving API only", "dir", *webDir, "err", err)
		} else {
			webFS = os.DirFS(*webDir)
			log.Info("serving frontend", "dir", *webDir)
		}
	}
	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           api.NewServer(h, ev, notif, agentH, Version, startedAt).Router(webFS),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("listening", "addr", *listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseDurEnv(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
