package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rxtx-hosting/flowlens/internal/config"
	"github.com/rxtx-hosting/flowlens/pkg/docker"
	"github.com/rxtx-hosting/flowlens/pkg/ebpf"
	"github.com/rxtx-hosting/flowlens/pkg/estimator"
	"github.com/rxtx-hosting/flowlens/pkg/exporter"
)

var (
	configPath = flag.String("config", "/etc/flowlens/config.yaml", "Path to configuration file")
	ifaceName  = flag.String("interface", "", "Network interface to monitor (overrides config)")
)

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))

	if *ifaceName != "" {
		cfg.Interface = *ifaceName
	}

	slog.Info("Starting FlowLens", "interface", cfg.Interface)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dockerClient, err := docker.NewClient(cfg.DockerLabels, cfg.ServerIDSource, cfg.PortEnvVar)
	if err != nil {
		log.Fatalf("Failed to initialize Docker client: %v", err)
	}
	defer dockerClient.Close()

	ebpfMonitor, err := ebpf.NewMonitor(cfg.Interface)
	if err != nil {
		log.Fatalf("Failed to initialize eBPF monitor: %v", err)
	}
	defer ebpfMonitor.Close()

	playerEstimator := estimator.NewEstimator(cfg.PlayerActivityThreshold, cfg.MinPacketsThreshold, cfg.MinBytesThreshold)

	apiServer := exporter.NewAPIServer(cfg.APIKey)

	go func() {
		slog.Info("Starting API server", "address", cfg.ServerAddr)
		if err := apiServer.StartServer(cfg.ServerAddr); err != nil {
			log.Fatalf("Failed to start API server: %v", err)
		}
	}()

	var promExporter *exporter.PrometheusExporter
	if cfg.PrometheusAddr != "" {
		promExporter = exporter.NewPrometheusExporter()
		go func() {
			slog.Info("Starting Prometheus server", "address", cfg.PrometheusAddr)
			if err := promExporter.StartServer(cfg.PrometheusAddr); err != nil {
				log.Fatalf("Failed to start Prometheus server: %v", err)
			}
		}()
	}

	discoveryTicker := time.NewTicker(cfg.DiscoveryInterval)
	defer discoveryTicker.Stop()

	metricsTicker := time.NewTicker(cfg.MetricsInterval)
	defer metricsTicker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("FlowLens started successfully")

	for {
		select {
		case <-sigCh:
			slog.Info("Received shutdown signal, cleaning up...")
			return

		case <-discoveryTicker.C:
			servers, err := dockerClient.DiscoverGameServers(ctx)
			if err != nil {
				slog.Error("Error discovering game servers", "error", err)
				continue
			}
			slog.Info("Discovered game servers", "count", len(servers))
			playerEstimator.UpdateServerMap(servers)

		case <-metricsTicker.C:
			flows, err := ebpfMonitor.ReadFlows()
			if err != nil {
				slog.Error("Error reading flows", "error", err)
				continue
			}

			stats := playerEstimator.EstimatePlayers(flows)
			slog.Info("Estimated players", "servers", len(stats))

			apiServer.UpdateStats(stats)
			if promExporter != nil {
				promExporter.UpdateStats(stats)
			}
		}
	}
}
