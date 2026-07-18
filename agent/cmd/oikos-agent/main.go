// Command oikos-agent is the read-mostly host agent for the Oikos fleet
// control panel. One static binary, one unit file — the entire footprint on a
// guest (CLAUDE.md §2, §4).
//
// Startup order is deliberate: load and strictly validate config, then run the
// bind guard, and only then open a socket. The agent binds to the resolved
// WireGuard address and refuses to start if that address is unspecified,
// loopback, or outside the configured subnet (CLAUDE.md §4.1).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/x2core/mrhandly/agent/internal/api"
	"github.com/x2core/mrhandly/agent/internal/audit"
	"github.com/x2core/mrhandly/agent/internal/config"
	"github.com/x2core/mrhandly/agent/internal/docker"
	"github.com/x2core/mrhandly/agent/internal/fingerprint"
	"github.com/x2core/mrhandly/agent/internal/journal"
	"github.com/x2core/mrhandly/agent/internal/metrics"
	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/sampler"
	"github.com/x2core/mrhandly/agent/internal/systemd"
)

// Build metadata, injected at link time via -ldflags -X (see the Makefile and
// the release workflow).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "/etc/oikos/agent.toml", "path to the agent TOML config")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("oikos-agent %s (commit %s, built %s, %s %s/%s)\n",
			version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log, *configPath); err != nil {
		log.Error("agent failed to start", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(log *slog.Logger, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// The bind guard: resolve the WireGuard interface and refuse any address
	// that would widen exposure beyond the tunnel. This must happen before any
	// socket is opened.
	bindAddr, err := config.ResolveBindAddr(cfg, config.DefaultInterfaceAddrs)
	if err != nil {
		// Loud, unambiguous refusal — this is the single most important guard
		// in the project.
		return err
	}
	listenAddr := net.JoinHostPort(bindAddr.String(), strconv.Itoa(cfg.ListenPort))

	fp := fingerprint.Detect(fingerprint.Options{
		Root:         "/",
		Arch:         runtime.GOARCH,
		DockerSocket: cfg.DockerSocket,
	})

	// One collector drives the stream sampler; one-shot reads use throwaway
	// collectors so they stay concurrency-safe.
	streamCollector := metrics.NewCollector("/", time.Now)
	metricSource := sampler.New(sampler.Config[protocol.Metrics]{
		Interval: cfg.SampleInterval.Std(),
		Sample:   streamCollector.Collect,
		Prime:    streamCollector.Reset,
		OnError: func(err error) {
			log.Warn("metrics sample failed", slog.String("error", err.Error()))
		},
	})
	oneShot := func() (protocol.Metrics, error) {
		return metrics.NewCollector("/", time.Now).Collect()
	}

	// The process table is the expensive read, so it samples at 2s and only
	// while a client watches it (CLAUDE.md §8).
	procCollector := metrics.NewProcessCollector("/")
	processSource := sampler.New(sampler.Config[[]protocol.Process]{
		Interval: 2 * time.Second,
		Sample:   procCollector.Collect,
		Prime:    procCollector.Reset,
		OnError: func(err error) {
			log.Warn("process sample failed", slog.String("error", err.Error()))
		},
	})
	processesOneShot := func() ([]protocol.Process, error) {
		return metrics.NewProcessCollector("/").Collect()
	}

	deps := api.Deps{
		Config:           cfg,
		Fingerprint:      fp,
		Version:          version,
		Commit:           commit,
		Metrics:          metricSource,
		OneShot:          oneShot,
		Audit:            audit.New(os.Stderr),
		Processes:        processSource,
		ProcessesOneShot: processesOneShot,
	}

	// Systemd is capability-gated: if the host isn't running systemd, the
	// service routes report systemd_unavailable and the agent doesn't care.
	// Same binary everywhere.
	if fp.Systemd() {
		conn, err := systemd.NewConn()
		if err != nil {
			log.Warn("systemd detected but D-Bus connect failed; service routes disabled",
				slog.String("error", err.Error()))
		} else {
			defer conn.Close()
			mgr := systemd.NewManager(conn, cfg.UnitWritable, cfg.UnitReadable)
			deps.Services = mgr
			deps.ServicesStream = sampler.NewEvent(mgr.Producer())
			deps.Journal = journal.New(nil)
		}
	}

	// Docker is capability-gated but re-probed lazily: construct the client
	// unconditionally and let each request's dial decide. On a host without a
	// socket, routes return docker_unavailable; if a socket later appears, they
	// start working with no restart. The socket is root-equivalent, so writes
	// obey docker_read_only (SECURITY.md).
	deps.Docker = docker.New(cfg.DockerSocket, cfg.DockerWritable())
	deps.DockerWritable = cfg.DockerWritable()

	srv := api.New(deps)

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: srv.Handler(),
		// SSE streams are long-lived, so no write timeout; keep a read-header
		// timeout so a slow-loris peer cannot pin a goroutine pre-handler.
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Info("agent listening",
		slog.String("addr", listenAddr),
		slog.String("interface", cfg.Interface),
		slog.String("subnet", cfg.Subnet().String()),
		slog.Int("peers", len(cfg.Peers)),
		slog.Bool("systemd", fp.Systemd()),
		slog.Bool("docker", fp.Docker()),
		slog.Bool("docker_writable", cfg.DockerWritable()),
		slog.String("version", version),
	)

	// Bind explicitly so a failure to bind the WireGuard address is a hard
	// startup error, not a background retry.
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
