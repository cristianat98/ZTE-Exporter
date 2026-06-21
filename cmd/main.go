package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cristianat98/zte-exporter/internal/collector"
	"github.com/cristianat98/zte-exporter/internal/config"
)

func main() {
	listenAddr := flag.String("web.listen-address", "0.0.0.0:9111", "Address to listen on for telemetry")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	registry := prometheus.NewRegistry()
	registry.MustRegister(buildInfoCollector())
	registry.MustRegister(collector.New(cfg))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: mux,
	}

	slog.Info("starting ZTE exporter", "version", Version, "address", *listenAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	_ = srv.Shutdown(context.Background())
}

func buildInfoCollector() prometheus.Collector {
	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zte_exporter_build_info",
			Help: "A metric with a constant '1' value labeled by version.",
		},
		[]string{"version"},
	)
	g.WithLabelValues(Version).Set(1)
	return g
}
