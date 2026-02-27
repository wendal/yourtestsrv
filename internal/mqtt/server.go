package mqtt

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	MQTT_CONNECT     byte = 1
	MQTT_CONNACK     byte = 2
	MQTT_PUBLISH     byte = 3
	MQTT_PUBACK      byte = 4
	MQTT_PUBREC      byte = 5
	MQTT_PUBREL      byte = 6
	MQTT_PUBCOMP     byte = 7
	MQTT_SUBSCRIBE   byte = 8
	MQTT_SUBACK      byte = 9
	MQTT_UNSUBSCRIBE byte = 10
	MQTT_UNSUBACK    byte = 11
	MQTT_PINGREQ     byte = 12
	MQTT_PINGRESP    byte = 13
	MQTT_DISCONNECT  byte = 14
)

type Packet struct {
	Type    byte
	Flags   byte
	Payload []byte
}

type Connect struct {
	ProtocolName  string
	ProtocolLevel byte
	CleanSession  bool
	WillFlag      bool
	WillQos       byte
	WillRetain    bool
	KeepAlive     uint16
	ClientID      string
	WillTopic     string
	WillMessage   string
	Username      string
	Password      string
}

type Publish struct {
	Topic    string
	Qos      byte
	Payload  []byte
	PacketID uint16
}

type Subscribe struct {
	PacketID uint16
	Topics   []Topic
}

type Topic struct {
	Topic string
	Qos   byte
}

type ConnAck struct {
	ReturnCode byte
}

type Server struct {
	Port           int
	RetainMessages bool
	Handler        Handler
	clients        sync.Map
	retained       map[string][]byte
}

type Handler interface {
	HandleConnect(conn net.Conn, connect *Connect)
	HandlePublish(publish *Publish)
	HandleSubscribe(subscribe *Subscribe)
}

type HandlerFunc struct {
	OnConnect   func(conn net.Conn, connect *Connect)
	OnPublish   func(publish *Publish)
	OnSubscribe func(subscribe *Subscribe)
}

func (h HandlerFunc) HandleConnect(conn net.Conn, connect *Connect) {
	if h.OnConnect != nil {
		h.OnConnect(conn, connect)
	}
}

func (h HandlerFunc) HandlePublish(publish *Publish) {
	if h.OnPublish != nil {
		h.OnPublish(publish)
	}
}

func (h HandlerFunc) HandleSubscribe(subscribe *Subscribe) {
	if h.OnSubscribe != nil {
		h.OnSubscribe(subscribe)
	}
}

func NewServer(port int) *Server {
	return &Server{
		Port:           port,
		RetainMessages: false,
		retained:       make(map[string][]byte),
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("MQTT server listening on %s", addr)

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

	log.Printf("MQTT TLS server listening on %s", addr)

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
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		packet, err := s.readPacket(reader)
		if err != nil {
			if err == io.EOF {
				log.Printf("MQTT client disconnected: %s", conn.RemoteAddr())
			} else {
				log.Printf("MQTT read error: %v", err)
			}
			return
		}

		s.handlePacket(conn, packet)
	}
}

func (s *Server) readPacket(reader *bufio.Reader) (*Packet, error) {
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	packetType := (firstByte >> 4) & 0x0F
	flags := firstByte & 0x0F

	length := 0
	multiplier := 1
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		length += int(b&127) * multiplier
		multiplier *= 128
		if b&128 == 0 {
			break
		}
	}

	payload := make([]byte, length)
	if length > 0 {
		_, err := io.ReadFull(reader, payload)
		if err != nil {
			return nil, err
		}
	}

	return &Packet{
		Type:    packetType,
		Flags:   flags,
		Payload: payload,
	}, nil
}

func (s *Server) handlePacket(conn net.Conn, packet *Packet) {
	switch packet.Type {
	case MQTT_CONNECT:
		s.handleConnect(conn, packet)
	case MQTT_PUBLISH:
		s.handlePublish(conn, packet)
	case MQTT_PUBACK:
		s.handlePubAck(packet)
	case MQTT_PUBREC:
		s.handlePubRec(conn, packet)
	case MQTT_PUBREL:
		s.handlePubRel(conn, packet)
	case MQTT_SUBSCRIBE:
		s.handleSubscribe(conn, packet)
	case MQTT_UNSUBSCRIBE:
		s.handleUnsubscribe(conn, packet)
	case MQTT_PINGREQ:
		s.handlePingReq(conn)
	case MQTT_DISCONNECT:
		log.Printf("MQTT client sent disconnect: %s", conn.RemoteAddr())
		conn.Close()
	}
}

func (s *Server) handleConnect(conn net.Conn, packet *Packet) {
	connect, err := s.parseConnect(packet.Payload)
	if err != nil {
		log.Printf("MQTT connect parse error: %v", err)
		return
	}

	log.Printf("MQTT CONNECT: client=%s, clean=%v", connect.ClientID, connect.CleanSession)

	s.clients.Store(connect.ClientID, conn)

	connAck := s.buildConnAck(0)
	s.writePacket(conn, MQTT_CONNACK, 0, connAck)

	if s.Handler != nil {
		s.Handler.HandleConnect(conn, connect)
	}
}

