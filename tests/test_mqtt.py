import os
import socket
import ssl
import struct
import tempfile
import threading
import time
import unittest

from yourtestsrv.mqtt_server import MQTTServer, MQTT_CONNECT, MQTT_CONNACK, MQTT_PUBLISH


def get_free_port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def make_temp_cert():
    from cryptography import x509
    from cryptography.x509.oid import NameOID
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import rsa
    import datetime
    import ipaddress
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, 'localhost')])
    cert = (x509.CertificateBuilder()
            .subject_name(subject).issuer_name(issuer)
            .public_key(key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1))
            .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1))
            .add_extension(x509.SubjectAlternativeName([
                x509.DNSName('localhost'),
                x509.IPAddress(ipaddress.IPv4Address('127.0.0.1'))
            ]), critical=False)
            .sign(key, hashes.SHA256()))
    td = tempfile.mkdtemp()
    cert_path = os.path.join(td, 'cert.pem')
    key_path = os.path.join(td, 'key.pem')
    with open(cert_path, 'wb') as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))
    with open(key_path, 'wb') as f:
        f.write(key.private_bytes(serialization.Encoding.PEM,
                                   serialization.PrivateFormat.TraditionalOpenSSL,
                                   serialization.NoEncryption()))
    return cert_path, key_path


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

    def test_tls(self):
        try:
            cert_path, key_path = make_temp_cert()
        except ImportError:
            self.skipTest('cryptography package not available')
        port = get_free_port()
        stop = threading.Event()
        srv = MQTTServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve_tls, args=(stop, cert_path, key_path), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            with ctx.wrap_socket(socket.create_connection(('127.0.0.1', port))) as conn:
                conn.sendall(build_connect('tls-client'))
                conn.settimeout(2.0)
                buf = b''
                while len(buf) < 4:
                    buf += conn.recv(16)
                self.assertEqual(buf[0] >> 4, MQTT_CONNACK)
        finally:
            stop.set()


if __name__ == '__main__':
    unittest.main()
