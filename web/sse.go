package web

// Server-sent events. A handler returns (web/sse-response ch); the Ring
// adapter detects the channel and streams events until the channel closes
// or the client disconnects. Producers run in their own goroutine — use
// (web/go-recover (fn [] ...)) so a panic doesn't kill the process, close
// the channel when done ((defer (close! ch)) pairs well), and race
// (web/done req) with select! so an infinite producer stops when the
// client goes away.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// sseKeepaliveDefault is how often an idle stream emits a comment line so
// proxies and load balancers don't drop the connection.
const sseKeepaliveDefault = 15 * time.Second

// SseResponse creates a streaming response whose events are read from ch.
// Each value sent on ch becomes one SSE event: a string becomes a data line;
// a map may carry "event", "data", "id" and "retry" keys (non-string data is
// JSON-encoded). The stream ends when ch is closed or the client disconnects.
// While idle, a ": keepalive" comment is written every 15 seconds; set a
// "keepalive" key on the response (seconds; 0 disables) to override.
func SseResponse(ch chan any) Response {
	return map[string]any{"status": 200, "sse": ch}
}

// sseKeepalive reads the optional "keepalive" override (in seconds) from
// the response map. 0 or negative disables keepalive comments.
func sseKeepalive(resp Response) time.Duration {
	v, ok := resp["keepalive"]
	if !ok {
		return sseKeepaliveDefault
	}
	switch n := v.(type) {
	case int:
		return time.Duration(n) * time.Second
	case int64:
		return time.Duration(n) * time.Second
	case float64:
		return time.Duration(n * float64(time.Second))
	}
	return sseKeepaliveDefault
}

// streamSse writes SSE events from ch to w until ch closes or the client
// disconnects. Returns true if it handled the response.
func streamSse(w http.ResponseWriter, r *http.Request, resp Response) bool {
	ch, ok := resp["sse"].(chan any)
	if !ok {
		return false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(500)
		fmt.Fprint(w, "streaming unsupported") //nolint:errcheck
		return true
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	if hdrs, ok := resp["headers"].(map[string]any); ok {
		for k, v := range hdrs {
			h.Set(k, fmt.Sprintf("%v", v))
		}
	}
	w.WriteHeader(statusOf(resp))
	flusher.Flush()

	var keepalive <-chan time.Time
	if d := sseKeepalive(resp); d > 0 {
		t := time.NewTicker(d)
		defer t.Stop()
		keepalive = t.C
	}
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return true
			}
			fmt.Fprint(w, formatSseEvent(v)) //nolint:errcheck
			flusher.Flush()
		case <-keepalive:
			fmt.Fprint(w, ": keepalive\n\n") //nolint:errcheck
			flusher.Flush()
		case <-r.Context().Done():
			return true
		}
	}
}

// formatSseEvent renders one value as an SSE event frame.
func formatSseEvent(v any) string {
	var b strings.Builder
	writeData := func(data any) {
		s, ok := data.(string)
		if !ok {
			enc, err := json.Marshal(data)
			if err != nil {
				s = fmt.Sprintf("%v", data)
			} else {
				s = string(enc)
			}
		}
		for _, line := range strings.Split(s, "\n") {
			b.WriteString("data: ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if m, ok := v.(map[string]any); ok {
		if ev, ok := m["event"].(string); ok {
			fmt.Fprintf(&b, "event: %s\n", ev)
		}
		if id, ok := m["id"]; ok {
			fmt.Fprintf(&b, "id: %v\n", id)
		}
		if retry, ok := m["retry"]; ok {
			fmt.Fprintf(&b, "retry: %v\n", retry)
		}
		if data, ok := m["data"]; ok {
			writeData(data)
		} else {
			writeData(m)
		}
	} else {
		writeData(v)
	}
	b.WriteByte('\n')
	return b.String()
}