func (s *Server) parseConnect(payload []byte) (*Connect, error) {
	connect := &Connect{}

	pos := 0

	if len(payload) < 2 {
		return nil, fmt.Errorf("connect payload too short")
	}
	protocolName, n := readMqttString(payload[pos:])
	if n == 0 {
		return nil, fmt.Errorf("invalid protocol name")
	}
	connect.ProtocolName = protocolName
	pos += n

	if len(payload) < pos+1 {
		return nil, fmt.Errorf("missing protocol level")
	}
	connect.ProtocolLevel = payload[pos]
	pos++

	if len(payload) < pos+1 {
		return nil, fmt.Errorf("missing connect flags")
	}
	flags := payload[pos]
	pos++

	connect.CleanSession = (flags & 0x02) != 0
	connect.WillFlag = (flags & 0x04) != 0
	connect.WillQos = (flags >> 3) & 0x03
	connect.WillRetain = (flags & 0x20) != 0

	if len(payload) < pos+2 {
		return nil, fmt.Errorf("missing keepalive")
	}
	connect.KeepAlive = binary.BigEndian.Uint16(payload[pos : pos+2])
	pos += 2

	clientID, n := readMqttString(payload[pos:])
	if n == 0 {
		return nil, fmt.Errorf("invalid client id")
	}
	connect.ClientID = clientID
	pos += n

	if connect.WillFlag {
		willTopic, n := readMqttString(payload[pos:])
		if n == 0 {
			return nil, fmt.Errorf("invalid will topic")
		}
		connect.WillTopic = willTopic
		pos += n

		willMessage, n := readMqttString(payload[pos:])
		if n == 0 {
			return nil, fmt.Errorf("invalid will message")
		}
		connect.WillMessage = willMessage
		pos += n
	}

	if pos < len(payload) {
		username, n := readMqttString(payload[pos:])
		if n == 0 {
			return nil, fmt.Errorf("invalid username")
		}
		connect.Username = username
		pos += n

		if pos < len(payload) {
			password, _ := readMqttString(payload[pos:])
			connect.Password = password
		}
	}

	return connect, nil
}

func readMqttString(data []byte) (string, int) {
	if len(data) < 2 {
		return "", 0
	}
	length := int(data[0])<<8 | int(data[1])
	if len(data) < 2+length {
		return "", 0
	}
	return string(data[2 : 2+length]), 2 + length
}

func (s *Server) buildConnAck(returnCode byte) []byte {
	payload := make([]byte, 4)
	payload[0] = 0
	payload[1] = returnCode
	return payload
}

func (s *Server) handlePublish(conn net.Conn, packet *Packet) {
	publish, err := s.parsePublish(packet)
	if err != nil {
		log.Printf("MQTT publish parse error: %v", err)
		return
	}

	log.Printf("MQTT PUBLISH: topic=%s, qos=%d, payload=%x", publish.Topic, publish.Qos, publish.Payload)

	if s.RetainMessages && len(publish.Payload) > 0 {
		s.retained[publish.Topic] = publish.Payload
	}

	if s.Handler != nil {
		s.Handler.HandlePublish(publish)
	}

	if publish.Qos == 1 {
		pubAck := make([]byte, 2)
		binary.BigEndian.PutUint16(pubAck, publish.PacketID)
		s.writePacket(conn, MQTT_PUBACK, 0, pubAck)
	} else if publish.Qos == 2 {
		pubRec := make([]byte, 2)
		binary.BigEndian.PutUint16(pubRec, publish.PacketID)
		s.writePacket(conn, MQTT_PUBREC, 0, pubRec)
	}
}

func (s *Server) parsePublish(packet *Packet) (*Publish, error) {
	publish := &Publish{}

	pos := 0
	topic, n := readMqttString(packet.Payload)
	publish.Topic = topic
	pos += n

	qos := (packet.Flags >> 1) & 0x03
	publish.Qos = qos

	if qos > 0 {
		if len(packet.Payload) < pos+2 {
			return publish, nil
		}
		publish.PacketID = binary.BigEndian.Uint16(packet.Payload[pos : pos+2])
		pos += 2
	}

	publish.Payload = packet.Payload[pos:]
	return publish, nil
}

func (s *Server) handlePubAck(packet *Packet) {
	if len(packet.Payload) >= 2 {
		packetID := binary.BigEndian.Uint16(packet.Payload)
		log.Printf("MQTT PUBACK: packetID=%d", packetID)
	}
}

