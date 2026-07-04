package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/conflict"
	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/routeread"
)

// Server is the named-pipe IPC server. One Server per running core.
type Server struct {
	core   *core.Core
	log    *slog.Logger
	logFan *logFanout

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	listener net.Listener
	closed   bool
	wg       sync.WaitGroup
}

// New constructs a Server for the given core. It also wires the global log
// fanout so SubscribeLogs works as soon as Start is called.
func New(c *core.Core, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		core:   c,
		log:    log,
		logFan: newLogFanout(),
	}
}

// Start opens the named pipe and begins accepting connections.
func (s *Server) Start() error {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: SecurityDescriptor,
		MessageMode:        false, // byte stream + line-delimited JSON
		InputBufferSize:    64 * 1024,
		OutputBufferSize:   64 * 1024,
	}
	ln, err := winio.ListenPipe(PipeName, cfg)
	if err != nil {
		return fmt.Errorf("listen %s: %w", PipeName, err)
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.listener = ln
	s.wg.Add(1)
	go s.acceptLoop()
	s.log.Info("IPC server listening", "pipe", PipeName)
	return nil
}

// Stop closes the listener and waits for in-flight handlers to finish.
func (s *Server) Stop() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	s.logFan.stopAll()
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return // shutting down
			}
			s.log.Warn("ipc accept error", "err", err)
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handle(c)
		}(conn)
	}
}

// handle services one connection. Single requests are answered inline and the
// loop continues (a connection may pipeline several). A streaming request
// takes over the connection until it ends or the client disconnects (§9.4 v1:
// one stream per connection).
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			_ = WriteError(conn, "", CodeBadParams, "malformed request: "+err.Error())
			continue
		}
		if IsStreaming(req.Method) {
			s.dispatchStream(ctx, conn, req)
			return // streaming owns the connection until end
		}
		s.dispatchSingle(conn, req)
	}
}

// dispatchSingle resolves one non-streaming method to a Response.
func (s *Server) dispatchSingle(conn net.Conn, req Request) {
	result, errBody := s.handleMethod(req)
	if errBody != nil {
		_ = WriteError(conn, req.ID, errBody.Code, errBody.Message)
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		_ = WriteError(conn, req.ID, CodeInternal, err.Error())
		return
	}
	_ = WriteResponse(conn, Response{ID: req.ID, OK: true, Result: payload})
}

// handleMethod returns either (result, nil) or (nil, *ErrorBody).
func (s *Server) handleMethod(req Request) (any, *ErrorBody) {
	switch req.Method {
	case MethodGetStatus:
		return s.core.Status(), nil
	case MethodGetConfig:
		return s.core.Config(), nil
	case MethodSaveConfig:
		return s.handleSaveConfig(req)
	case MethodSetActiveProfile:
		return s.handleSetActive(req)
	case MethodApplyNow:
		return s.core.ApplyOnce("ipc"), nil
	case MethodGetRouteTable:
		return s.handleGetRouteTable(req)
	default:
		return nil, NewError(CodeUnknownMethod, "unknown method: "+req.Method)
	}
}

