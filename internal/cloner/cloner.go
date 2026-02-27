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
	"strconv"
	"strings"
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
		// Throttle requests to avoid triggering CDN rate limits (429)
		time.Sleep(100 * time.Millisecond)
	}

	// Process CSS sub-assets: rewrite url() references inside downloaded CSS files.
	for i := range assets {
		if !assets[i].Downloaded || !strings.HasSuffix(assets[i].LocalPath, ".css") {
			continue
		}
		cssPath := filepath.Join(assetsDir, assets[i].LocalPath)
		cssBytes, err := os.ReadFile(cssPath)
		if err != nil {
			continue
		}
		cssBase, err := url.Parse(assets[i].AbsoluteURL)
		if err != nil {
			continue
		}
		cssAssets := ExtractCSSURLs(string(cssBytes), cssBase)
		if c.verbose {
			fmt.Printf("[v] CSS %s: found %d sub-assets\n", assets[i].LocalPath, len(cssAssets))
		}
		for j := range cssAssets {
			if err := c.downloadCSSAsset(ctx, &cssAssets[j], assetsDir); err != nil {
				if c.verbose {
					fmt.Printf("[v] CSS sub-asset skipped: %s: %v\n", cssAssets[j].AbsoluteURL, err)
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		rewritten := RewriteCSSURLs(string(cssBytes), cssAssets)
		if err := os.WriteFile(cssPath, []byte(rewritten), 0644); err != nil && c.verbose {
			fmt.Printf("[v] Failed to rewrite CSS %s: %v\n", assets[i].LocalPath, err)
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
	if err := c.downloadToFile(ctx, asset.AbsoluteURL, asset.LocalPath, assetsDir); err != nil {
		return err
	}
	asset.Downloaded = true
	return nil
}

func (c *Cloner) downloadCSSAsset(ctx context.Context, asset *CSSAsset, assetsDir string) error {
	if err := c.downloadToFile(ctx, asset.AbsoluteURL, asset.LocalPath, assetsDir); err != nil {
		return err
	}
	asset.Downloaded = true
	return nil
}

func (c *Cloner) downloadToFile(ctx context.Context, absoluteURL, localName, assetsDir string) error {
	const maxRetries = 1

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, absoluteURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Referer", c.targetURL)
		req.Header.Set("Accept", "*/*")

		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			resp.Body.Close()
			wait := 3 * time.Second
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil && secs > 0 && secs <= 30 {
					wait = time.Duration(secs) * time.Second
				}
			}
			if c.verbose {
				fmt.Printf("[v] 429 on %s, retrying in %v\n", localName, wait)
			}
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		f, err := os.Create(filepath.Join(assetsDir, localName))
		if err != nil {
			resp.Body.Close()
			return err
		}

		_, err = io.Copy(f, io.LimitReader(resp.Body, maxAssetSize))
		resp.Body.Close()
		f.Close()
		return err
	}

	return fmt.Errorf("HTTP %d after %d retries", http.StatusTooManyRequests, maxRetries)
}
