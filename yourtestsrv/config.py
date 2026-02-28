import json
import re


def parse_duration(s):
    """Parse Go duration string like '5s', '200ms', '1m30s' to seconds (float)."""
    if not s or s == '0' or s == '0s':
        return 0.0
    total = 0.0
    pattern = re.compile(r'(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)')
    for m in pattern.finditer(s):
        val, unit = float(m.group(1)), m.group(2)
        if unit == 'ns':
            total += val * 1e-9
        elif unit in ('us', 'µs'):
            total += val * 1e-6
        elif unit == 'ms':
            total += val * 1e-3
        elif unit == 's':
            total += val
        elif unit == 'm':
            total += val * 60
        elif unit == 'h':
            total += val * 3600
    return total


class TCPConfig:
    def __init__(self, port=9000, delay='0s', close_after='0s'):
        self.port = port
        self.tls_port = port + 10000
        self.delay = parse_duration(delay)
        self.close_after = parse_duration(close_after)


class UDPConfig:
    def __init__(self, port=9001, drop_rate=0.0, delay='0s'):
        self.port = port
        self.drop_rate = drop_rate
        self.delay = parse_duration(delay)


class HTTPConfig:
    def __init__(self, port=8080, slow_response=False, slow_duration='0s', error_code=200, chunked=False):
        self.port = port
        self.tls_port = port + 10000
        self.slow_response = slow_response
        self.slow_duration = parse_duration(slow_duration)
        self.error_code = error_code
        self.chunked = chunked


class MQTTConfig:
    def __init__(self, port=1883, retain=False):
        self.port = port
        self.tls_port = port + 10000
        self.retain = retain


class ServerConfig:
    def __init__(self, bind='0.0.0.0', tcp=None, udp=None, http=None, mqtt=None):
        self.bind = bind or '0.0.0.0'
        self.tcp = TCPConfig(**(tcp or {}))
        self.udp = UDPConfig(**(udp or {}))
        self.http = HTTPConfig(**(http or {}))
        self.mqtt = MQTTConfig(**(mqtt or {}))


class Config:
    def __init__(self, server=None, logging=None):
        self.server = ServerConfig(**(server or {}))
        self.logging_level = (logging or {}).get('level', 'info')


def load(path):
    with open(path) as f:
        data = json.load(f)
    return Config(**data)


def default():
    return Config()
