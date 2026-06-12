package web

// RFC 6455 websocket server support, dependency-free. Text messages arrive
// as strings, binary messages as []byte; fragmented messages are
// reassembled; ping is answered with pong and an idle ping keeps the
// connection alive through NATs/proxies; close is negotiated with proper
// close codes (1002 protocol error, 1007 invalid UTF-8, 1009 too big). No
// extensions (permessage-deflate) and no subprotocol negotiation.
//
// glisp usage:
//
//	(web/get "/ws" (web/websocket
//	  (fn [req web/Request in (chan any) out (chan any)] -> any
//	    (for-chan [msg in]
//	      (send! out (str "echo: " msg))))))
//
// The handler runs in the request goroutine; in closes when the client
// disconnects, values sent on out are written as text frames ([]byte
// values as binary frames), and returning from the handler closes the
// connection. Producers spawned by the handler should use web/go-recover
// and stop when in closes or (web/done req) fires.

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// WsHandler receives the request map plus an inbound and an outbound
// message channel. Inbound messages arrive on in (strings for text frames,
// []byte for binary; closed when the client disconnects); values sent on
// out are written as frames. When the handler returns, the connection is
// closed.
type WsHandler func(req Request, in chan any, out chan any) any

// wsMaxMessageDefault caps a single (possibly fragmented) message; override
// per route with a "max-message" byte count on the websocket response map.
const wsMaxMessageDefault = 1 << 20 // 1 MiB

// wsWriteTimeout bounds each frame write so a stuck client cannot wedge
// the connection's writer forever.
const wsWriteTimeout = 10 * time.Second

// wsPingInterval is how often an idle connection is pinged server-side.
const wsPingInterval = 30 * time.Second

// Websocket wraps a WsHandler as a Ring handler whose response instructs
// the adapter to upgrade the connection. To override the message size cap,
// build the response map directly: {"websocket" h "max-message" 65536}.
func Websocket(h WsHandler) Handler {
	return func(req Request) Response {
		return map[string]any{"websocket": h}
	}
}

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsHandlerOf extracts the websocket handler from a response map. A glisp
// fn arrives as the raw func type rather than the named WsHandler.
func wsHandlerOf(resp Response) (WsHandler, bool) {
	switch h := resp["websocket"].(type) {
	case WsHandler:
		return h, true
	case func(Request, chan any, chan any) any:
		return h, true
	}
	return nil, false
}

// wsMaxMessage reads the optional "max-message" override (bytes).
func wsMaxMessage(resp Response) int {
	switch n := resp["max-message"].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return wsMaxMessageDefault
}

// upgradeWs performs the RFC 6455 handshake and runs the handler.
// Returns true if it handled the response.
func upgradeWs(w http.ResponseWriter, r *http.Request, req Request, resp Response) bool {
	h, ok := wsHandlerOf(resp)
	if !ok {
		return false
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!headerContainsToken(r.Header.Get("Connection"), "upgrade") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" ||
		r.Header.Get("Sec-WebSocket-Key") == "" {
		w.WriteHeader(400)
		fmt.Fprint(w, "bad websocket handshake") //nolint:errcheck
		return true
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		w.WriteHeader(500)
		fmt.Fprint(w, "hijacking unsupported") //nolint:errcheck
		return true
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return true
	}
	defer conn.Close() //nolint:errcheck

	sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err := rw.Flush(); err != nil {
		return true
	}

	wc := &wsConn{conn: conn}
	in := make(chan any, 8)
	out := make(chan any, 8)
	stop := make(chan struct{})

	// Out pump: handler messages → data frames. The stop case prevents a
	// goroutine leak when the handler returns without closing out; anything
	// already buffered at stop time is still flushed before the close frame.
	outDone := make(chan struct{})
	sendOut := func(v any) bool {
		f := wsFrame{opcode: 0x1}
		switch p := v.(type) {
		case []byte:
			f.opcode = 0x2
			f.payload = p
		case string:
			f.payload = []byte(p)
		default:
			f.payload = []byte(fmt.Sprintf("%v", v))
		}
		return wc.send(f) == nil
	}
	go func() {
		defer close(outDone)
		for {
			select {
			case v, ok := <-out:
				if !ok {
					return
				}
				if !sendOut(v) {
					return
				}
			case <-stop:
				for { // drain what the handler queued before returning
					select {
					case v, ok := <-out:
						if !ok || !sendOut(v) {
							return
						}
					default:
						return
					}
				}
			}
		}
	}()

	// Idle ping so NATs/proxies keep the connection open.
	go func() {
		t := time.NewTicker(wsPingInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if wc.send(wsFrame{opcode: 0x9}) != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}()

	// Reader: client frames → in channel; answers ping; negotiates close.
	go func() {
		defer close(in)
		wc.readLoop(bufio.NewReader(conn), in, wsMaxMessage(resp))
	}()

	h(req, in, out)

	// Handler done: stop the pumps, then close the socket politely (a
	// no-op if the close negotiation already happened).
	close(stop)
	<-outDone
	wc.send(wsFrame{opcode: 0x8, payload: wsClosePayload(1000)}) //nolint:errcheck
	return true
}

// wsConn serializes frame writes; the writer is shared by the out pump,
// the ping ticker, and the reader (pong/close replies).
type wsConn struct {
	conn      net.Conn
	mu        sync.Mutex
	closeSent bool
}

var errWsClosed = errors.New("ws: close frame already sent")

// send writes one frame under the write lock with a deadline. After a
// close frame has been written, all further sends are rejected.
func (c *wsConn) send(f wsFrame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeSent {
		return errWsClosed
	}
	c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)) //nolint:errcheck
	if err := writeFrame(c.conn, f); err != nil {
		return err
	}
	if f.opcode == 0x8 {
		c.closeSent = true
	}
	return nil
}

