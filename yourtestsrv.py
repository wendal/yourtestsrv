#!/usr/bin/env python3
"""yourtestsrv - Network test server for embedded devices."""

import argparse
import logging
import os
import signal
import sys
import threading

from yourtestsrv import config as cfg_module
from yourtestsrv.tcp_server import TCPServer
from yourtestsrv.udp_server import UDPServer
from yourtestsrv.http_server import HTTPServer
from yourtestsrv.mqtt_server import MQTTServer

logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s')
logger = logging.getLogger(__name__)

VERSION = 'v1.0.0'


def load_config(path):
    if not path or not os.path.exists(path):
        return cfg_module.default()
    return cfg_module.load(path)


def apply_defaults(cfg):
    if cfg.server.tcp.port == 0:
        cfg.server.tcp.port = 9000
        cfg.server.tcp.tls_port = cfg.server.tcp.port + 10000
    if cfg.server.udp.port == 0:
        cfg.server.udp.port = 9001
    if cfg.server.http.port == 0:
        cfg.server.http.port = 8080
        cfg.server.http.tls_port = cfg.server.http.port + 10000
    if cfg.server.mqtt.port == 0:
        cfg.server.mqtt.port = 1883
        cfg.server.mqtt.tls_port = cfg.server.mqtt.port + 10000


def make_stop_event():
    stop_event = threading.Event()

    def handler(sig, frame):
        stop_event.set()

    signal.signal(signal.SIGINT, handler)
    signal.signal(signal.SIGTERM, handler)
    return stop_event


def cmd_serve_all(args, mode):
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default='config.json')
    parser.add_argument('--bind', default='')
    opts = parser.parse_args(args)
    cfg = load_config(opts.config)
    apply_defaults(cfg)
    if opts.bind:
        cfg.server.bind = opts.bind

    stop_event = make_stop_event()
    threads = []

    cert_file, key_file = 'cert.pem', 'key.pem'

    def start(fn, *a):
        t = threading.Thread(target=fn, args=a, daemon=True)
        t.start()
        threads.append(t)

    if mode == 'both':
        start(TCPServer(cfg.server.tcp.port, cfg.server.bind,
                        cfg.server.tcp.delay, cfg.server.tcp.close_after).listen_and_serve, stop_event)
        start(HTTPServer(cfg.server.http.port, cfg.server.bind,
                         cfg.server.http.slow_response, cfg.server.http.slow_duration,
                         cfg.server.http.error_code, cfg.server.http.chunked).listen_and_serve, stop_event)
        start(MQTTServer(cfg.server.mqtt.port, cfg.server.bind,
                         cfg.server.mqtt.retain).listen_and_serve, stop_event)

    if mode in ('both', 'tls'):
        start(TCPServer(cfg.server.tcp.tls_port, cfg.server.bind,
                        cfg.server.tcp.delay, cfg.server.tcp.close_after).listen_and_serve_tls,
              stop_event, cert_file, key_file)
        start(HTTPServer(cfg.server.http.tls_port, cfg.server.bind,
                         cfg.server.http.slow_response, cfg.server.http.slow_duration,
                         cfg.server.http.error_code, cfg.server.http.chunked).listen_and_serve_tls,
              stop_event, cert_file, key_file)
        start(MQTTServer(cfg.server.mqtt.tls_port, cfg.server.bind,
                         cfg.server.mqtt.retain).listen_and_serve_tls,
              stop_event, cert_file, key_file)

    start(UDPServer(cfg.server.udp.port, cfg.server.bind,
                    cfg.server.udp.drop_rate, cfg.server.udp.delay).listen_and_serve, stop_event)

    logger.info('All servers started')
    logger.info(f'TCP: {cfg.server.tcp.port}, TCP TLS: {cfg.server.tcp.tls_port}')
    logger.info(f'UDP: {cfg.server.udp.port}')
    logger.info(f'HTTP: {cfg.server.http.port}, HTTP TLS: {cfg.server.http.tls_port}')
    logger.info(f'MQTT: {cfg.server.mqtt.port}, MQTT TLS: {cfg.server.mqtt.tls_port}')

    stop_event.wait()
    logger.info('All servers stopped')


