package cloner

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// Asset represents a discovered external resource in the cloned page.
type Asset struct {
	OriginalURL string
	AbsoluteURL string
	LocalPath   string // filename under assets/
	Tag         string
	Attr        string
}

// ExtractAssetURLs parses HTML and returns all external assets referenced in
// <link>, <script>, <img>, <video>, <audio>, <source>, and <track> tags.
func ExtractAssetURLs(htmlContent string, base *url.URL) []Asset {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	seen := map[string]bool{}
	var assets []Asset

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}

		tok := tokenizer.Token()
		switch tok.Data {
		case "link":
			if src := attrVal(tok, "href"); src != "" && !isDataURI(src) {
				if a := makeAsset(src, base, tok.Data, "href"); a != nil && !seen[a.OriginalURL] {
					seen[a.OriginalURL] = true
					assets = append(assets, *a)
				}
			}
		case "script", "img", "video", "audio", "source", "track":
			if src := attrVal(tok, "src"); src != "" && !isDataURI(src) {
				if a := makeAsset(src, base, tok.Data, "src"); a != nil && !seen[a.OriginalURL] {
					seen[a.OriginalURL] = true
					assets = append(assets, *a)
				}
			}
			if srcset := attrVal(tok, "srcset"); srcset != "" {
				for _, src := range parseSrcset(srcset) {
					if !isDataURI(src) {
						if a := makeAsset(src, base, tok.Data, "srcset"); a != nil && !seen[a.OriginalURL] {
							seen[a.OriginalURL] = true
							assets = append(assets, *a)
						}
					}
				}
			}
		}
	}
	return assets
}

// RewriteAssetURLs replaces original asset URLs in HTML with local /assets/ paths.
func RewriteAssetURLs(htmlContent string, assets []Asset) string {
	for _, a := range assets {
		if a.LocalPath == "" {
			continue
		}
		htmlContent = strings.ReplaceAll(htmlContent, a.OriginalURL, "/assets/"+a.LocalPath)
	}
	return htmlContent
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
	if ext == "" {
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