// fail sends a close frame with the given code and gives up on the
// connection (used for protocol violations).
func (c *wsConn) fail(code uint16) {
	c.send(wsFrame{opcode: 0x8, payload: wsClosePayload(code)}) //nolint:errcheck
}

// wsClosePayload encodes a close frame body carrying just a status code.
func wsClosePayload(code uint16) []byte {
	return []byte{byte(code >> 8), byte(code)}
}

// readLoop processes client frames until close, protocol error, or EOF.
// Complete text messages are delivered to in as strings, binary messages
// as []byte.
func (c *wsConn) readLoop(br *bufio.Reader, in chan any, maxMessage int) {
	var msg []byte // fragmented message being assembled
	var msgOp byte // 0 when no fragmented message is in progress
	for {
		f, err := readFrame(br)
		if err != nil {
			if errors.Is(err, errWsProtocol) {
				c.fail(1002)
			}
			return
		}
		switch f.opcode {
		case 0x8: // close — echo the client's code, then stop reading
			payload := f.payload
			if len(payload) == 1 { // a 1-byte close body is malformed
				payload = wsClosePayload(1002)
			} else if len(payload) > 2 {
				payload = payload[:2] // echo the code, drop the reason
			}
			c.send(wsFrame{opcode: 0x8, payload: payload}) //nolint:errcheck
			return
		case 0x9: // ping → pong with the same payload
			if c.send(wsFrame{opcode: 0xA, payload: f.payload}) != nil {
				return
			}
		case 0xA: // pong — ignore
		case 0x0: // continuation
			if msgOp == 0 {
				c.fail(1002) // continuation with no message in progress
				return
			}
			msg = append(msg, f.payload...)
			if len(msg) > maxMessage {
				c.fail(1009)
				return
			}
			if f.fin {
				if !c.deliver(in, msgOp, msg) {
					return
				}
				msg, msgOp = nil, 0
			}
		case 0x1, 0x2: // text or binary
			if msgOp != 0 {
				c.fail(1002) // new data frame while fragmented message in progress
				return
			}
			if len(f.payload) > maxMessage {
				c.fail(1009)
				return
			}
			if f.fin {
				if !c.deliver(in, f.opcode, f.payload) {
					return
				}
			} else {
				msgOp = f.opcode
				msg = append([]byte{}, f.payload...)
			}
		default: // reserved opcode
			c.fail(1002)
			return
		}
	}
}

// deliver validates and hands one complete message to the in channel.
// Returns false when the connection should be failed.
func (c *wsConn) deliver(in chan any, opcode byte, payload []byte) bool {
	if opcode == 0x1 {
		if !utf8.Valid(payload) {
			c.fail(1007)
			return false
		}
		in <- string(payload)
		return true
	}
	in <- payload
	return true
}

type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

// errWsProtocol marks reader errors that warrant a 1002 close frame.
var errWsProtocol = errors.New("ws: protocol error")

func readFrame(r *bufio.Reader) (wsFrame, error) {
	var f wsFrame
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return f, err
	}
	f.fin = hdr[0]&0x80 != 0
	f.opcode = hdr[0] & 0x0F
	if hdr[0]&0x70 != 0 {
		// RSV bits set without a negotiated extension (RFC 6455 §5.2).
		return f, errWsProtocol
	}
	masked := hdr[1]&0x80 != 0
	n := uint64(hdr[1] & 0x7F)
	if f.opcode >= 0x8 && (!f.fin || n > 125) {
		// Control frames must not be fragmented and carry ≤125 bytes (§5.5).
		return f, errWsProtocol
	}
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return f, err
		}
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return f, err
		}
		n = binary.BigEndian.Uint64(ext[:])
	}
	if n > 1<<24 {
		return f, fmt.Errorf("ws: frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return f, err
		}
	} else {
		// RFC 6455 §5.1: client frames MUST be masked.
		return f, errWsProtocol
	}
	f.payload = make([]byte, n)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return f, err
	}
	for i := range f.payload {
		f.payload[i] ^= mask[i%4]
	}
	return f, nil
}

func writeFrame(w net.Conn, f wsFrame) error {
	var hdr []byte
	b0 := byte(0x80) | f.opcode // FIN always set — no server fragmentation
	n := len(f.payload)
	switch {
	case n < 126:
		hdr = []byte{b0, byte(n)}
	case n < 1<<16:
		hdr = []byte{b0, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0], hdr[1] = b0, 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(f.payload)
	return err
}

// headerContainsToken reports whether a comma-separated header value
// contains token (case-insensitive).
func headerContainsToken(v, token string) bool {
	for _, part := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
