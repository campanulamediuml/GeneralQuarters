package gqcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

type stdioServer struct {
	client     *Client
	stdout     io.Writer
	writeMu    sync.Mutex
	requestsMu sync.Mutex
	requests   map[string]context.CancelFunc
	wg         sync.WaitGroup
}

func Serve(ctx context.Context, client *Client, stdin io.Reader, stdout io.Writer) error {
	server := &stdioServer{client: client, stdout: stdout, requests: map[string]context.CancelFunc{}}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var request Request
		if err := json.Unmarshal(line, &request); err != nil {
			server.write(Response{OK: false, Error: &ToolError{Message: "请求不是有效 JSON", Kind: "protocol"}})
			continue
		}
		if request.ID == "" || len(request.ID) > 128 || request.Method == "" {
			server.write(Response{ID: request.ID, OK: false, Error: &ToolError{Message: "id 和 method 必填，id 最长 128 字节", Kind: "protocol"}})
			continue
		}
		if request.Method == "gq_cancel" {
			server.cancel(request)
			continue
		}
		requestCtx, cancel := context.WithCancel(ctx)
		if !server.add(request.ID, cancel) {
			cancel()
			server.write(Response{ID: request.ID, OK: false, Error: &ToolError{Message: "请求 ID 正在使用中", Kind: "protocol"}})
			continue
		}
		server.wg.Add(1)
		go func() {
			defer server.wg.Done()
			defer server.remove(request.ID)
			defer cancel()
			result, err := client.call(requestCtx, request)
			response := Response{ID: request.ID, OK: err == nil, Result: result}
			if err != nil {
				response.Result = nil
				response.Error = normalizeError(err)
			}
			server.write(response)
		}()
	}
	server.requestsMu.Lock()
	for _, cancel := range server.requests {
		cancel()
	}
	server.requestsMu.Unlock()
	server.wg.Wait()
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *stdioServer) add(id string, cancel context.CancelFunc) bool {
	s.requestsMu.Lock()
	defer s.requestsMu.Unlock()
	if _, exists := s.requests[id]; exists {
		return false
	}
	s.requests[id] = cancel
	return true
}

func (s *stdioServer) remove(id string) {
	s.requestsMu.Lock()
	delete(s.requests, id)
	s.requestsMu.Unlock()
}

func (s *stdioServer) cancel(request Request) {
	var params struct {
		TargetID string `json:"target_id"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		s.write(Response{ID: request.ID, OK: false, Error: &ToolError{Message: err.Error(), Kind: "protocol"}})
		return
	}
	s.requestsMu.Lock()
	cancel := s.requests[params.TargetID]
	s.requestsMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.write(Response{ID: request.ID, OK: true, Result: map[string]any{"canceled": cancel != nil}})
}

func (s *stdioServer) write(response Response) {
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.stdout.Write(append(data, '\n'))
}

func normalizeError(err error) *ToolError {
	var toolError *ToolError
	if errors.As(err, &toolError) {
		return toolError
	}
	if isCanceled(err) {
		return &ToolError{Message: err.Error(), Kind: "canceled"}
	}
	return &ToolError{Message: err.Error(), Kind: "internal"}
}
