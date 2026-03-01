import socket
import threading
import time
import unittest

from yourtestsrv.udp_server import UDPServer


def get_free_udp_port():
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


class TestUDPEcho(unittest.TestCase):
    def test_echo(self):
        port = get_free_udp_port()
        stop = threading.Event()
        srv = UDPServer(port, '127.0.0.1')
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        time.sleep(0.1)
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as conn:
                conn.settimeout(2.0)
                deadline = time.time() + 2.0
                while True:
                    conn.sendto(b'hello', ('127.0.0.1', port))
                    try:
                        data, _ = conn.recvfrom(64)
                        self.assertEqual(data, b'hello')
                        break
                    except socket.timeout:
                        if time.time() > deadline:
                            self.fail('no UDP echo received')
        finally:
            stop.set()

    def test_packet_loss(self):
        port = get_free_udp_port()
        stop = threading.Event()
        srv = UDPServer(port, '127.0.0.1', drop_rate=1.0)
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        time.sleep(0.1)
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as conn:
                conn.sendto(b'hello', ('127.0.0.1', port))
                conn.settimeout(0.3)
                try:
                    data, _ = conn.recvfrom(64)
                    self.fail('expected packet drop')
                except socket.timeout:
                    pass
        finally:
            stop.set()

    def test_delay(self):
        port = get_free_udp_port()
        stop = threading.Event()
        srv = UDPServer(port, '127.0.0.1', delay=0.15)
        t = threading.Thread(target=srv.listen_and_serve, args=(stop,), daemon=True)
        t.start()
        time.sleep(0.1)
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as conn:
                conn.settimeout(2.0)
                start = time.time()
                conn.sendto(b'x', ('127.0.0.1', port))
                conn.recvfrom(64)
                elapsed = time.time() - start
                self.assertGreater(elapsed, 0.1)
        finally:
            stop.set()


if __name__ == '__main__':
    unittest.main()
