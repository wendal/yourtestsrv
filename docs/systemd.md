# systemd Service

This guide installs yourtestsrv as a systemd service using the `serve-all` command.

## Prereqs
- A built `yourtestsrv` binary on the target host.
- Config file at `/etc/yourtestsrv/config.json`.

## Install

1. Create the service user and config directory:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin yourtestsrv
sudo install -d -o yourtestsrv -g yourtestsrv /etc/yourtestsrv
```

2. Install the binary:

```bash
sudo install -m 0755 yourtestsrv /usr/local/bin/yourtestsrv
```

3. Install the config:

```bash
sudo install -m 0644 config.json /etc/yourtestsrv/config.json
```

4. Install the unit file:

```bash
sudo install -m 0644 systemd/yourtestsrv.service /etc/systemd/system/yourtestsrv.service
```

5. Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now yourtestsrv
```

## Check status and logs

```bash
systemctl status yourtestsrv
journalctl -u yourtestsrv -f
```

## TLS mode

To run TLS servers, change `ExecStart` to `serve-all-tls` and ensure
`/etc/yourtestsrv/cert.pem` and `/etc/yourtestsrv/key.pem` are present.

## Ports

Default ports (non-TLS): TCP 9000, UDP 9001, HTTP 8080, MQTT 1883.
TLS ports are +10000 from the base port.