func (s *Server) handleSaveConfig(req Request) (any, *ErrorBody) {
	var p struct {
		Config *config.Config `json:"config"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, NewError(CodeBadParams, "decode config: "+err.Error())
	}
	if p.Config == nil {
		return nil, NewError(CodeBadParams, "missing config")
	}
	if err := s.core.SaveConfig(p.Config); err != nil {
		return nil, toErrorBody(err)
	}
	return map[string]any{"ok": true}, nil
}

func (s *Server) handleSetActive(req Request) (any, *ErrorBody) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, NewError(CodeBadParams, "decode id: "+err.Error())
	}
	if err := s.core.SetActiveProfile(p.ID); err != nil {
		return nil, NewError(CodeInvalidConfig, err.Error())
	}
	return s.core.Status(), nil
}

// routeRowOut is one row of the GetRouteTable response, tagged with its
// likely owner so the GUI Routes page can color it.
type routeRowOut struct {
	DestinationPrefix string `json:"destinationPrefix"`
	NextHop           string `json:"nextHop"`
	InterfaceIndex    int    `json:"interfaceIndex"`
	InterfaceAlias    string `json:"interfaceAlias"`
	RouteMetric       int    `json:"routeMetric"`
	InterfaceMetric   int    `json:"interfaceMetric"`
	Source            string `json:"source"` // managed | system | suspect
}

func (s *Server) handleGetRouteTable(req Request) (any, *ErrorBody) {
	rows, err := routeread.Read(s.ctx)
	if err != nil {
		return nil, NewError(CodeInternal, "read route table: "+err.Error())
	}
	managed := s.core.ManagedRoutes()
	managedSet := make(map[string]bool, len(managed))
	for _, e := range managed {
		managedSet[e.Destination] = true
	}
	// VPN interfaces from the live snapshot → suspect rows.
	st := s.core.Status()
	vpnIdx := make(map[int]bool)
	for _, ifc := range st.Interfaces {
		if conflict.IsVPNInterface(ifc) {
			vpnIdx[ifc.Index] = true
		}
	}

	out := make([]routeRowOut, 0, len(rows))
	for _, r := range rows {
		src := string(routeread.SourceSystem)
		if managedSet[r.DestinationPrefix] {
			src = string(routeread.SourceManaged)
		} else if vpnIdx[r.InterfaceIndex] {
			src = string(routeread.SourceSuspect)
		}
		out = append(out, routeRowOut{
			DestinationPrefix: r.DestinationPrefix,
			NextHop:           r.NextHop,
			InterfaceIndex:    r.InterfaceIndex,
			InterfaceAlias:    r.InterfaceAlias,
			RouteMetric:       r.RouteMetric,
			InterfaceMetric:   r.InterfaceMetric,
			Source:            src,
		})
	}
	return struct {
		Rows []routeRowOut `json:"rows"`
	}{Rows: out}, nil
}

// toErrorBody maps a domain error to an IPC error code.
func toErrorBody(err error) *ErrorBody {
	if err == nil {
		return nil
	}
	var verrs config.ValidationErrors
	if errors.As(err, &verrs) {
		return NewError(CodeInvalidConfig, verrs.Error())
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "route"), strings.Contains(msg, "netsh"):
		return NewError(CodeRouteExecFail, msg)
	}
	return NewError(CodeInternal, msg)
}

// dispatchStream resolves one streaming method; it writes data frames and an
// end marker (for finite streams) directly to conn.
func (s *Server) dispatchStream(ctx context.Context, conn net.Conn, req Request) {
	switch req.Method {
	case MethodPing:
		var p struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			_ = WriteError(conn, req.ID, CodeBadParams, err.Error())
			return
		}
		if err := runPing(ctx, conn, req.ID, p.Target); err != nil {
			_ = WriteError(conn, req.ID, CodeInternal, err.Error())
		}
	case MethodTracert:
		var p struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			_ = WriteError(conn, req.ID, CodeBadParams, err.Error())
			return
		}
		if err := runTracert(ctx, conn, req.ID, p.Target); err != nil {
			_ = WriteError(conn, req.ID, CodeInternal, err.Error())
		}
	case MethodSubscribeLogs:
		var p struct {
			Level string `json:"level"`
		}
		_ = json.Unmarshal(req.Params, &p)
		s.streamLogs(ctx, conn, req.ID, p.Level)
	case MethodSubscribeStatus:
		s.streamStatus(ctx, conn, req.ID)
	default:
		_ = WriteError(conn, req.ID, CodeUnknownMethod, "unknown stream method: "+req.Method)
	}
}

// streamLogs emits each matching log record as a stream data frame, forever,
// until the client disconnects or the server stops.
func (s *Server) streamLogs(ctx context.Context, conn net.Conn, id, levelStr string) {
	level := logging.LevelFromString(levelStr)
	sid, ch := s.logFan.subscribe(level)
	defer s.logFan.unsubscribe(sid)
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			if err := WriteStreamData(conn, id, json.RawMessage(line)); err != nil {
				return
			}
		}
	}
}

// streamStatus sends the current status then each subsequent change, forever.
func (s *Server) streamStatus(ctx context.Context, conn net.Conn, id string) {
	first := s.core.Status()
	if err := WriteStreamData(conn, id, first); err != nil {
		return
	}
	events := make(chan core.StatusResponse, 32)
	unsub := s.core.SubscribeStatus(func(st core.StatusResponse) {
		select {
		case events <- st:
		default: // drop on slow consumer; next apply resyncs
		}
	})
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case st, ok := <-events:
			if !ok {
				return
			}
			if err := WriteStreamData(conn, id, st); err != nil {
				return
			}
		}
	}
}
