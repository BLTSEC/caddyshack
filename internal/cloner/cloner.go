package cloner

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const maxAssetSize = 50 * 1024 * 1024 // 50 MB per asset

// Cloner fetches and stores a clone of a target web page and its assets.
type Cloner struct {
	client    *http.Client
	targetURL string
	cloneDir  string
	userAgent string
	verbose   bool
}

// New constructs a Cloner.
func New(targetURL, cloneDir, userAgent string, insecure, verbose bool) *Cloner {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
	}
	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	return &Cloner{
		client:    client,
		targetURL: targetURL,
		cloneDir:  cloneDir,
		userAgent: userAgent,
		verbose:   verbose,
	}
}

// Clone fetches the target page, downloads all assets, rewrites URLs, and
// writes index.html to the clone directory.
func (c *Cloner) Clone(ctx context.Context) error {
	assetsDir := filepath.Join(c.cloneDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("create assets dir: %w", err)
	}

	htmlBytes, err := c.fetch(ctx, c.targetURL)
	if err != nil {
		return fmt.Errorf("fetch target page: %w", err)
	}

	base, err := url.Parse(c.targetURL)
	if err != nil {
		return fmt.Errorf("parse target URL: %w", err)
	}

	assets := ExtractAssetURLs(string(htmlBytes), base)
	if c.verbose {
		fmt.Printf("[v] Found %d assets to download\n", len(assets))
	}

	for i := range assets {
		if err := c.downloadAsset(ctx, &assets[i], assetsDir); err != nil {
			if c.verbose {
				fmt.Printf("[v] Asset skipped (non-fatal): %s: %v\n", assets[i].AbsoluteURL, err)
			}
		}
	}

	modified := RewriteAssetURLs(string(htmlBytes), assets)

	indexPath := filepath.Join(c.cloneDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(modified), 0644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	if c.verbose {
		fmt.Printf("[v] Wrote cloned page to %s\n", indexPath)
	}
	return nil
}

func (c *Cloner) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

func (c *Cloner) downloadAsset(ctx context.Context, asset *Asset, assetsDir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.AbsoluteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	localPath := filepath.Join(assetsDir, asset.LocalPath)
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, io.LimitReader(resp.Body, maxAssetSize))
	return err
}
