# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v1.1.0] - 2024-03-01

### Added
- MIT License
- This CHANGELOG file

## [v1.0.0] - 2024-01-01

### Added
- TCP echo server with configurable delay, close-after, and error-response modes
- UDP echo server with packet drop simulation and delay
- HTTP server with chunked transfer, slow response, error codes, and range support
- MQTT server supporting MQTT 3.1.1 and 5.0 with QoS, retain, and will message handling
- TLS support for TCP, HTTP, and MQTT (port + 10000 convention)
- JSON-based configuration with Go-style duration strings
- CLI entry point (`yourtestsrv.py`) with per-protocol subcommands
- Docker support via `Dockerfile`
- systemd service unit file
