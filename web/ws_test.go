package web

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wsTestEcho is the handler under test: prefixes every message with "echo: ".
func wsTestEcho(req Request, in chan any, out chan any) any {
	out <- "welcome"
	for msg := range in {
		out <- fmt.Sprintf("echo: %v", msg)
	}
	close(out)
	return nil
}

// dialWs performs a raw RFC 6455 client handshake against a test server.
func dialWs(t *testing.T, srvURL string) (net.Conn, *bufio.Reader) {
	t.Helper()
	addr := strings.TrimPrefix(srvURL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", addr, key)
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("handshake status = %q (err=%v)", status, err)
	}
	wantAccept := func() string {
		s := sha1.Sum([]byte(key + wsGUID))
		return base64.StdEncoding.EncodeToString(s[:])
	}()
	gotAccept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Sec-WebSocket-Accept: "); ok {
			gotAccept = strings.TrimSpace(v)
		}
	}
	if gotAccept != wantAccept {
		t.Fatalf("Sec-WebSocket-Accept = %q, want %q", gotAccept, wantAccept)
	}
	return conn, br
}

// writeClientFrame sends one masked frame (client → server frames must be
// masked). Supports short payloads only (≤125 bytes).
func writeClientFrame(t *testing.T, conn net.Conn, fin bool, opcode byte, payload []byte) {
	t.Helper()
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	if len(payload) > 125 {
		t.Fatal("test helper supports short frames only")
	}
	b0 := opcode
	if fin {
		b0 |= 0x80
	}
	frame := []byte{b0, byte(0x80 | len(payload))}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
}

// writeClientText sends a masked text frame.
func writeClientText(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	writeClientFrame(t, conn, true, 0x1, []byte(msg))
}

