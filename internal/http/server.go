package http

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Request struct {
	Method  string
	Path    string
	Version string
	Headers map[string]string
	Body    []byte
}

type Response struct {
	Code    int
	Message string
	Headers map[string]string
	Body    []byte
}

type Server struct {
	Port         int
	TLS          bool
	SlowResponse bool
	SlowDuration time.Duration
	ErrorCode    int
	Chunked      bool
	Handler      Handler
}

type Handler interface {
	Handle(req *Request) *Response
}

type HandlerFunc func(req *Request) *Response

func (f HandlerFunc) Handle(req *Request) *Response {
	return f(req)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("HTTP server listening on %s", addr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.Port)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	ln, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("HTTP TLS server listening on %s", addr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		req, err := s.parseRequest(reader)
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Printf("HTTP parse error: %v", err)
			s.sendError(conn, 400, "Bad Request")
			return
		}

		log.Printf("HTTP request: %s %s %s", req.Method, req.Path, req.Version)

		var resp *Response
		if s.Handler != nil {
			resp = s.Handler.Handle(req)
		} else {
			resp = s.defaultHandle(req)
		}

		if s.SlowResponse && s.SlowDuration > 0 {
			time.Sleep(s.SlowDuration)
		}

		if s.ErrorCode > 0 && s.ErrorCode != 200 {
			resp.Code = s.ErrorCode
		}

		if err := s.sendResponse(conn, resp); err != nil {
			log.Printf("HTTP write error: %v", err)
			return
		}

		if req.Headers["Connection"] == "close" {
			return
		}
	}
}

func (s *Server) parseRequest(reader *bufio.Reader) (*Request, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = strings.TrimSuffix(line, "\r\n")
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid request line")
	}

	req := &Request{
		Method:  parts[0],
		Path:    parts[1],
		Version: parts[2],
		Headers: make(map[string]string),
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSuffix(line, "\r\n")
		if line == "" {
			break
		}

		headerParts := strings.SplitN(line, ":", 2)
		if len(headerParts) == 2 {
			key := strings.TrimSpace(headerParts[0])
			value := strings.TrimSpace(headerParts[1])
			req.Headers[key] = value
		}
	}

	if req.Headers["Content-Length"] != "" {
		length, _ := strconv.Atoi(req.Headers["Content-Length"])
		if length > 0 {
			req.Body = make([]byte, length)
			_, err := io.ReadFull(reader, req.Body)
			if err != nil {
				return nil, err
			}
		}
	}

	return req, nil
}

func (s *Server) sendResponse(conn net.Conn, resp *Response) error {
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}

	if s.Chunked && resp.Headers["Transfer-Encoding"] == "" {
		resp.Headers["Transfer-Encoding"] = "chunked"
		delete(resp.Headers, "Content-Length")
	} else if resp.Body != nil && resp.Headers["Content-Length"] == "" {
		resp.Headers["Content-Length"] = strconv.Itoa(len(resp.Body))
	}

	header := fmt.Sprintf("%s %d %s\r\n", "HTTP/1.1", resp.Code, resp.Message)

	for key, value := range resp.Headers {
		header += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	header += "\r\n"

	_, err := conn.Write([]byte(header))
	if err != nil {
		return err
	}

	if s.Chunked {
		if resp.Body != nil {
			chunk := fmt.Sprintf("%x\r\n%s\r\n", len(resp.Body), resp.Body)
			conn.Write([]byte(chunk))
		}
		conn.Write([]byte("0\r\n\r\n"))
	} else if resp.Body != nil {
		conn.Write(resp.Body)
	}

	return nil
}

func (s *Server) sendError(conn net.Conn, code int, message string) error {
	resp := &Response{
		Code:    code,
		Message: message,
		Body:    []byte(message),
	}
	return s.sendResponse(conn, resp)
}

func (s *Server) defaultHandle(req *Request) *Response {
	body := fmt.Sprintf("Method: %s\nPath: %s\nVersion: %s\n", req.Method, req.Path, req.Version)

	for k, v := range req.Headers {
		body += fmt.Sprintf("%s: %s\n", k, v)
	}

	return &Response{
		Code:    200,
		Message: "OK",
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		Body: []byte(body),
	}
}

type Scenarios struct{}

func (s *Scenarios) NotFound(req *Request) *Response {
	return &Response{
		Code:    404,
		Message: "Not Found",
		Body:    []byte("404 Not Found"),
	}
}

func (s *Scenarios) ServerError(req *Request) *Response {
	return &Response{
		Code:    500,
		Message: "Internal Server Error",
		Body:    []byte("500 Internal Server Error"),
	}
}

func (s *Scenarios) SlowResponse(duration time.Duration) *Server {
	return &Server{
		Port:         8080,
		SlowResponse: true,
		SlowDuration: duration,
	}
}

func (s *Scenarios) ChunkedResponse(req *Request) *Response {
	return &Response{
		Code:    200,
		Message: "OK",
		Headers: map[string]string{
			"Transfer-Encoding": "chunked",
		},
		Body: []byte("Hello World"),
	}
}

func (s *Scenarios) NoContent(req *Request) *Response {
	return &Response{
		Code:    204,
		Message: "No Content",
		Headers: map[string]string{},
		Body:    nil,
	}
}

func (s *Scenarios) Redirect(req *Request) *Response {
	return &Response{
		Code:    301,
		Message: "Moved Permanently",
		Headers: map[string]string{
			"Location": "/new-location",
		},
		Body: nil,
	}
}

func (s *Scenarios) CustomHeaders(req *Request) *Response {
	return &Response{
		Code:    200,
		Message: "OK",
		Headers: map[string]string{
			"X-Custom-Header":  "custom-value",
			"X-Another-Header": "another-value",
			"Content-Type":     "application/json",
			"X-Request-ID":     req.Headers["X-Request-ID"],
		},
		Body: []byte(`{"status":"ok"}`),
	}
}

func (s *Scenarios) IncompleteResponse(conn net.Conn) error {
	header := "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n"
	conn.Write([]byte(header))
	conn.Write([]byte("partial"))
	return nil
}

func (s *Scenarios) RangeRequest(req *Request) *Response {
	content := "0123456789"
	rangeHeader := req.Headers["Range"]

	if rangeHeader == "" {
		return &Response{
			Code:    416,
			Message: "Range Not Satisfiable",
			Body:    []byte("416 Range Not Satisfiable"),
		}
	}

	parts := strings.Split(rangeHeader, "=")
	if len(parts) != 2 {
		return &Response{
			Code:    400,
			Message: "Bad Request",
			Body:    []byte("400 Bad Request"),
		}
	}

	rangeParts := strings.Split(parts[1], "-")
	start := 0
	end := len(content) - 1

	if rangeParts[0] != "" {
		start, _ = strconv.Atoi(rangeParts[0])
	}
	if rangeParts[1] != "" {
		end, _ = strconv.Atoi(rangeParts[1])
	}

	return &Response{
		Code:    206,
		Message: "Partial Content",
		Headers: map[string]string{
			"Content-Range":  fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)),
			"Content-Length": strconv.Itoa(end - start + 1),
		},
		Body: []byte(content[start : end+1]),
	}
}
