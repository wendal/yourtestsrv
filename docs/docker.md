# Docker

This guide builds and runs yourtestsrv in Docker using a minimal distroless image.

## Build

```bash
docker build -t yourtestsrv:latest .
```

## Run (default config)

```bash
docker run --rm -p 9000:9000 -p 9001:9001/udp -p 8080:8080 -p 1883:1883 \
  yourtestsrv:latest
```

## Run with custom config

```bash
docker run --rm \
  -v $(pwd)/config.json:/etc/yourtestsrv/config.json:ro \
  -p 9000:9000 -p 9001:9001/udp -p 8080:8080 -p 1883:1883 \
  yourtestsrv:latest
```

## Health check

The HTTP server exposes a plain-text health endpoint:

```bash
curl http://localhost:8080/healthz
```

## TLS mode

To use TLS, mount `cert.pem` and `key.pem` and override the command:

```bash
docker run --rm \
  -v $(pwd)/config.json:/etc/yourtestsrv/config.json:ro \
  -v $(pwd)/cert.pem:/etc/yourtestsrv/cert.pem:ro \
  -v $(pwd)/key.pem:/etc/yourtestsrv/key.pem:ro \
  -p 9000:9000 -p 9001:9001/udp -p 8080:8080 -p 1883:1883 \
  -p 19000:19000 -p 19001:19001/udp -p 18080:18080 -p 11883:11883 \
  yourtestsrv:latest serve-all-tls --config /etc/yourtestsrv/config.json
```

## Notes

- The image runs as a non-root user.
- UDP requires `/udp` in the port mapping.
- TLS ports are base port +10000.
- To bind on localhost only, add `--bind 127.0.0.1` to the command.
