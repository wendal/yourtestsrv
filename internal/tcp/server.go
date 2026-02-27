package tcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Server struct {
	Port       int
	TLS        bool
	Delay      time.Duration
	CloseAfter time.Duration
	Handler    Handler
}

type Handler interface {
	Handle(conn net.Conn)
}

type HandlerFunc func(conn net.Conn)

func (f HandlerFunc) Handle(conn net.Conn) {
	f(conn)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.Port)
	network := "tcp"

	ln, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("TCP server listening on %s", addr)

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

	log.Printf("TCP TLS server listening on %s", addr)

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

	remoteAddr := conn.RemoteAddr()
	log.Printf("TCP connection from %s", remoteAddr)

	if s.CloseAfter > 0 {
		time.Sleep(s.CloseAfter)
		conn.Close()
		log.Printf("TCP connection closed (close-after): %s", remoteAddr)
		return
	}

	if s.Handler != nil {
		s.Handler.Handle(conn)
		return
	}

	s.defaultHandle(conn)
}

func (s *Server) defaultHandle(conn net.Conn) {
	buf := make([]byte, 4096)

	for {
		if s.Delay > 0 {
			time.Sleep(s.Delay)
		}

		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Printf("TCP connection closed by client: %s", conn.RemoteAddr())
			} else {
				log.Printf("TCP read error: %v", err)
			}
			return
		}

		data := buf[:n]
		log.Printf("TCP received from %s: %x", conn.RemoteAddr(), data)

		_, err = conn.Write(data)
		if err != nil {
			log.Printf("TCP write error: %v", err)
			return
		}
	}
}

type Scenarios struct{}

func (s *Scenarios) Echo(conn net.Conn) {
	io.Copy(conn, conn)
}

func (s *Scenarios) DelayedEcho(delay time.Duration) HandlerFunc {
	return func(conn net.Conn) {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			time.Sleep(delay)
			conn.Write(buf[:n])
		}
	}
}

func (s *Scenarios) CloseAfter(duration time.Duration) HandlerFunc {
	return func(conn net.Conn) {
		time.Sleep(duration)
		conn.Close()
	}
}

func (s *Scenarios) HalfClose(conn net.Conn) {
	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	conn.Write(buf[:n])
	rwc, ok := conn.(interface{ CloseWrite() error })
	if ok {
		rwc.CloseWrite()
	}
}

func (s *Scenarios) ErrorResponse(conn net.Conn) {
	conn.Write([]byte("ERROR: simulated error response\n"))
	conn.Close()
}

func (s *Scenarios) SlowSend(delay time.Duration) HandlerFunc {
	return func(conn net.Conn) {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		data := buf[:n]
		for i := 0; i < len(data); i++ {
			conn.Write([]byte{data[i]})
			time.Sleep(delay)
		}
	}
}

func (s *Scenarios) DropConnection(conn net.Conn) {
	conn.Close()
}

func (s *Scenarios) KeepAlive(conn net.Conn) {
	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		conn.Write(buf[:n])
	}
}
