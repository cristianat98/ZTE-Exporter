package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	listenAddr := flag.String("web.listen-address", "0.0.0.0:9111", "Address to listen on for telemetry")
	flag.Parse()

	registry := prometheus.NewRegistry()
	registry.MustRegister(buildInfoCollector())

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: mux,
	}

	log.Printf("Starting ZTE exporter version=%s on %s", Version, *listenAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %s", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down")
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
