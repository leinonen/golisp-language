package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSseStreamsEvents(t *testing.T) {
	h := func(req Request) Response {
		ch := make(chan any, 4)
		ch <- "plain"
		ch <- map[string]any{"event": "tick", "id": 7, "data": map[string]any{"n": 1}}
		ch <- map[string]any{"data": "multi\nline"}
		close(ch)
		return SseResponse(ch)
	}
	srv := httptest.NewServer(RingToHTTP(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	got := b.String()
	want := "data: plain\n\n" +
		"event: tick\nid: 7\ndata: {\"n\":1}\n\n" +
		"data: multi\ndata: line\n\n"
	if got != want {
		t.Errorf("stream = %q, want %q", got, want)
	}
}

func TestSseClientDisconnectClosesDone(t *testing.T) {
	gotDone := make(chan struct{})
	h := func(req Request) Response {
		ch := make(chan any)
		done := Done(req)
		go func() {
			<-done
			close(gotDone)
			close(ch)
		}()
		return SseResponse(ch)
	}
	srv := httptest.NewServer(RingToHTTP(h))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	r, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck
	cancel()                // client walks away

	select {
	case <-gotDone:
	case <-time.After(2 * time.Second):
		t.Fatal("req[\"done\"] did not close on client disconnect")
	}
}

func TestFormatSseEvent(t *testing.T) {
	got := formatSseEvent(map[string]any{"event": "x", "retry": 1000, "data": "d"})
	want := "event: x\nretry: 1000\ndata: d\n\n"
	if got != want {
		t.Errorf("formatSseEvent = %q, want %q", got, want)
	}
	// map without "data" key: whole map is the JSON payload
	got = formatSseEvent(map[string]any{"n": 1})
	want = "data: {\"n\":1}\n\n"
	if got != want {
		t.Errorf("formatSseEvent = %q, want %q", got, want)
	}
}

func TestDoneIsCachedAndSafeWithoutAdapter(t *testing.T) {
	// Outside the adapter there is no request context: the channel must
	// still be usable (it just never closes), and repeat calls must return
	// the same channel.
	req := Request{}
	d1 := Done(req)
	d2 := Done(req)
	if d1 != d2 {
		t.Error("Done not cached: two calls returned different channels")
	}
	select {
	case <-d1:
		t.Error("Done channel closed without a request context")
	default:
	}
}

func TestSseKeepaliveComment(t *testing.T) {
	h := func(req Request) Response {
		ch := make(chan any)
		done := Done(req)
		go func() { <-done; close(ch) }()
		resp := SseResponse(ch)
		resp["keepalive"] = 0.05 // 50ms, exercises the float64 path
		return resp
	}
	srv := httptest.NewServer(RingToHTTP(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), ": keepalive") {
		t.Errorf("expected keepalive comment on idle stream, got %q", buf[:n])
	}
}

func TestSseKeepaliveDisabled(t *testing.T) {
	resp := Response{"keepalive": 0}
	if d := sseKeepalive(resp); d != 0 {
		t.Errorf("keepalive 0 should disable, got %v", d)
	}
	if d := sseKeepalive(Response{}); d != sseKeepaliveDefault {
		t.Errorf("default keepalive = %v, want %v", d, sseKeepaliveDefault)
	}
}

func TestGoRecoverContainsPanic(t *testing.T) {
	doneCh := make(chan any)
	GoRecover(func() any {
		defer close(doneCh)
		panic("producer blew up")
	})
	select {
	case <-doneCh:
		// reaching here means the panic was recovered and the process survived
	case <-time.After(2 * time.Second):
		t.Fatal("GoRecover goroutine did not finish")
	}
}