def cmd_tcp(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default='config.json')
    parser.add_argument('--bind', default='')
    parser.add_argument('--port', '-p', type=int, default=0)
    parser.add_argument('--tls', action='store_true')
    parser.add_argument('--delay', default='0s')
    parser.add_argument('--close-after', default='0s')
    opts = parser.parse_args(args)
    c = load_config(opts.config)
    apply_defaults(c)
    bind = opts.bind or c.server.bind
    port = opts.port or (c.server.tcp.tls_port if opts.tls else c.server.tcp.port)
    from yourtestsrv.config import parse_duration
    delay = parse_duration(opts.delay) or c.server.tcp.delay
    close_after = parse_duration(opts.close_after) or c.server.tcp.close_after
    srv = TCPServer(port, bind, delay, close_after)
    stop_event = make_stop_event()
    if opts.tls:
        srv.listen_and_serve_tls(stop_event, 'cert.pem', 'key.pem')
    else:
        srv.listen_and_serve(stop_event)


def cmd_udp(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default='config.json')
    parser.add_argument('--bind', default='')
    parser.add_argument('--port', '-p', type=int, default=0)
    parser.add_argument('--drop-rate', type=float, default=0.0)
    parser.add_argument('--delay', default='0s')
    opts = parser.parse_args(args)
    c = load_config(opts.config)
    apply_defaults(c)
    bind = opts.bind or c.server.bind
    port = opts.port or c.server.udp.port
    from yourtestsrv.config import parse_duration
    drop_rate = opts.drop_rate or c.server.udp.drop_rate
    delay = parse_duration(opts.delay) or c.server.udp.delay
    srv = UDPServer(port, bind, drop_rate, delay)
    stop_event = make_stop_event()
    srv.listen_and_serve(stop_event)


def cmd_http(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default='config.json')
    parser.add_argument('--bind', default='')
    parser.add_argument('--port', '-p', type=int, default=0)
    parser.add_argument('--tls', action='store_true')
    parser.add_argument('--slow-response', action='store_true')
    parser.add_argument('--slow-duration', default='0s')
    parser.add_argument('--error-code', type=int, default=0)
    parser.add_argument('--chunked', action='store_true')
    opts = parser.parse_args(args)
    c = load_config(opts.config)
    apply_defaults(c)
    bind = opts.bind or c.server.bind
    port = opts.port or (c.server.http.tls_port if opts.tls else c.server.http.port)
    from yourtestsrv.config import parse_duration
    slow_response = opts.slow_response or c.server.http.slow_response
    slow_duration = parse_duration(opts.slow_duration) or c.server.http.slow_duration
    error_code = opts.error_code or c.server.http.error_code
    chunked = opts.chunked or c.server.http.chunked
    srv = HTTPServer(port, bind, slow_response, slow_duration, error_code, chunked)
    stop_event = make_stop_event()
    if opts.tls:
        srv.listen_and_serve_tls(stop_event, 'cert.pem', 'key.pem')
    else:
        srv.listen_and_serve(stop_event)


def cmd_mqtt(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default='config.json')
    parser.add_argument('--bind', default='')
    parser.add_argument('--port', '-p', type=int, default=0)
    parser.add_argument('--tls', action='store_true')
    parser.add_argument('--retain', '-r', action='store_true')
    opts = parser.parse_args(args)
    c = load_config(opts.config)
    apply_defaults(c)
    bind = opts.bind or c.server.bind
    port = opts.port or (c.server.mqtt.tls_port if opts.tls else c.server.mqtt.port)
    retain = opts.retain or c.server.mqtt.retain
    srv = MQTTServer(port, bind, retain)
    stop_event = make_stop_event()
    if opts.tls:
        srv.listen_and_serve_tls(stop_event, 'cert.pem', 'key.pem')
    else:
        srv.listen_and_serve(stop_event)


HELP = """\
yourtestsrv - Network test server for embedded devices

Usage:
  yourtestsrv.py <command> [options]

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
"""


def main():
    if len(sys.argv) < 2 or sys.argv[1] in ('-h', '--help'):
        print(HELP)
        sys.exit(0)

    command = sys.argv[1]
    args = sys.argv[2:]

    if command == 'serve-all':
        cmd_serve_all(args, 'both')
    elif command == 'serve-all-tls':
        cmd_serve_all(args, 'tls')
    elif command == 'tcp':
        cmd_tcp(args)
    elif command == 'udp':
        cmd_udp(args)
    elif command == 'http':
        cmd_http(args)
    elif command == 'mqtt':
        cmd_mqtt(args)
    elif command == 'version':
        print(f'yourtestsrv {VERSION}')
    else:
        print(HELP, file=sys.stderr)
        print(f'unknown command: {command}', file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
