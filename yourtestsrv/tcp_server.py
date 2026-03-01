import socket
import ssl
import threading
import time
import logging

logger = logging.getLogger(__name__)


class TCPServer:
    def __init__(self, port, bind='0.0.0.0', delay=0.0, close_after=0.0, handler=None):
        self.port = port
        self.bind = bind or '0.0.0.0'
        self.delay = delay
        self.close_after = close_after
        self.handler = handler

    def _serve(self, sock, stop_event):
        sock.settimeout(1.0)
        logger.info(f'TCP server listening on {self.bind}:{self.port}')
        try:
            while not stop_event.is_set():
                try:
                    conn, addr = sock.accept()
                except socket.timeout:
                    continue
                except OSError:
                    break
                t = threading.Thread(target=self._handle_conn, args=(conn, addr), daemon=True)
                t.start()
        finally:
            sock.close()

    def listen_and_serve(self, stop_event):
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((self.bind, self.port))
        sock.listen(128)
        self._serve(sock, stop_event)

    def listen_and_serve_tls(self, stop_event, cert_file, key_file):
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_2
        ctx.load_cert_chain(cert_file, key_file)
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((self.bind, self.port))
        sock.listen(128)
        sock.settimeout(1.0)
        logger.info(f'TCP TLS server listening on {self.bind}:{self.port}')
        try:
            while not stop_event.is_set():
                try:
                    conn, addr = sock.accept()
                except socket.timeout:
                    continue
                except OSError:
                    break
                conn.settimeout(5.0)
                try:
                    tls_conn = ctx.wrap_socket(conn, server_side=True)
                    tls_conn.settimeout(None)
                except ssl.SSLError as e:
                    logger.debug(f'TCP TLS handshake error from {addr}: {e}')
                    conn.close()
                    continue
                t = threading.Thread(target=self._handle_conn, args=(tls_conn, addr), daemon=True)
                t.start()
        finally:
            sock.close()

    def _handle_conn(self, conn, addr):
        logger.info(f'TCP connection from {addr}')
        try:
            if self.close_after > 0:
                time.sleep(self.close_after)
                logger.info(f'TCP connection closed (close-after): {addr}')
                return
            if self.handler:
                self.handler(conn, addr)
            else:
                self._default_handle(conn, addr)
        finally:
            try:
                conn.close()
            except Exception:
                pass

    def _default_handle(self, conn, addr):
        conn.settimeout(30.0)
        try:
            while True:
                if self.delay > 0:
                    time.sleep(self.delay)
                try:
                    data = conn.recv(4096)
                except socket.timeout:
                    return
                if not data:
                    logger.info(f'TCP connection closed by client: {addr}')
                    return
                logger.info(f'TCP received from {addr}: {data.hex()}')
                conn.sendall(data)
        except (ConnectionResetError, BrokenPipeError, OSError):
            pass
