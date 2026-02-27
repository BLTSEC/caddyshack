package cloner

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var (
	// cssURLRegex matches url("..."), url('...'), url(...) — captures the URL in group 1.
	cssURLRegex = regexp.MustCompile(`url\(\s*['"]?(.*?)['"]?\s*\)`)
	// cssImportRegex matches @import "..." and @import '...' — captures the URL in group 1.
	cssImportRegex = regexp.MustCompile(`@import\s+['"]([^'"]*)['"]`)
)

// Asset represents a discovered external resource in the cloned page.
type Asset struct {
	OriginalURL string
	AbsoluteURL string
	LocalPath   string // filename under assets/
	Tag         string
	Attr        string
	Downloaded  bool
}

// CSSAsset represents a resource discovered inside a CSS file.
type CSSAsset struct {
	OriginalRef string // the full matched token, e.g. `url("foo.png")`
	RawURL      string // the URL string within the token
	AbsoluteURL string
	LocalPath   string
	Downloaded  bool
}

// ExtractAssetURLs parses HTML and returns all external assets referenced in
// tags, inline <style> blocks, and style="" attributes.
func ExtractAssetURLs(htmlContent string, base *url.URL) []Asset {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	seen := map[string]bool{}
	var assets []Asset
	inStyle := false

	addFromURL := func(rawURL, tag, attr string) {
		if rawURL == "" || isDataURI(rawURL) || strings.HasPrefix(rawURL, "#") {
			return
		}
		if a := makeAsset(rawURL, base, tag, attr); a != nil && !seen[a.OriginalURL] {
			seen[a.OriginalURL] = true
			assets = append(assets, *a)
		}
	}

	addSrcset := func(srcset, tag string) {
		for _, src := range parseSrcset(srcset) {
			addFromURL(src, tag, "srcset")
		}
	}

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.EndTagToken:
			tok := tokenizer.Token()
			if tok.Data == "style" {
				inStyle = false
			}

		case html.TextToken:
			if !inStyle {
				continue
			}
			tok := tokenizer.Token()
			for _, m := range cssURLRegex.FindAllStringSubmatch(tok.Data, -1) {
				addFromURL(m[1], "style", "url")
			}

		case html.StartTagToken, html.SelfClosingTagToken:
			tok := tokenizer.Token()

			// Inline style="" on any element.
			if styleAttr := attrVal(tok, "style"); styleAttr != "" {
				for _, m := range cssURLRegex.FindAllStringSubmatch(styleAttr, -1) {
					addFromURL(m[1], "inline-style", "url")
				}
			}

			switch tok.Data {
			case "style":
				if tt == html.StartTagToken {
					inStyle = true
				}
			case "link":
				addFromURL(attrVal(tok, "href"), tok.Data, "href")
			case "script", "img", "audio", "source", "track":
				addFromURL(attrVal(tok, "src"), tok.Data, "src")
				addSrcset(attrVal(tok, "srcset"), tok.Data)
			case "video":
				addFromURL(attrVal(tok, "src"), tok.Data, "src")
				addFromURL(attrVal(tok, "poster"), tok.Data, "poster")
				addSrcset(attrVal(tok, "srcset"), tok.Data)
			case "meta":
				switch attrVal(tok, "property") {
				case "og:image", "og:video", "og:audio", "twitter:image":
					addFromURL(attrVal(tok, "content"), tok.Data, "content")
				}
			}
		}
	}
	return assets
}

