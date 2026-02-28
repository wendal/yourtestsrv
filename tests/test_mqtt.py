import socket
import struct
import threading
import time
import unittest

from yourtestsrv.mqtt_server import MQTTServer, MQTT_CONNECT, MQTT_CONNACK, MQTT_PUBLISH


def get_free_port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def wait_tcp(port, timeout=2.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=0.1):
                return
        except (ConnectionRefusedError, socket.timeout, OSError):
            time.sleep(0.05)
    raise RuntimeError(f'server not ready on port {port}')


def append_mqtt_string(data, s):
    b = s.encode('utf-8')
    return data + struct.pack('>H', len(b)) + b


def build_mqtt_packet(packet_type, flags, payload):
    header = (packet_type << 4) | flags
    length = len(payload)
    length_bytes = b''
    while True:
        b = length % 128
        length //= 128
        if length > 0:
            b |= 0x80
        length_bytes += bytes([b])
        if length == 0:
            break
    return bytes([header]) + length_bytes + payload


def build_connect(client_id):
    payload = b''
    payload = append_mqtt_string(payload, 'MQTT')
    payload += bytes([4, 2, 0, 60])
    payload = append_mqtt_string(payload, client_id)
    return build_mqtt_packet(MQTT_CONNECT, 0, payload)


def build_publish(topic, msg):
    payload = b''
    payload = append_mqtt_string(payload, topic)
    payload += msg
    return build_mqtt_packet(MQTT_PUBLISH, 0, payload)


class TestMQTTConnect(unittest.TestCase):
    def test_connect(self):
        port = get_free_port()
        stop = threading.Event()
        srv = MQTTServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(build_connect('testclient'))
                conn.settimeout(2.0)
                buf = b''
                while len(buf) < 4:
                    buf += conn.recv(16)
                self.assertEqual(buf[0] >> 4, MQTT_CONNACK)
        finally:
            stop.set()

    def test_publish(self):
        port = get_free_port()
        stop = threading.Event()
        received = []

        class Handler:
            def on_publish(self, topic, qos, payload, packet_id):
                received.append((topic, payload))

        srv = MQTTServer(port, '127.0.0.1', handler=Handler())
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(build_connect('testclient'))
                conn.settimeout(2.0)
                buf = b''
                while len(buf) < 4:
                    buf += conn.recv(16)
                conn.sendall(build_publish('test/topic', b'hello'))
                time.sleep(0.2)
                self.assertEqual(len(received), 1)
                self.assertEqual(received[0][0], 'test/topic')
        finally:
            stop.set()


if __name__ == '__main__':
    unittest.main()
