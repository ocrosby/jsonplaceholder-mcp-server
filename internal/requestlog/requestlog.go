// Package requestlog records the upstream HTTP requests made while serving one
// tool call, so the MCP adapter can echo them in the result (sun-mcp's
// "api_requests"). It is context-scoped: each call gets its own Recorder.
package requestlog

import (
	"context"
	"sync"
)

type ctxKey struct{}

// Recorder collects request descriptions for one call. Safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	entries []string
}

// WithRecorder returns a child context carrying a fresh Recorder.
func WithRecorder(ctx context.Context) (context.Context, *Recorder) {
	r := &Recorder{}
	return context.WithValue(ctx, ctxKey{}, r), r
}

// Record appends entry to the Recorder in ctx; it is a no-op without one.
func Record(ctx context.Context, entry string) {
	if r, ok := ctx.Value(ctxKey{}).(*Recorder); ok {
		r.mu.Lock()
		r.entries = append(r.entries, entry)
		r.mu.Unlock()
	}
}

// Entries returns a copy of the recorded entries (never nil).
func (r *Recorder) Entries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.entries))
	copy(out, r.entries)
	return out
}
