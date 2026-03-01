import socket
import ssl
import threading
import time
import logging

logger = logging.getLogger(__name__)


class HTTPRequest:
    def __init__(self, method, path, version, headers, body):
        self.method = method
        self.path = path
        self.version = version
        self.headers = headers
        self.body = body


class HTTPResponse:
    def __init__(self, code=200, message='OK', headers=None, body=None):
        self.code = code
        self.message = message
        self.headers = headers or {}
        self.body = body


class HTTPServer:
    def __init__(self, port, bind='0.0.0.0', slow_response=False, slow_duration=0.0,
                 error_code=0, chunked=False, handler=None):
        self.port = port
        self.bind = bind or '0.0.0.0'
        self.slow_response = slow_response
        self.slow_duration = slow_duration
        self.error_code = error_code
        self.chunked = chunked
        self.handler = handler

    def _serve(self, sock, stop_event):
        sock.settimeout(1.0)
        logger.info(f'HTTP server listening on {self.bind}:{self.port}')
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
        logger.info(f'HTTP TLS server listening on {self.bind}:{self.port}')
        try:
            while not stop_event.is_set():
                try:
                    conn, addr = sock.accept()
                except socket.timeout:
                    continue
                except OSError:
                    break
                try:
                    conn.settimeout(5.0)
                    tls_conn = ctx.wrap_socket(conn, server_side=True)
                    tls_conn.settimeout(None)
                except ssl.SSLError as e:
                    logger.debug(f'HTTP TLS handshake error from {addr}: {e}')
                    conn.close()
                    continue
                t = threading.Thread(target=self._handle_conn, args=(tls_conn, addr), daemon=True)
                t.start()
        finally:
            sock.close()

    def _handle_conn(self, conn, addr):
        conn.settimeout(30.0)
        try:
            buf = b''
            while True:
                try:
                    req, buf = self._parse_request(conn, buf)
                except Exception as e:
                    logger.debug(f'HTTP parse error: {e}')
                    self._send_error(conn, 400, 'Bad Request')
                    return
                if req is None:
                    return
                logger.info(f'HTTP request: {req.method} {req.path} {req.version}')
                if self.handler:
                    resp = self.handler(req)
                else:
                    resp = self._default_handle(req)
                if self.slow_response and self.slow_duration > 0:
                    time.sleep(self.slow_duration)
                if self.error_code > 0 and self.error_code != 200:
                    resp.code = self.error_code
                self._send_response(conn, resp)
                if req.headers.get('connection', '').lower() == 'close':
                    return
        except (ConnectionResetError, BrokenPipeError, OSError):
            pass
        finally:
            try:
                conn.close()
            except Exception:
                pass

    def _recv_until(self, conn, buf, delimiter):
        while delimiter not in buf:
            chunk = conn.recv(4096)
            if not chunk:
                return None, buf
            buf += chunk
        idx = buf.index(delimiter)
        return buf[:idx], buf[idx + len(delimiter):]

    def _parse_request(self, conn, buf):
        line_bytes, buf = self._recv_until(conn, buf, b'\r\n')
        if line_bytes is None:
            return None, buf
        line = line_bytes.decode('latin-1')
        parts = line.split(' ', 2)
        if len(parts) != 3:
            raise ValueError(f'invalid request line: {line!r}')
        method, path, version = parts

        headers = {}
        while True:
            hline_bytes, buf = self._recv_until(conn, buf, b'\r\n')
            if hline_bytes is None:
                return None, buf
            hline = hline_bytes.decode('latin-1')
            if hline == '':
                break
            if ':' in hline:
                k, v = hline.split(':', 1)
                headers[k.strip().lower()] = v.strip()

        body = b''
        content_length = int(headers.get('content-length', 0))
        if content_length > 0:
            while len(buf) < content_length:
                chunk = conn.recv(4096)
                if not chunk:
                    break
                buf += chunk
            body = buf[:content_length]
            buf = buf[content_length:]

        req = HTTPRequest(method, path, version, headers, body)
        return req, buf

    def _send_response(self, conn, resp):
        if resp.headers is None:
            resp.headers = {}
        if self.chunked and 'Transfer-Encoding' not in resp.headers:
            resp.headers['Transfer-Encoding'] = 'chunked'
            resp.headers.pop('Content-Length', None)
        elif resp.body is not None and 'Content-Length' not in resp.headers:
            resp.headers['Content-Length'] = str(len(resp.body))

        header = f'HTTP/1.1 {resp.code} {resp.message}\r\n'
        for k, v in resp.headers.items():
            header += f'{k}: {v}\r\n'
        header += '\r\n'
        conn.sendall(header.encode('latin-1'))

        if self.chunked:
            if resp.body:
                chunk = f'{len(resp.body):x}\r\n'.encode() + resp.body + b'\r\n'
                conn.sendall(chunk)
            conn.sendall(b'0\r\n\r\n')
        elif resp.body:
            conn.sendall(resp.body)

    def _send_error(self, conn, code, message):
        resp = HTTPResponse(code, message, {}, message.encode())
        self._send_response(conn, resp)

    def _default_handle(self, req):
        if req.path == '/healthz':
            return HTTPResponse(200, 'OK', {'Content-Type': 'text/plain'}, b'ok\n')
        body = f'Method: {req.method}\nPath: {req.path}\nVersion: {req.version}\n'
        for k, v in req.headers.items():
            body += f'{k}: {v}\n'
        return HTTPResponse(200, 'OK', {'Content-Type': 'text/plain'}, body.encode())