func (s *Server) handlePubRec(conn net.Conn, packet *Packet) {
	if len(packet.Payload) >= 2 {
		packetID := binary.BigEndian.Uint16(packet.Payload)
		log.Printf("MQTT PUBREC: packetID=%d", packetID)

		pubRel := make([]byte, 2)
		binary.BigEndian.PutUint16(pubRel, packetID)
		s.writePacket(conn, MQTT_PUBREL, 2, pubRel)
	}
}

func (s *Server) handlePubRel(conn net.Conn, packet *Packet) {
	if len(packet.Payload) >= 2 {
		packetID := binary.BigEndian.Uint16(packet.Payload)
		log.Printf("MQTT PUBREL: packetID=%d", packetID)

		pubComp := make([]byte, 2)
		binary.BigEndian.PutUint16(pubComp, packetID)
		s.writePacket(conn, MQTT_PUBCOMP, 0, pubComp)
	}
}

func (s *Server) handleSubscribe(conn net.Conn, packet *Packet) {
	if len(packet.Payload) < 2 {
		return
	}

	packetID := binary.BigEndian.Uint16(packet.Payload[0:2])
	pos := 2

	var returnCodes []byte

	for pos < len(packet.Payload) {
		topic, n := readMqttString(packet.Payload[pos:])
		if n == 0 {
			break
		}
		pos += n

		if pos < len(packet.Payload) {
			qos := packet.Payload[pos]
			pos++
			returnCodes = append(returnCodes, qos)
			log.Printf("MQTT SUBSCRIBE: packetID=%d, topic=%s, qos=%d", packetID, topic, qos)
		}
	}

	if s.Handler != nil {
		subscribe := &Subscribe{
			PacketID: packetID,
		}
		s.Handler.HandleSubscribe(subscribe)
	}

	response := make([]byte, 2+len(returnCodes))
	response[0] = byte(packetID >> 8)
	response[1] = byte(packetID & 0xFF)
	copy(response[2:], returnCodes)

	s.writePacket(conn, MQTT_SUBACK, 0, response)
}

func (s *Server) handleUnsubscribe(conn net.Conn, packet *Packet) {
	if len(packet.Payload) < 2 {
		return
	}

	packetID := binary.BigEndian.Uint16(packet.Payload[0:2])
	pos := 2

	for pos < len(packet.Payload) {
		topic, n := readMqttString(packet.Payload[pos:])
		if n == 0 {
			break
		}
		pos += n
		log.Printf("MQTT UNSUBSCRIBE: packetID=%d, topic=%s", packetID, topic)
	}

	response := make([]byte, 2)
	binary.BigEndian.PutUint16(response, packetID)

	s.writePacket(conn, MQTT_UNSUBACK, 0, response)
}

func (s *Server) handlePingReq(conn net.Conn) {
	s.writePacket(conn, MQTT_PINGRESP, 0, nil)
}

func (s *Server) writePacket(conn net.Conn, packetType byte, flags byte, payload []byte) error {
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
	data = append(data, payload...)

	_, err := conn.Write(data)
	return err
}

type Scenarios struct{}

func (s *Scenarios) InvalidConnect(conn net.Conn) {
	connAck := s.buildConnAck(1)
	payload := []byte{0, 0}
	payload = append(payload, connAck...)
	header := (MQTT_CONNACK << 4)
	conn.Write([]byte{header, 2})
	conn.Write(payload)
}

func (s *Scenarios) buildConnAck(returnCode byte) []byte {
	payload := make([]byte, 2)
	payload[0] = 0
	payload[1] = returnCode
	return payload
}

func (s *Scenarios) RetainMessage(server *Server, topic string, payload []byte) {
	server.retained[topic] = payload
}

func (s *Scenarios) WillMessage(connect *Connect) {
	log.Printf("Will message: topic=%s, message=%s", connect.WillTopic, connect.WillMessage)
}

func (s *Scenarios) QoS0Publish(conn net.Conn, topic string, payload []byte) {
	var packet []byte

	topicLen := make([]byte, 2)
	binary.BigEndian.PutUint16(topicLen, uint16(len(topic)))
	packet = append(packet, topicLen...)
	packet = append(packet, topic...)
	packet = append(packet, payload...)

	header := (MQTT_PUBLISH << 4)
	length := len(packet)

	var lengthBytes []byte
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
	data = append(data, packet...)

	conn.Write(data)
}

func (s *Scenarios) QoS1Publish(conn net.Conn, topic string, packetID uint16, payload []byte) {
	var packet []byte

	topicLen := make([]byte, 2)
	binary.BigEndian.PutUint16(topicLen, uint16(len(topic)))
	packet = append(packet, topicLen...)
	packet = append(packet, topic...)

	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, packetID)
	packet = append(packet, pid...)
	packet = append(packet, payload...)

	header := (MQTT_PUBLISH << 4) | 0x02
	length := len(packet)

	var lengthBytes []byte
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
	data = append(data, packet...)

	conn.Write(data)
}

func (s *Scenarios) MalformedPacket(conn net.Conn) {
	conn.Write([]byte{0xFF, 0xFF})
}
