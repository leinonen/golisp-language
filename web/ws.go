package web

// PROTOTYPE for the web-enhancements exploration — minimal RFC 6455
// websocket server support, dependency-free. Text messages only; ping is
// answered with pong; fragmented messages are reassembled; close is
// negotiated. No extensions (permessage-deflate), no subprotocols.
//
// glisp usage:
//
//	(web/get "/ws" (web/websocket
//	  (fn [req web/Request in (chan any) out (chan any)] -> any
//	    (for-chan [msg in]
//	      (send! out (str "echo: " msg))))))

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
)

// WsHandler receives the request map plus an inbound and an outbound
// message channel. Inbound text messages arrive on in (closed when the
// client disconnects); values sent on out are written as text frames.
// When the handler returns, the connection is closed.
type WsHandler func(req Request, in chan any, out chan any) any

// Websocket wraps a WsHandler as a Ring handler whose response instructs
// the adapter to upgrade the connection.
func Websocket(h WsHandler) Handler {
	return func(req Request) Response {
		return map[string]any{"websocket": h}
	}
}

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// upgradeWs performs the RFC 6455 handshake and runs the handler.
// Returns true if it handled the response.
func upgradeWs(w http.ResponseWriter, r *http.Request, req Request, resp Response) bool {
	h, ok := resp["websocket"].(WsHandler)
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

	in := make(chan any, 8)
	out := make(chan any, 8)
	writeDone := make(chan struct{})

	// Writer: serializes all frames onto the connection.
	frames := make(chan wsFrame, 8)
	go func() {
		defer close(writeDone)
		for f := range frames {
			if err := writeFrame(conn, f); err != nil {
				return
			}
		}
	}()
	// Out pump: handler messages → text frames.
	outClosed := make(chan struct{})
	go func() {
		defer close(outClosed)
		for v := range out {
			s, ok := v.(string)
			if !ok {
				s = fmt.Sprintf("%v", v)
			}
			frames <- wsFrame{opcode: 0x1, payload: []byte(s)}
		}
	}()
	// Reader: client frames → in channel; answers ping; detects close.
	go func() {
		defer close(in)
		var msg []byte
		var msgOp byte
		br := bufio.NewReader(conn)
		for {
			f, err := readFrame(br)
			if err != nil {
				return
			}
			switch f.opcode {
			case 0x8: // close — echo it back, then stop reading
				frames <- wsFrame{opcode: 0x8, payload: f.payload}
				return
			case 0x9: // ping
				frames <- wsFrame{opcode: 0xA, payload: f.payload}
			case 0xA: // pong — ignore
			case 0x0: // continuation
				msg = append(msg, f.payload...)
				if f.fin {
					if msgOp == 0x1 {
						in <- string(msg)
					}
					msg = nil
				}
			default: // text or binary
				if f.fin {
					if f.opcode == 0x1 {
						in <- string(f.payload)
					}
				} else {
					msgOp = f.opcode
					msg = append([]byte{}, f.payload...)
				}
			}
		}
	}()

	h(req, in, out)

	// Handler done: close the socket politely.
	frames <- wsFrame{opcode: 0x8, payload: []byte{0x03, 0xE8}} // 1000 normal
	close(frames)
	<-writeDone
	return true
}

type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

func readFrame(r *bufio.Reader) (wsFrame, error) {
	var f wsFrame
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return f, err
	}
	f.fin = hdr[0]&0x80 != 0
	f.opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	n := uint64(hdr[1] & 0x7F)
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
		return f, errors.New("ws: frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return f, err
		}
	} else {
		// RFC 6455 §5.1: client frames MUST be masked.
		return f, errors.New("ws: unmasked client frame")
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