// RewriteAssetURLs replaces original asset URLs in HTML with local /assets/ paths.
// Assets that were not successfully downloaded keep their original URLs.
func RewriteAssetURLs(htmlContent string, assets []Asset) string {
	for _, a := range assets {
		if a.LocalPath == "" || !a.Downloaded {
			continue
		}
		replacement := "/assets/" + a.LocalPath
		htmlContent = strings.ReplaceAll(htmlContent, a.OriginalURL, replacement)
		// The HTML tokenizer decodes &amp; → &, but the raw HTML still has &amp;.
		// Replace the entity-encoded form too so dynamic URLs (e.g. /w/load.php?a=1&b=2) are rewritten.
		if strings.Contains(a.OriginalURL, "&") {
			encoded := strings.ReplaceAll(a.OriginalURL, "&", "&amp;")
			htmlContent = strings.ReplaceAll(htmlContent, encoded, replacement)
		}
	}
	return htmlContent
}

// ExtractCSSURLs finds all url() and @import references in CSS text, resolving
// relative URLs against cssBaseURL (the URL of the CSS file itself).
func ExtractCSSURLs(css string, cssBaseURL *url.URL) []CSSAsset {
	seen := map[string]bool{}
	var assets []CSSAsset

	add := func(originalRef, rawURL string) {
		if rawURL == "" || isDataURI(rawURL) || strings.HasPrefix(rawURL, "#") {
			return
		}
		u, err := url.Parse(rawURL)
		if err != nil {
			return
		}
		abs := cssBaseURL.ResolveReference(u)
		if abs.Scheme != "http" && abs.Scheme != "https" {
			return
		}
		absStr := abs.String()
		if seen[absStr] {
			return
		}
		seen[absStr] = true

		ext := path.Ext(abs.Path)
		if len(ext) > 8 {
			ext = ext[:8]
		}
		hash := sha256.Sum256([]byte(absStr))
		localPath := fmt.Sprintf("%x%s", hash[:6], ext)

		assets = append(assets, CSSAsset{
			OriginalRef: originalRef,
			RawURL:      rawURL,
			AbsoluteURL: absStr,
			LocalPath:   localPath,
		})
	}

	for _, m := range cssURLRegex.FindAllStringSubmatch(css, -1) {
		add(m[0], m[1])
	}
	for _, m := range cssImportRegex.FindAllStringSubmatch(css, -1) {
		add(m[0], m[1])
	}
	return assets
}

// RewriteCSSURLs replaces original url()/import references in CSS with local /assets/ paths.
// Only rewrites assets that were successfully downloaded.
func RewriteCSSURLs(css string, assets []CSSAsset) string {
	for _, a := range assets {
		if !a.Downloaded {
			continue
		}
		// Replace the URL value within the original reference to preserve quote style.
		replacement := strings.Replace(a.OriginalRef, a.RawURL, "/assets/"+a.LocalPath, 1)
		css = strings.ReplaceAll(css, a.OriginalRef, replacement)
	}
	return css
}

func makeAsset(rawURL string, base *url.URL, tag, attr string) *Asset {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	abs := base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return nil
	}

	ext := path.Ext(abs.Path)
	// Dynamic resource loaders (e.g. /load.php?only=styles) have misleading extensions.
	// Fall back to tag-based defaults for common cases.
	if ext == "" || ext == ".php" || ext == ".asp" || ext == ".aspx" || ext == ".jsp" {
		switch tag {
		case "link":
			ext = ".css"
		case "script":
			ext = ".js"
		case "img":
			ext = ".png"
		}
	}
	// Cap extension length to avoid surprises
	if len(ext) > 8 {
		ext = ext[:8]
	}

	hash := sha256.Sum256([]byte(abs.String()))
	localPath := fmt.Sprintf("%x%s", hash[:6], ext)

	return &Asset{
		OriginalURL: rawURL,
		AbsoluteURL: abs.String(),
		LocalPath:   localPath,
		Tag:         tag,
		Attr:        attr,
	}
}

func attrVal(tok html.Token, name string) string {
	for _, a := range tok.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func isDataURI(s string) bool {
	return strings.HasPrefix(s, "data:")
}

// parseSrcset extracts URLs from a srcset attribute value.
func parseSrcset(srcset string) []string {
	var urls []string
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) > 0 {
			urls = append(urls, fields[0])
		}
	}
	return urls
}
