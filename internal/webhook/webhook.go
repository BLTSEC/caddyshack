package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier sends credential payloads to a configured webhook URL.
type Notifier struct {
	url    string
	client *http.Client
}

// New returns a Notifier, or nil if url is empty (nil-safe Send).
func New(url string) *Notifier {
	if url == "" {
		return nil
	}
	return &Notifier{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send POSTs payload as JSON to the webhook URL. Errors are logged but non-fatal.
func (n *Notifier) Send(ctx context.Context, payload any) {
	if n == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("[webhook] marshal error: %v\n", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[webhook] request error: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		fmt.Printf("[webhook] send error: %v\n", err)
		return
	}
	defer resp.Body.Close()
}
