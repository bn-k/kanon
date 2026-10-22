// Package lsp serves Kanon documents and schemas to an editor through the Language Server Protocol: diagnostics as
// the text is typed, go to definition, references, hover and symbols.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// delay is how long the server waits after an edit before it checks the documents again and publishes what it found.
const delay = 300 * time.Millisecond

// Serve answers the editor that writes to in and reads out, until it says exit or closes in.
func Serve(in io.Reader, out io.Writer) error {
	return serve(in, out, delay)
}

func serve(in io.Reader, out io.Writer, delay time.Duration) error {
	s := newServer(out)
	msgs := make(chan *message)
	failed := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		r := bufio.NewReader(in)
		for {
			m, err := read(r)
			if err != nil {
				failed <- err
				return
			}
			select {
			case msgs <- m:
			case <-done:
				return
			}
		}
	}()
	var due <-chan time.Time
	for {
		select {
		case m := <-msgs:
			if m.Method == "exit" {
				return nil
			}
			edits := s.edits
			if err := s.handle(m); err != nil {
				return err
			}
			if s.edits != edits {
				due = time.After(delay)
			}
		case <-due:
			due = nil
			if err := s.publish(); err != nil {
				return err
			}
		case err := <-failed:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// message is a request or a notification from the editor. err is set when its body isn't JSON.
type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	err    error
}

// request reports whether m awaits an answer: a notification has no id.
func (m *message) request() bool {
	return len(m.ID) > 0 && string(m.ID) != "null"
}

// read reads one message.
func read(r *bufio.Reader) (*message, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	m := &message{}
	if err := json.Unmarshal(body, m); err != nil {
		return &message{ID: json.RawMessage("null"), err: err}, nil
	}
	return m, nil
}

// readBody reads the body of one message: headers, a blank line, then Content-Length bytes.
func readBody(r *bufio.Reader) ([]byte, error) {
	length := -1
	for started := false; ; started = true {
		line, err := r.ReadString('\n')
		if err == io.EOF && (started || line != "") {
			return nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, _ := strings.Cut(line, ":")
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if length, err = strconv.Atoi(strings.TrimSpace(value)); err != nil || length < 0 {
				return nil, fmt.Errorf("a message with a bad Content-Length: %q", strings.TrimSpace(value))
			}
		}
	}
	if length < 0 {
		return nil, errors.New("a message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type failure struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *rpcError       `json:"error"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// rpcError is an error the editor is told about, with a JSON-RPC code.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

const (
	parseError     = -32700
	methodNotFound = -32601
	invalidParams  = -32602
	internalError  = -32603
)

// write sends v as one message.
func write(out io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

// decode reads the params of a message into a T.
func decode[T any](params json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(params, &v); err != nil {
		return v, &rpcError{invalidParams, err.Error()}
	}
	return v, nil
}

// toPath returns the path a file URI names.
func toPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", &rpcError{invalidParams, err.Error()}
	}
	if u.Scheme != "file" {
		return "", &rpcError{invalidParams, fmt.Sprintf("not a file: %s", uri)}
	}
	return filepath.Clean(filepath.FromSlash(u.Path)), nil
}

// toURI returns the file URI of path.
func toURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
