package mqtt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
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

func startMQTTServer(t *testing.T, srv *Server, useTLS bool, certFile, keyFile string) {
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

func buildConnectPacket(clientID string) []byte {
	var payload []byte
	protocol := "MQTT"
	payload = appendString(payload, protocol)
	payload = append(payload, 4)
	payload = append(payload, 2)
	payload = append(payload, 0, 60)
	payload = appendString(payload, clientID)

	return buildPacket(MQTT_CONNECT, 0, payload)
}

func buildPublishPacket(topic string, payload []byte) []byte {
	var body []byte
	body = appendString(body, topic)
	body = append(body, payload...)
	return buildPacket(MQTT_PUBLISH, 0, body)
}

func appendString(data []byte, s string) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(len(s)))
	data = append(data, buf...)
	data = append(data, []byte(s)...)
	return data
}

func buildPacket(packetType byte, flags byte, payload []byte) []byte {
	header := (packetType << 4) | flags
	var lengthBytes []byte
	length := len(payload)
	for {
		b := byte(length % 128)
		length /= 128
		if length > 0 {
			b |= 0x80
		}
		lengthBytes = append(lengthBytes, b)
		if length == 0 {
			break
		}
	}
	data := append([]byte{header}, lengthBytes...)
	return append(data, payload...)
}

func TestMQTTConnect(t *testing.T) {
	port := getFreePort(t)
	srv := NewServer(port)
	startMQTTServer(t, srv, false, "", "")
	waitMQTT(t, port)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.Write(buildConnectPacket("client1"))
	buf := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if buf[0]>>4 != MQTT_CONNACK {
		t.Fatalf("expected connack")
	}
}

func TestMQTTPublish(t *testing.T) {
	port := getFreePort(t)
	received := make(chan *Publish, 1)
	srv := NewServer(port)
	srv.Handler = HandlerFunc{
		OnPublish: func(p *Publish) {
			received <- p
		},
	}
	startMQTTServer(t, srv, false, "", "")
	waitMQTT(t, port)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.Write(buildConnectPacket("client1"))
	buf := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	conn.Read(buf)
	conn.Write(buildPublishPacket("test/topic", []byte("hello")))

	select {
	case msg := <-received:
		if msg.Topic != "test/topic" {
			t.Fatalf("unexpected topic: %s", msg.Topic)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected publish")
	}
}

func TestMQTTTLS(t *testing.T) {
	port := getFreePort(t)
	certFile, keyFile := makeTempCert(t)
	srv := NewServer(port)
	startMQTTServer(t, srv, true, certFile, keyFile)
	waitMQTT(t, port)

	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("dial tls: %v", err)
	}
	defer conn.Close()

	conn.Write(buildConnectPacket("client1"))
	buf := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if buf[0]>>4 != MQTT_CONNACK {
		t.Fatalf("expected connack")
	}
}

func waitMQTT(t *testing.T, port int) {
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
