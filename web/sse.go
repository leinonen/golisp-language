package web

// PROTOTYPE for the web-enhancements exploration — server-sent events.
// A handler returns (web/sse-response ch); the Ring adapter detects the
// channel and streams events until the channel closes or the client
// disconnects. The request map carries req["done"], a chan any that closes
// on client disconnect, so producers can select! on it.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SseResponse creates a streaming response whose events are read from ch.
// Each value sent on ch becomes one SSE event: a string becomes a data line;
// a map may carry "event", "data", "id" and "retry" keys (non-string data is
// JSON-encoded). The stream ends when ch is closed or the client disconnects.
func SseResponse(ch chan any) Response {
	return map[string]any{"status": 200, "sse": ch}
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
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return true
			}
			fmt.Fprint(w, formatSseEvent(v)) //nolint:errcheck
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
