// glisp-lsp is the Language Server Protocol server for glisp (.glsp files).
// It speaks JSON-RPC 2.0 over stdio with Content-Length framing.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"golisp/internal/lsp"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	reader := bufio.NewReader(os.Stdin)
	server := lsp.NewServer()

	for {
		body, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("read: %v", err)
			continue
		}

		var req lsp.Request
		if err := json.Unmarshal(body, &req); err != nil {
			log.Printf("unmarshal: %v", err)
			continue
		}

		if req.Method == "exit" {
			os.Exit(0)
		}

		resp, notifs := server.Handle(&req)

		for _, n := range notifs {
			if err := writeMessage(os.Stdout, n); err != nil {
				log.Printf("write notification: %v", err)
			}
		}
		if resp != nil {
			if err := writeMessage(os.Stdout, resp); err != nil {
				log.Printf("write response: %v", err)
			}
		}
	}
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err == nil {
				contentLength = n
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing or zero Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeMessage(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
