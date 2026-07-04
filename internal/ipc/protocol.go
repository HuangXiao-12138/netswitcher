// Package ipc implements the named-pipe control protocol between the
// NetSwitcher service and the GUI / CLI clients (spec §9).
//
// Transport: Windows named pipe \\.\pipe\NetSwitcher, byte stream with one
// JSON object per line. ACL allows SYSTEM, Administrators, and the local
// interactive user (D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)) — no token.
//
// Methods (§9.3): GetStatus, GetConfig, SaveConfig, SetActiveProfile,
// ApplyNow, GetRouteTable are single request/response. Ping, Tracert,
// SubscribeLogs, SubscribeStatus stream multiple responses until end /
// disconnect (§9.4).
package ipc

import (
	"encoding/json"
	"fmt"
	"io"
)

// PipeName is the canonical pipe path.
const PipeName = `\\.\pipe\NetSwitcher`

// SecurityDescriptor grants full access to SYSTEM, Administrators, and the
// local interactive user (spec §9.1). The IU ACE excludes remote callers
// because named pipes are local-only by default; the explicit SD makes the
// intent obvious and lockable.
const SecurityDescriptor = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)"

// Request is one client→server message.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ErrorBody carries a code (machine-readable) and a message (human-readable).
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is one server→client message (single response shape).
// Streaming responses are written by writeStreamData / writeStreamEnd which
// hand-marshal the discriminated "stream" field (bool true vs "end") per §9.2.
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorBody      `json:"error,omitempty"`
}

// Error codes (spec §9.3).
const (
	CodeIfaceNotFound = "IFACE_NOT_FOUND"
	CodeInvalidCIDR   = "INVALID_CIDR"
	CodeInvalidConfig = "INVALID_CONFIG"
	CodeRouteExecFail = "ROUTE_EXEC_FAILED"
	CodeInternal      = "INTERNAL"
	CodeServiceBusy   = "SERVICE_BUSY"
	CodeUnknownMethod = "UNKNOWN_METHOD"
	CodeBadParams     = "BAD_PARAMS"
	CodeCancelled     = "CANCELLED"
)

// Method names (spec §9.3).
const (
	MethodGetStatus        = "GetStatus"
	MethodGetConfig        = "GetConfig"
	MethodSaveConfig       = "SaveConfig"
	MethodSetActiveProfile = "SetActiveProfile"
	MethodApplyNow         = "ApplyNow"
	MethodGetRouteTable    = "GetRouteTable"
	MethodPing             = "Ping"
	MethodTracert          = "Tracert"
	MethodSubscribeLogs    = "SubscribeLogs"
	MethodSubscribeStatus  = "SubscribeStatus"
)

// IsStreaming reports whether a method emits a stream of responses.
func IsStreaming(method string) bool {
	switch method {
	case MethodPing, MethodTracert, MethodSubscribeLogs, MethodSubscribeStatus:
		return true
	}
	return false
}

// EncodeLine marshals v and appends a newline.
func EncodeLine(v any) ([]byte, error) {
	bs, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(bs, '\n'), nil
}

// WriteResponse writes a single (non-stream) Response.
func WriteResponse(w io.Writer, r Response) error {
	r.OK = r.Error == nil
	line, err := EncodeLine(r)
	if err != nil {
		return err
	}
	_, err = w.Write(line)
	return err
}

// WriteError is shorthand for a failure response.
func WriteError(w io.Writer, id, code, message string) error {
	return WriteResponse(w, Response{
		ID:    id,
		Error: &ErrorBody{Code: code, Message: message},
	})
}

// WriteStreamData writes one stream data frame: {"id":..,"stream":true,"data":..}.
// Per §9.2 the "stream" field is boolean true for data frames.
func WriteStreamData(w io.Writer, id string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("{\"id\":%q,\"stream\":true,\"data\":%s}\n", id, payload)
	_, err = w.Write([]byte(line))
	return err
}

// WriteStreamEnd writes the terminal marker: {"id":..,"stream":"end"}.
func WriteStreamEnd(w io.Writer, id string) error {
	line := fmt.Sprintf("{\"id\":%q,\"stream\":\"end\"}\n", id)
	_, err := w.Write([]byte(line))
	return err
}

// NewError constructs an ErrorBody.
func NewError(code, message string) *ErrorBody {
	return &ErrorBody{Code: code, Message: message}
}
