import os
import socket
import ssl
import tempfile
import threading
import time
import unittest

from yourtestsrv.http_server import HTTPServer


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


class TestHTTPBasic(unittest.TestCase):
    def test_basic(self):
        port = get_free_port()
        stop = threading.Event()
        srv = HTTPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
                conn.settimeout(2.0)
                data = b''
                while True:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    data += chunk
                self.assertIn(b'200', data)
        finally:
            stop.set()

    def test_chunked(self):
        port = get_free_port()
        stop = threading.Event()
        srv = HTTPServer(port, '127.0.0.1', chunked=True)
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
                conn.settimeout(2.0)
                data = b''
                while True:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    data += chunk
                self.assertIn(b'Transfer-Encoding: chunked', data)
        finally:
            stop.set()

    def test_healthz(self):
        port = get_free_port()
        stop = threading.Event()
        srv = HTTPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(b'GET /healthz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
                conn.settimeout(2.0)
                data = b''
                while True:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    data += chunk
                self.assertIn(b'200', data)
                self.assertIn(b'ok', data)
        finally:
            stop.set()

    def test_tls(self):
        try:
            cert_path, key_path = make_temp_cert()
        except ImportError:
            self.skipTest('cryptography package not available')
        port = get_free_port()
        stop = threading.Event()
        srv = HTTPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve_tls, args=(stop, cert_path, key_path), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            with ctx.wrap_socket(socket.create_connection(('127.0.0.1', port))) as conn:
                conn.sendall(b'GET /healthz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
                conn.settimeout(2.0)
                data = b''
                while True:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    data += chunk
                self.assertIn(b'200', data)
                self.assertIn(b'ok', data)
        finally:
            stop.set()


if __name__ == '__main__':
    unittest.main()
