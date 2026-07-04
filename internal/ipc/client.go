package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
)

// CallError carries an IPC error code so callers can branch on it.
type CallError struct {
	Code    string
	Message string
}

func (e *CallError) Error() string { return e.Code + ": " + e.Message }

// AsCallError extracts a *CallError if err is one.
func AsCallError(err error) *CallError {
	var ce *CallError
	if errors.As(err, &ce) {
		return ce
	}
	return nil
}

// Client connects to the service's named pipe. Methods are safe for
// concurrent use; each Call/Stream opens its own connection (§9.4).
type Client struct {
	PipeName string
	Timeout  time.Duration
	id       atomic.Uint64
}

// NewClient returns a Client targeting the default pipe.
func NewClient() *Client {
	return &Client{PipeName: PipeName, Timeout: 3 * time.Second}
}

func (c *Client) nextID() string {
	return fmt.Sprintf("c%d", c.id.Add(1))
}

func (c *Client) dial() (net.Conn, error) {
	name := c.PipeName
	if name == "" {
		name = PipeName
	}
	to := c.Timeout
	if to <= 0 {
		to = 3 * time.Second
	}
	conn, err := winio.DialPipe(name, &to)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w (service running?)", name, err)
	}
	return conn, nil
}

// Call sends one request and returns the raw result payload.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := writeRequest(conn, c.nextID(), method, params); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, &CallError{Code: resp.Error.Code, Message: resp.Error.Message}
	}
	return resp.Result, nil
}

// CallJSON sends one request and unmarshals the result into out.
func (c *Client) CallJSON(method string, params any, out any) error {
	raw, err := c.Call(method, params)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Frame is one decoded stream message.
type Frame struct {
	Data json.RawMessage // present for data frames
	End  bool            // true on the terminal marker
}

// Stream opens a streaming request, returning a channel of frames and a
// channel that emits the final error (nil on clean stream:end). The data
// channel closes after the end marker.
func (c *Client) Stream(method string, params any) (<-chan Frame, <-chan error, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, nil, err
	}
	if err := writeRequest(conn, c.nextID(), method, params); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	frames := make(chan Frame, 32)
	errCh := make(chan error, 1)
	go func() {
		defer conn.Close()
		defer close(frames)
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			f, err := decodeFrame(scanner.Bytes())
			if err != nil {
				errCh <- err
				return
			}
			if f.End {
				errCh <- nil
				return
			}
			select {
			case frames <- f:
			default: // drop on slow consumer
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			errCh <- err
			return
		}
		errCh <- io.ErrUnexpectedEOF // connection closed without end marker
	}()
	return frames, errCh, nil
}

// writeRequest serializes params (if any) and writes one request line.
func writeRequest(w io.Writer, id, method string, params any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		bs, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsRaw = bs
	}
	req := Request{ID: id, Method: method, Params: paramsRaw}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = w.Write(line)
	return err
}

// decodeFrame parses one stream message, handling the discriminated "stream"
// field (bool true → data, string "end" → terminal).
func decodeFrame(line []byte) (Frame, error) {
	var probe struct {
		Stream json.RawMessage `json:"stream"`
		Data   json.RawMessage `json:"data"`
		Error  *ErrorBody      `json:"error"`
		ID     string          `json:"id"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return Frame{}, fmt.Errorf("decode stream frame: %w", err)
	}
	if probe.Error != nil {
		return Frame{}, &CallError{Code: probe.Error.Code, Message: probe.Error.Message}
	}
	s := string(probe.Stream)
	switch s {
	case `"end"`:
		return Frame{End: true}, nil
	case "true":
		// Raw JSON boolean true token.
		return Frame{Data: probe.Data}, nil
	}
	// Older/unknown shape: treat as data if any.
	if len(probe.Data) > 0 {
		return Frame{Data: probe.Data}, nil
	}
	return Frame{End: true}, nil
}
