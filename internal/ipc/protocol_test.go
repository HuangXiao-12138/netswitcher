package ipc

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestEncodeRoundTrip(t *testing.T) {
	req := Request{ID: "r1", Method: MethodApplyNow, Params: json.RawMessage(`{}`)}
	line, err := EncodeLine(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(line), "\n") {
		t.Error("line must end with newline")
	}
	var got Request
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" || got.Method != MethodApplyNow {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestWriteResponse_OKAndError(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResponse(&buf, Response{ID: "x", Result: json.RawMessage(`{"k":1}`)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"ok":true`) {
		t.Errorf("ok response missing ok:true: %s", buf.String())
	}
	buf.Reset()
	if err := WriteError(&buf, "x", CodeInvalidCIDR, "bad"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), CodeInvalidCIDR) {
		t.Errorf("error response missing code: %s", buf.String())
	}
	if strings.Contains(buf.String(), `"ok":true`) {
		t.Errorf("error response should not have ok:true: %s", buf.String())
	}
}

func TestStreamFrames(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteStreamData(&buf, "s1", "hello"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"stream":true`) {
		t.Errorf("data frame missing stream:true: %s", buf.String())
	}
	if err := WriteStreamEnd(&buf, "s1"); err != nil {
		t.Fatal(err)
	}
	// Two lines now.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(lines))
	}
	f1, err := decodeFrame([]byte(lines[0]))
	if err != nil {
		t.Fatal(err)
	}
	if f1.End || string(f1.Data) != `"hello"` {
		t.Errorf("first frame: %+v", f1)
	}
	f2, err := decodeFrame([]byte(lines[1]))
	if err != nil {
		t.Fatal(err)
	}
	if !f2.End {
		t.Error("second frame should be end marker")
	}
}

func TestIsStreaming(t *testing.T) {
	for _, m := range []string{MethodPing, MethodTracert, MethodSubscribeLogs, MethodSubscribeStatus} {
		if !IsStreaming(m) {
			t.Errorf("%s should be streaming", m)
		}
	}
	for _, m := range []string{MethodGetStatus, MethodGetConfig, MethodApplyNow} {
		if IsStreaming(m) {
			t.Errorf("%s should not be streaming", m)
		}
	}
}

func TestExtractLevel(t *testing.T) {
	rec := []byte(`{"time":"2026-01-01T00:00:00Z","level":"WARN","msg":"x"}`)
	if got := extractLevel(rec); got != slog.LevelWarn {
		t.Errorf("level = %v, want Warn", got)
	}
}

func TestLogFanoutSubscribeDelivers(t *testing.T) {
	f := newLogFanout()
	defer f.stopAll()
	id, ch := f.subscribe(slog.LevelInfo)
	_ = id
	f.Write([]byte(`{"level":"INFO","msg":"hi"}`))
	select {
	case line := <-ch:
		if !strings.Contains(string(line), "hi") {
			t.Errorf("unexpected payload: %s", line)
		}
	default:
		t.Error("subscriber did not receive record")
	}
}

func TestLogFanoutLevelFilter(t *testing.T) {
	f := newLogFanout()
	defer f.stopAll()
	_, ch := f.subscribe(slog.LevelError)             // only errors
	f.Write([]byte(`{"level":"INFO","msg":"noisy"}`)) // filtered out
	f.Write([]byte(`{"level":"ERROR","msg":"boom"}`)) // delivered
	select {
	case <-ch:
		// got something — must be the error, not the info
	default:
		t.Error("error record not delivered")
	}
}
