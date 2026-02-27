package udp

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func startUDPServer(t *testing.T, srv *Server) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil && err != context.Canceled {
			t.Errorf("server error: %v", err)
		}
	}()
}

func getFreeUDPPort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func TestUDPEcho(t *testing.T) {
	port := getFreeUDPPort(t)
	srv := &Server{Port: port}
	startUDPServer(t, srv)

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello")
	buf := make([]byte, 32)
	deadline := time.Now().Add(800 * time.Millisecond)
	for {
		if _, err := conn.Write(msg); err != nil {
			t.Fatalf("write: %v", err)
		}
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := conn.Read(buf)
		if err == nil {
			if string(buf[:n]) != string(msg) {
				t.Fatalf("unexpected response: %q", string(buf[:n]))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("read: %v", err)
		}
	}
}

func TestUDPPacketLoss(t *testing.T) {
	port := getFreeUDPPort(t)
	srv := &Server{Port: port, DropRate: 1.0}
	startUDPServer(t, srv)
	time.Sleep(30 * time.Millisecond)

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte("hello"))
	buf := make([]byte, 32)
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatalf("expected packet drop")
	}
}

func TestUDPDelay(t *testing.T) {
	port := getFreeUDPPort(t)
	srv := &Server{Port: port, Delay: 150 * time.Millisecond}
	startUDPServer(t, srv)
	time.Sleep(30 * time.Millisecond)

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("x"))
	buf := make([]byte, 8)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if time.Since(start) < 120*time.Millisecond {
		t.Fatalf("expected delay")
	}
}