// readServerFrame reads one (unmasked, short) frame from the server.
func readServerFrame(t *testing.T, br *bufio.Reader) (opcode byte, payload []byte) {
	t.Helper()
	var hdr [2]byte
	if _, err := br.Read(hdr[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := br.Read(hdr[1:]); err != nil {
		t.Fatal(err)
	}
	n := int(hdr[1] & 0x7F)
	payload = make([]byte, n)
	for read := 0; read < n; {
		k, err := br.Read(payload[read:])
		if err != nil {
			t.Fatal(err)
		}
		read += k
	}
	return hdr[0] & 0x0F, payload
}

func TestWebsocketEcho(t *testing.T) {
	srv := httptest.NewServer(RingToHTTP(Routes(Get("/ws", Websocket(wsTestEcho)))))
	defer srv.Close()

	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck

	if op, p := readServerFrame(t, br); op != 0x1 || string(p) != "welcome" {
		t.Fatalf("first frame = op %#x %q", op, p)
	}
	writeClientText(t, conn, "hi")
	if op, p := readServerFrame(t, br); op != 0x1 || string(p) != "echo: hi" {
		t.Fatalf("echo frame = op %#x %q", op, p)
	}
	// ping → pong with same payload
	mask := [4]byte{1, 2, 3, 4}
	ping := []byte{0x89, 0x83, mask[0], mask[1], mask[2], mask[3],
		'a' ^ mask[0], 'b' ^ mask[1], 'c' ^ mask[2]}
	if _, err := conn.Write(ping); err != nil {
		t.Fatal(err)
	}
	if op, p := readServerFrame(t, br); op != 0xA || string(p) != "abc" {
		t.Fatalf("pong frame = op %#x %q", op, p)
	}
}

func TestWebsocketRejectsBadHandshake(t *testing.T) {
	srv := httptest.NewServer(RingToHTTP(Get("/ws", Websocket(wsTestEcho)).handler))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/ws") // no Upgrade headers
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestWebsocketRejectsUnmaskedClientFrame(t *testing.T) {
	received := make(chan any, 1)
	h := func(req Request, in chan any, out chan any) any {
		for msg := range in {
			received <- msg
		}
		close(received)
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()

	conn, _ := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	// Unmasked text frame — server must drop the connection (RFC 6455 §5.1).
	if _, err := conn.Write([]byte{0x81, 0x02, 'h', 'i'}); err != nil {
		t.Fatal(err)
	}
	if msg, ok := <-received; ok {
		t.Fatalf("server accepted unmasked frame: %v", msg)
	}
}

// expectClose reads frames until a close frame arrives and returns its code.
func expectClose(t *testing.T, br *bufio.Reader) uint16 {
	t.Helper()
	for i := 0; i < 8; i++ {
		op, p := readServerFrame(t, br)
		if op != 0x8 {
			continue // skip data/ping frames preceding the close
		}
		if len(p) < 2 {
			return 0
		}
		return uint16(p[0])<<8 | uint16(p[1])
	}
	t.Fatal("no close frame received")
	return 0
}

func TestWebsocketClosesWhenHandlerForgetsOut(t *testing.T) {
	// The handler returns without closing out — the connection must still
	// close politely (close frame 1000) and not leak the out pump.
	h := func(req Request, in chan any, out chan any) any {
		out <- "bye"
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	if op, p := readServerFrame(t, br); op != 0x1 || string(p) != "bye" {
		t.Fatalf("first frame = op %#x %q", op, p)
	}
	if code := expectClose(t, br); code != 1000 {
		t.Errorf("close code = %d, want 1000", code)
	}
}

func TestWebsocketFragmentedMessage(t *testing.T) {
	srv := httptest.NewServer(RingToHTTP(Websocket(wsTestEcho)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close()     //nolint:errcheck
	readServerFrame(t, br) // welcome
	writeClientFrame(t, conn, false, 0x1, []byte("hel"))
	writeClientFrame(t, conn, true, 0x0, []byte("lo"))
	if op, p := readServerFrame(t, br); op != 0x1 || string(p) != "echo: hello" {
		t.Fatalf("echo frame = op %#x %q", op, p)
	}
}

func TestWebsocketBinaryMessage(t *testing.T) {
	h := func(req Request, in chan any, out chan any) any {
		for msg := range in {
			out <- msg // []byte goes back out as a binary frame
		}
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	writeClientFrame(t, conn, true, 0x2, []byte{0x00, 0xFF, 0x10})
	if op, p := readServerFrame(t, br); op != 0x2 || len(p) != 3 || p[1] != 0xFF {
		t.Fatalf("binary echo = op %#x % x", op, p)
	}
}

func TestWebsocketInvalidUtf8Fails1007(t *testing.T) {
	received := make(chan any, 1)
	h := func(req Request, in chan any, out chan any) any {
		for msg := range in {
			received <- msg
		}
		close(received)
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	writeClientFrame(t, conn, true, 0x1, []byte{0xFF, 0xFE, 0xFD})
	if code := expectClose(t, br); code != 1007 {
		t.Errorf("close code = %d, want 1007", code)
	}
	if msg, ok := <-received; ok {
		t.Fatalf("invalid UTF-8 text delivered to handler: %v", msg)
	}
}

func TestWebsocketMessageTooBigFails1009(t *testing.T) {
	h := func(req Request) Response {
		// Raw response map: exercises the unwrapped-func handler path and
		// the max-message override together.
		var f func(Request, chan any, chan any) any = func(req Request, in chan any, out chan any) any {
			for range in {
			}
			return nil
		}
		return Response{"websocket": f, "max-message": 8}
	}
	srv := httptest.NewServer(RingToHTTP(h))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close()                                         //nolint:errcheck
	writeClientFrame(t, conn, true, 0x1, []byte("0123456789")) // 10 > 8
	if code := expectClose(t, br); code != 1009 {
		t.Errorf("close code = %d, want 1009", code)
	}
}

func TestWebsocketCloseCodeEchoed(t *testing.T) {
	h := func(req Request, in chan any, out chan any) any {
		for range in {
		}
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	// close with code 1001 (going away) + reason text
	writeClientFrame(t, conn, true, 0x8, append([]byte{0x03, 0xE9}, "bye"...))
	if code := expectClose(t, br); code != 1001 {
		t.Errorf("close code = %d, want 1001 echoed back", code)
	}
}

func TestWebsocketOversizedControlFrameFails1002(t *testing.T) {
	h := func(req Request, in chan any, out chan any) any {
		for range in {
		}
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	// A ping with a 126-byte payload needs the 16-bit length form, which is
	// itself the violation (control frames are capped at 125 bytes).
	payload := make([]byte, 126)
	mask := [4]byte{1, 2, 3, 4}
	frame := []byte{0x89, 0x80 | 126, 0, 126}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
	if code := expectClose(t, br); code != 1002 {
		t.Errorf("close code = %d, want 1002", code)
	}
}

func TestWebsocketContinuationWithoutStartFails1002(t *testing.T) {
	h := func(req Request, in chan any, out chan any) any {
		for range in {
		}
		return nil
	}
	srv := httptest.NewServer(RingToHTTP(Websocket(h)))
	defer srv.Close()
	conn, br := dialWs(t, srv.URL)
	defer conn.Close() //nolint:errcheck
	writeClientFrame(t, conn, true, 0x0, []byte("orphan"))
	if code := expectClose(t, br); code != 1002 {
		t.Errorf("close code = %d, want 1002", code)
	}
}
