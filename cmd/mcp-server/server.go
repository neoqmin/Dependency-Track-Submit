package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type Server struct {
	client *DTrackClient
	tools  []toolDef
	r      *bufio.Reader
	w      io.Writer
	// useContentLength mirrors the inbound framing for outbound responses:
	// clients using newline-delimited JSON cannot parse Content-Length frames
	// and vice versa, so we answer in whatever framing the client sent.
	useContentLength bool
}

func NewServer(client *DTrackClient) *Server {
	return &Server{
		client: client,
		tools:  allTools(client),
		r:      bufio.NewReaderSize(os.Stdin, 4*1024*1024),
		w:      os.Stdout,
	}
}

func (s *Server) Run() error {
	for {
		msg, framed, err := readFramed(s.r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		s.useContentLength = framed
		if len(bytes.TrimSpace(msg)) == 0 {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			s.writeError(nil, -32700, "parse error", nil)
			continue
		}
		s.handle(req)
	}
}

// readFramed reads one JSON-RPC message from the stream, supporting both
// transports MCP clients use over stdio:
//   - newline-delimited JSON (the MCP stdio standard; e.g. Claude Code), where
//     each message is a single line terminated by '\n'.
//   - LSP-style Content-Length framing (e.g. some clients/editors), where a
//     "Content-Length: N" header is followed by a blank line and an N-byte body.
//
// We disambiguate on the first non-empty line: if it starts with the
// Content-Length header we read a framed body; otherwise the line itself is the
// JSON message.
// Returns the message bytes and whether it arrived via Content-Length framing.
func readFramed(r *bufio.Reader) ([]byte, bool, error) {
	var first string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			// A final line without a trailing newline is still a valid message.
			if err == io.EOF && strings.TrimSpace(line) != "" {
				return []byte(strings.TrimRight(line, "\r\n")), false, nil
			}
			return nil, false, err
		}
		first = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(first) != "" {
			break
		}
	}

	// Not a Content-Length header → newline-delimited JSON; the line is the message.
	if !strings.HasPrefix(strings.ToLower(first), "content-length:") {
		return []byte(first), false, nil
	}

	// LSP-style framing: parse this and any further headers up to the blank line.
	contentLength := -1
	parseHeader := func(line string) {
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			val := strings.TrimSpace(line[len("content-length:"):])
			contentLength, _ = strconv.Atoi(val)
		}
	}
	parseHeader(first)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, true, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parseHeader(line)
	}
	if contentLength < 0 {
		return nil, true, fmt.Errorf("missing Content-Length header")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, true, err
	}
	return buf, true, nil
}

func (s *Server) handle(req jsonRPCRequest) {
	ctx := context.Background()
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "dtrack-mcp-server", "version": version},
		})
	case "notifications/initialized":
		// notifications require no response
	case "tools/list":
		items := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			items = append(items, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		s.writeResult(req.ID, map[string]any{"tools": items})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "invalid params", nil)
			return
		}
		for _, t := range s.tools {
			if t.Name == params.Name {
				result, err := t.Handler(ctx, params.Arguments)
				if err != nil {
					s.writeResult(req.ID, map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": fmt.Sprintf("Error: %s", err.Error())},
						},
						"isError": true,
					})
					return
				}
				s.writeResult(req.ID, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": result},
					},
				})
				return
			}
		}
		s.writeError(req.ID, -32601, "tool not found: "+params.Name, nil)
	default:
		if req.ID != nil {
			s.writeError(req.ID, -32601, "method not found", nil)
		}
	}
}

func (s *Server) writeResult(id any, result any) {
	s.writeFrame(jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeError(id any, code int, message string, data any) {
	s.writeFrame(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]any{"code": code, "message": message, "data": data},
	})
}

func (s *Server) writeFrame(resp jsonRPCResponse) {
	body, err := json.Marshal(resp)
	if err != nil {
		return
	}
	if s.useContentLength {
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
		_, _ = io.WriteString(s.w, header)
		_, _ = s.w.Write(body)
		return
	}
	// newline-delimited JSON (MCP stdio standard)
	_, _ = s.w.Write(body)
	_, _ = io.WriteString(s.w, "\n")
}

// logf writes to stderr so stdout stays clean for JSON-RPC framing.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[dtrack-mcp] "+format+"\n", args...)
}
