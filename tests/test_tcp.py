import socket
import ssl
import tempfile
import threading
import time
import unittest

from yourtestsrv.tcp_server import TCPServer


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
    cert_path = td + '/cert.pem'
    key_path = td + '/key.pem'
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


class TestTCPEcho(unittest.TestCase):
    def test_echo(self):
        port = get_free_port()
        stop = threading.Event()
        srv = TCPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(b'hello')
                data = b''
                conn.settimeout(2.0)
                while len(data) < 5:
                    data += conn.recv(16)
                self.assertEqual(data, b'hello')
        finally:
            stop.set()

    def test_delay(self):
        port = get_free_port()
        stop = threading.Event()
        srv = TCPServer(port, '127.0.0.1', delay=0.2)
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                conn.sendall(b'x')
                conn.settimeout(2.0)
                start = time.time()
                conn.recv(1)
                elapsed = time.time() - start
                self.assertGreater(elapsed, 0.15)
        finally:
            stop.set()

    def test_close_after(self):
        port = get_free_port()
        stop = threading.Event()
        srv = TCPServer(port, '127.0.0.1', close_after=0.1)
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            with socket.create_connection(('127.0.0.1', port)) as conn:
                time.sleep(0.3)
                try:
                    conn.sendall(b'x')
                    conn.settimeout(0.5)
                    data = conn.recv(16)
                    self.assertEqual(data, b'', 'expected connection close')
                except (ConnectionResetError, BrokenPipeError, socket.timeout):
                    pass
        finally:
            stop.set()

    def test_tls(self):
        try:
            cert_path, key_path = make_temp_cert()
        except ImportError:
            self.skipTest('cryptography package not available')
        port = get_free_port()
        stop = threading.Event()
        srv = TCPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve_tls, args=(stop, cert_path, key_path), daemon=True)
        t.start()
        wait_tcp(port)
        try:
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            with ctx.wrap_socket(socket.create_connection(('127.0.0.1', port))) as conn:
                conn.sendall(b'hello')
                conn.settimeout(2.0)
                data = b''
                while len(data) < 5:
                    data += conn.recv(16)
                self.assertEqual(data, b'hello')
        finally:
            stop.set()


if __name__ == '__main__':
    unittest.main()
