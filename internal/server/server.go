// Package server provides the HTTP server exposing /metrics, /health, /ready, and /config endpoints.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/collector"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
)

// Server is the HTTP server that exposes Prometheus metrics and operational endpoints.
type Server struct {
	httpServer *http.Server
	registry   *collector.Registry
	config     *config.Config
	ready      atomic.Bool
	logger     *logrus.Entry
}

// NewServer creates a new HTTP server configured from cfg. The provided registry
// is used as the Prometheus collector source for the /metrics endpoint.
func NewServer(cfg *config.Config, registry *collector.Registry, logger *logrus.Entry) *Server {
	s := &Server{
		registry: registry,
		config:   cfg,
		logger:   logger.WithField("component", "server"),
	}

	mux := http.NewServeMux()

	// --- Prometheus metrics ---
	promRegistry := prometheus.NewRegistry()
	promRegistry.MustRegister(registry)
	// Also register default Go and process collectors.
	promRegistry.MustRegister(collectors.NewGoCollector())
	promRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	mux.Handle("/metrics", promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// --- Health / readiness ---
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)

	// --- Config (redacted) ---
	mux.HandleFunc("/config", s.handleConfig)

	// --- pprof ---
	if cfg.Server.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		s.logger.Info("pprof endpoints enabled under /debug/pprof/")
	}

	s.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddress,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start begins serving HTTP in a background goroutine. It returns immediately.
// If the server fails to start (other than http.ErrServerClosed), the error is
// logged at Fatal level.
func (s *Server) Start(ctx context.Context) error {
	s.logger.WithField("addr", s.httpServer.Addr).Info("starting HTTP server")

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("HTTP server error")
			errCh <- err
		}
		close(errCh)
	}()

	// Give the listener a moment to bind; surface immediate errors.
	select {
	case err := <-errCh:
		return fmt.Errorf("server failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		// Likely started successfully.
	}

	return nil
}

// Stop performs a graceful shutdown of the HTTP server. The provided context
// controls the maximum time to wait for in-flight requests to complete.
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// SetReady updates the readiness state exposed by the /ready endpoint.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// --- HTTP handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	// Return a redacted copy of the configuration so that secrets are not
	// exposed through the /config endpoint.
	redacted := *s.config
	redacted.GitLab.Token = "***REDACTED***"
	if redacted.Redis.URL != "" {
		redacted.Redis.URL = "***REDACTED***"
	}
	if redacted.Server.Webhook.SecretToken != "" {
		redacted.Server.Webhook.SecretToken = "***REDACTED***"
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(redacted); err != nil {
		s.logger.WithError(err).Error("failed to encode config")
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
