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

// writeClientText sends a masked text frame (client → server frames must be masked).
func writeClientText(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	payload := []byte(msg)
	if len(payload) > 125 {
		t.Fatal("test helper supports short frames only")
	}
	frame := []byte{0x81, byte(0x80 | len(payload))}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
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
