import socket
import threading
import time
import unittest

from yourtestsrv.http_server import HTTPServer


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


if __name__ == '__main__':
    unittest.main()
