package udp

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Server struct {
	Port     int
	DropRate float64
	Delay    time.Duration
	Bind     string
	Handler  Handler
}

type Handler interface {
	Handle(addr *net.UDPAddr, data []byte) []byte
}

type HandlerFunc func(addr *net.UDPAddr, data []byte) []byte

func (f HandlerFunc) Handle(addr *net.UDPAddr, data []byte) []byte {
	return f(addr, data)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	bind := s.Bind
	if bind == "" {
		bind = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", bind, s.Port)

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("UDP server listening on %s", addr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		conn.Close()
	}()

	buffer := make([]byte, 65535)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
				log.Printf("UDP read error: %v", err)
				continue
			}
		}

		data := make([]byte, n)
		copy(data, buffer[:n])

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handlePacket(conn, clientAddr, data)
		}()
	}
}

func (s *Server) handlePacket(conn *net.UDPConn, addr *net.UDPAddr, data []byte) {
	if s.DropRate > 0 && rand.Float64() < s.DropRate {
		log.Printf("UDP packet dropped from %s", addr)
		return
	}

	if s.Delay > 0 {
		time.Sleep(s.Delay)
	}

	log.Printf("UDP received from %s: %x", addr, data)

	var response []byte
	if s.Handler != nil {
		response = s.Handler.Handle(addr, data)
	} else {
		response = s.defaultHandle(addr, data)
	}

	if len(response) > 0 {
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		conn.WriteToUDP(response, addr)
	}
}

func (s *Server) defaultHandle(addr *net.UDPAddr, data []byte) []byte {
	return data
}

type Scenarios struct{}

func (s *Scenarios) Echo(addr *net.UDPAddr, data []byte) []byte {
	return data
}

func (s *Scenarios) DelayedEcho(delay time.Duration) HandlerFunc {
	return func(addr *net.UDPAddr, data []byte) []byte {
		time.Sleep(delay)
		return data
	}
}

func (s *Scenarios) PacketLoss(dropRate float64) HandlerFunc {
	return func(addr *net.UDPAddr, data []byte) []byte {
		if rand.Float64() < dropRate {
			return nil
		}
		return data
	}
}

func (s *Scenarios) OutOfOrder(count int, delay time.Duration) HandlerFunc {
	responses := make(map[int][][]byte)
	mu := sync.Mutex{}
	received := 0

	return func(addr *net.UDPAddr, data []byte) []byte {
		mu.Lock()
		received++
		responses[received] = append(responses[received], data)
		if count <= 0 {
			mu.Unlock()
			return data
		}
		idx := received - count
		if idx > 0 && len(responses[idx]) > 0 {
			resp := responses[idx][0]
			responses[idx] = responses[idx][1:]
			mu.Unlock()
			return resp
		}
		mu.Unlock()
		return nil
	}
}

func (s *Scenarios) Truncate(size int) HandlerFunc {
	return func(addr *net.UDPAddr, data []byte) []byte {
		if len(data) > size {
			return data[:size]
		}
		return data
	}
}

func (s *Scenarios) Broadcast() HandlerFunc {
	return func(addr *net.UDPAddr, data []byte) []byte {
		return data
	}
}

func (s *Scenarios) NoResponse(addr *net.UDPAddr, data []byte) []byte {
	return nil
}

func (s *Scenarios) ErrorResponse(addr *net.UDPAddr, data []byte) []byte {
	return []byte("ERROR\n")
}
