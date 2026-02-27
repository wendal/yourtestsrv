package tcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func startServer(t *testing.T, srv *Server, useTLS bool, certFile, keyFile string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		var err error
		if useTLS {
			err = srv.ListenAndServeTLS(ctx, certFile, keyFile)
		} else {
			err = srv.ListenAndServe(ctx)
		}
		if err != nil && err != context.Canceled {
			t.Errorf("server error: %v", err)
		}
	}()
}

func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func makeTempCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
		DNSNames:  []string{"localhost"},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
		},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()

	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	keyOut.Close()

	return certFile, keyFile
}

func TestTCPEcho(t *testing.T) {
	port := getFreePort(t)
	srv := &Server{Port: port}
	startServer(t, srv, false, "", "")
	waitTCP(t, port)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("unexpected response: %q", string(buf))
	}
}

func TestTCPDelay(t *testing.T) {
	port := getFreePort(t)
	srv := &Server{Port: port, Delay: 200 * time.Millisecond}
	startServer(t, srv, false, "", "")
	waitTCP(t, port)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("x"))
	reader := bufio.NewReader(conn)
	_, err = reader.ReadByte()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatalf("expected delay")
	}
}

func TestTCPCloseAfter(t *testing.T) {
	port := getFreePort(t)
	srv := &Server{Port: port, CloseAfter: 100 * time.Millisecond}
	startServer(t, srv, false, "", "")
	waitTCP(t, port)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(200 * time.Millisecond)
	_, err = conn.Write([]byte("x"))
	if err == nil {
		buf := make([]byte, 1)
		_, err = conn.Read(buf)
	}
	if err == nil {
		t.Fatalf("expected connection to close")
	}
}

func TestTCPTLS(t *testing.T) {
	port := getFreePort(t)
	certFile, keyFile := makeTempCert(t)
	srv := &Server{Port: port}
	startServer(t, srv, true, certFile, keyFile)
	waitTCP(t, port)

	config := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), config)
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("unexpected response: %q", string(buf))
	}
}

func waitTCP(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
