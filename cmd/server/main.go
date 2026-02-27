package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"yourtestsrv/internal/config"
	httpServer "yourtestsrv/internal/http"
	mqttServer "yourtestsrv/internal/mqtt"
	tcpServer "yourtestsrv/internal/tcp"
	udpServer "yourtestsrv/internal/udp"
)

var (
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc
)

func main() {
	cfg = config.Default()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printHelp()
		return errors.New("missing command")
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	if cmd == "--help" || cmd == "-h" {
		printHelp()
		return nil
	}

	switch cmd {
	case "serve-all":
		return handleServeAll(args, "both")
	case "serve-all-tls":
		return handleServeAll(args, "tls")
	case "tcp":
		return handleTCP(args)
	case "udp":
		return handleUDP(args)
	case "http":
		return handleHTTP(args)
	case "mqtt":
		return handleMQTT(args)
	case "version":
		fmt.Println("yourtestsrv v1.0.0")
		return nil
	default:
		printHelp()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func handleServeAll(args []string, mode string) error {
	fs := flag.NewFlagSet("serve-all", flag.ContinueOnError)
	configFile := fs.String("config", "config.json", "Config file (JSON)")
	bind := fs.String("bind", "", "Bind address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadConfig(*configFile); err != nil {
		return err
	}
	applyConfigDefaults()
	if *bind != "" {
		cfg.Server.Bind = *bind
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	return startAllServers(mode)
}

func handleTCP(args []string) error {
	fs := flag.NewFlagSet("tcp", flag.ContinueOnError)
	configFile := fs.String("config", "config.json", "Config file (JSON)")
	bind := fs.String("bind", "", "Bind address")
	port := fs.Int("port", 0, "TCP port")
	portShort := fs.Int("p", 0, "TCP port")
	useTLS := fs.Bool("tls", false, "Enable TLS")
	delay := fs.Duration("delay", 0, "Response delay")
	closeAfter := fs.Duration("close-after", 0, "Close connection after duration")
	echo := fs.Bool("echo", true, "Enable echo")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadConfig(*configFile); err != nil {
		return err
	}
	applyConfigDefaults()
	if *bind != "" {
		cfg.Server.Bind = *bind
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	if *port == 0 && *portShort != 0 {
		*port = *portShort
	}
	if *port == 0 {
		if *useTLS {
			*port = cfg.Server.TCP.Port + 10000
		} else {
			*port = cfg.Server.TCP.Port
		}
	}
	if *delay == 0 {
		*delay = cfg.Server.TCP.Delay.Duration
	}
	if *closeAfter == 0 {
		*closeAfter = cfg.Server.TCP.CloseAfter.Duration
	}
	srv := &tcpServer.Server{
		Port:       *port,
		TLS:        *useTLS,
		Delay:      *delay,
		CloseAfter: *closeAfter,
		Bind:       cfg.Server.Bind,
	}
	if !*echo {
		srv.Handler = tcpServer.HandlerFunc(func(conn net.Conn) {
			buf := make([]byte, 4096)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					return
				}
			}
		})
	}
	if *useTLS {
		return srv.ListenAndServeTLS(ctx, "cert.pem", "key.pem")
	}
	return srv.ListenAndServe(ctx)
}

func handleUDP(args []string) error {
	fs := flag.NewFlagSet("udp", flag.ContinueOnError)
	configFile := fs.String("config", "config.json", "Config file (JSON)")
	bind := fs.String("bind", "", "Bind address")
	port := fs.Int("port", 0, "UDP port")
	portShort := fs.Int("p", 0, "UDP port")
	dropRate := fs.Float64("drop-rate", 0, "Packet drop rate (0-1)")
	delay := fs.Duration("delay", 0, "Response delay")
	echo := fs.Bool("echo", true, "Enable echo")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadConfig(*configFile); err != nil {
		return err
	}
	applyConfigDefaults()
	if *bind != "" {
		cfg.Server.Bind = *bind
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	if *port == 0 && *portShort != 0 {
		*port = *portShort
	}
	if *port == 0 {
		*port = cfg.Server.UDP.Port
	}
	if *dropRate == 0 {
		*dropRate = cfg.Server.UDP.DropRate
	}
	if *delay == 0 {
		*delay = cfg.Server.UDP.Delay.Duration
	}
	srv := &udpServer.Server{
		Port:     *port,
		DropRate: *dropRate,
		Delay:    *delay,
		Bind:     cfg.Server.Bind,
	}
	if !*echo {
		srv.Handler = udpServer.HandlerFunc(func(addr *net.UDPAddr, data []byte) []byte {
			return nil
		})
	}
	return srv.ListenAndServe(ctx)
}

func handleHTTP(args []string) error {
	fs := flag.NewFlagSet("http", flag.ContinueOnError)
	configFile := fs.String("config", "config.json", "Config file (JSON)")
	bind := fs.String("bind", "", "Bind address")
	port := fs.Int("port", 0, "HTTP port")
	portShort := fs.Int("p", 0, "HTTP port")
	useTLS := fs.Bool("tls", false, "Enable TLS")
	slowResponse := fs.Bool("slow-response", false, "Enable slow response")
	slowDuration := fs.Duration("slow-duration", 0, "Slow response duration")
	errorCode := fs.Int("error-code", 0, "HTTP response error code")
	chunked := fs.Bool("chunked", false, "Enable chunked response")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadConfig(*configFile); err != nil {
		return err
	}
	applyConfigDefaults()
	if *bind != "" {
		cfg.Server.Bind = *bind
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	if *port == 0 && *portShort != 0 {
		*port = *portShort
	}
	if *port == 0 {
		if *useTLS {
			*port = cfg.Server.HTTP.Port + 10000
		} else {
			*port = cfg.Server.HTTP.Port
		}
	}
	if !*slowResponse {
		*slowResponse = cfg.Server.HTTP.SlowResponse
	}
	if *slowDuration == 0 {
		*slowDuration = cfg.Server.HTTP.SlowDuration.Duration
	}
	if *errorCode == 0 {
		*errorCode = cfg.Server.HTTP.ErrorCode
	}
	if !*chunked {
		*chunked = cfg.Server.HTTP.Chunked
	}
	srv := &httpServer.Server{
		Port:         *port,
		TLS:          *useTLS,
		SlowResponse: *slowResponse,
		SlowDuration: *slowDuration,
		ErrorCode:    *errorCode,
		Chunked:      *chunked,
		Bind:         cfg.Server.Bind,
	}
	if *useTLS {
		return srv.ListenAndServeTLS(ctx, "cert.pem", "key.pem")
	}
	return srv.ListenAndServe(ctx)
}

func handleMQTT(args []string) error {
	fs := flag.NewFlagSet("mqtt", flag.ContinueOnError)
	configFile := fs.String("config", "config.json", "Config file (JSON)")
	bind := fs.String("bind", "", "Bind address")
	port := fs.Int("port", 0, "MQTT port")
	portShort := fs.Int("p", 0, "MQTT port")
	useTLS := fs.Bool("tls", false, "Enable TLS")
	retain := fs.Bool("retain", false, "Enable retain messages")
	retainShort := fs.Bool("r", false, "Enable retain messages")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadConfig(*configFile); err != nil {
		return err
	}
	applyConfigDefaults()
	if *bind != "" {
		cfg.Server.Bind = *bind
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	if *port == 0 && *portShort != 0 {
		*port = *portShort
	}
	if *port == 0 {
		if *useTLS {
			*port = cfg.Server.MQTT.Port + 10000
		} else {
			*port = cfg.Server.MQTT.Port
		}
	}
	if !*retain {
		*retain = *retainShort
	}
	if !*retain {
		*retain = cfg.Server.MQTT.Retain
	}
	srv := mqttServer.NewServer(*port)
	srv.RetainMessages = *retain
	srv.Bind = cfg.Server.Bind
	if *useTLS {
		return srv.ListenAndServeTLS(ctx, "cert.pem", "key.pem")
	}
	return srv.ListenAndServe(ctx)
}

func loadConfig(path string) error {
	if path == "" {
		return nil
	}
	loaded, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg = loaded
	return nil
}

func applyConfigDefaults() {
	if cfg.Server.TCP.Port == 0 {
		cfg.Server.TCP.Port = 9000
	}
	cfg.Server.TCP.TLSPort = cfg.Server.TCP.Port + 10000
	if cfg.Server.UDP.Port == 0 {
		cfg.Server.UDP.Port = 9001
	}
	if cfg.Server.HTTP.Port == 0 {
		cfg.Server.HTTP.Port = 8080
	}
	cfg.Server.HTTP.TLSPort = cfg.Server.HTTP.Port + 10000
	if cfg.Server.MQTT.Port == 0 {
		cfg.Server.MQTT.Port = 1883
	}
	cfg.Server.MQTT.TLSPort = cfg.Server.MQTT.Port + 10000
}

func startAllServers(mode string) error {
	certFile := "cert.pem"
	keyFile := "key.pem"
	if mode == "tls" || mode == "both" {
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			log.Println("Warning: cert.pem not found, TLS servers will fail")
		}
	}

	if mode == "both" {
		go func() {
			tcpSrv := &tcpServer.Server{
				Port:       cfg.Server.TCP.Port,
				Delay:      cfg.Server.TCP.Delay.Duration,
				CloseAfter: cfg.Server.TCP.CloseAfter.Duration,
				Bind:       cfg.Server.Bind,
			}
			if err := tcpSrv.ListenAndServe(ctx); err != nil && err != context.Canceled {
				log.Printf("TCP server error: %v", err)
			}
		}()
	}

	if mode == "tls" || mode == "both" {
		go func() {
			tcpTLSSrv := &tcpServer.Server{
				Port:       cfg.Server.TCP.TLSPort,
				TLS:        true,
				Delay:      cfg.Server.TCP.Delay.Duration,
				CloseAfter: cfg.Server.TCP.CloseAfter.Duration,
				Bind:       cfg.Server.Bind,
			}
			if err := tcpTLSSrv.ListenAndServeTLS(ctx, certFile, keyFile); err != nil && err != context.Canceled {
				log.Printf("TCP TLS server error: %v", err)
			}
		}()
	}

	go func() {
		udpSrv := &udpServer.Server{
			Port:     cfg.Server.UDP.Port,
			DropRate: cfg.Server.UDP.DropRate,
			Delay:    cfg.Server.UDP.Delay.Duration,
			Bind:     cfg.Server.Bind,
		}
		if err := udpSrv.ListenAndServe(ctx); err != nil && err != context.Canceled {
			log.Printf("UDP server error: %v", err)
		}
	}()

	if mode == "both" {
		go func() {
			httpSrv := &httpServer.Server{
				Port:         cfg.Server.HTTP.Port,
				SlowResponse: cfg.Server.HTTP.SlowResponse,
				SlowDuration: cfg.Server.HTTP.SlowDuration.Duration,
				ErrorCode:    cfg.Server.HTTP.ErrorCode,
				Chunked:      cfg.Server.HTTP.Chunked,
				Bind:         cfg.Server.Bind,
			}
			if err := httpSrv.ListenAndServe(ctx); err != nil && err != context.Canceled {
				log.Printf("HTTP server error: %v", err)
			}
		}()
	}

	if mode == "tls" || mode == "both" {
		go func() {
			httpTLSSrv := &httpServer.Server{
				Port:         cfg.Server.HTTP.TLSPort,
				SlowResponse: cfg.Server.HTTP.SlowResponse,
				SlowDuration: cfg.Server.HTTP.SlowDuration.Duration,
				ErrorCode:    cfg.Server.HTTP.ErrorCode,
				Chunked:      cfg.Server.HTTP.Chunked,
				Bind:         cfg.Server.Bind,
			}
			if err := httpTLSSrv.ListenAndServeTLS(ctx, certFile, keyFile); err != nil && err != context.Canceled {
				log.Printf("HTTP TLS server error: %v", err)
			}
		}()
	}

	if mode == "both" {
		go func() {
			mqttSrv := mqttServer.NewServer(cfg.Server.MQTT.Port)
			mqttSrv.RetainMessages = cfg.Server.MQTT.Retain
			mqttSrv.Bind = cfg.Server.Bind
			if err := mqttSrv.ListenAndServe(ctx); err != nil && err != context.Canceled {
				log.Printf("MQTT server error: %v", err)
			}
		}()
	}

	if mode == "tls" || mode == "both" {
		go func() {
			mqttTLSSrv := mqttServer.NewServer(cfg.Server.MQTT.TLSPort)
			mqttTLSSrv.RetainMessages = cfg.Server.MQTT.Retain
			mqttTLSSrv.Bind = cfg.Server.Bind
			if err := mqttTLSSrv.ListenAndServeTLS(ctx, certFile, keyFile); err != nil && err != context.Canceled {
				log.Printf("MQTT TLS server error: %v", err)
			}
		}()
	}

	log.Println("All servers started")
	log.Printf("TCP: %d, TCP TLS: %d", cfg.Server.TCP.Port, cfg.Server.TCP.TLSPort)
	log.Printf("UDP: %d", cfg.Server.UDP.Port)
	log.Printf("HTTP: %d, HTTP TLS: %d", cfg.Server.HTTP.Port, cfg.Server.HTTP.TLSPort)
	log.Printf("MQTT: %d, MQTT TLS: %d", cfg.Server.MQTT.Port, cfg.Server.MQTT.TLSPort)

	<-ctx.Done()
	log.Println("All servers stopped")
	return nil
}

func printHelp() {
	msg := `yourtestsrv - Network test server for embedded devices

Usage:
  yourtestsrv <command> [options]

Commands:
  serve-all        Start all servers (non-encrypted)
  serve-all-tls    Start all servers (TLS encrypted)
  tcp              Start TCP server
  udp              Start UDP server
  http             Start HTTP server
  mqtt             Start MQTT server
  version          Print version

Global options:
  --config <path>  Config file (JSON)
  --bind <addr>    Bind address (default: 0.0.0.0)
`
	fmt.Println(msg)
}
