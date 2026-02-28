import socket
import ssl
import struct
import threading
import time
import logging

logger = logging.getLogger(__name__)

MQTT_CONNECT     = 1
MQTT_CONNACK     = 2
MQTT_PUBLISH     = 3
MQTT_PUBACK      = 4
MQTT_PUBREC      = 5
MQTT_PUBREL      = 6
MQTT_PUBCOMP     = 7
MQTT_SUBSCRIBE   = 8
MQTT_SUBACK      = 9
MQTT_UNSUBSCRIBE = 10
MQTT_UNSUBACK    = 11
MQTT_PINGREQ     = 12
MQTT_PINGRESP    = 13
MQTT_DISCONNECT  = 14


def _read_mqtt_string(data, pos):
    if len(data) < pos + 2:
        return None, pos
    length = struct.unpack_from('>H', data, pos)[0]
    pos += 2
    if len(data) < pos + length:
        return None, pos
    return data[pos:pos + length].decode('utf-8', errors='replace'), pos + length


def _build_packet(packet_type, flags, payload):
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


class MQTTServer:
    def __init__(self, port, bind='0.0.0.0', retain_messages=False, handler=None):
        self.port = port
        self.bind = bind or '0.0.0.0'
        self.retain_messages = retain_messages
        self.handler = handler
        self._clients = {}
        self._retained = {}
        self._lock = threading.Lock()

    def _serve(self, sock, stop_event):
        sock.settimeout(1.0)
        logger.info(f'MQTT server listening on {self.bind}:{self.port}')
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
        logger.info(f'MQTT TLS server listening on {self.bind}:{self.port}')
        try:
            while not stop_event.is_set():
                try:
                    conn, addr = sock.accept()
                except socket.timeout:
                    continue
                except OSError:
                    break
                try:
                    tls_conn = ctx.wrap_socket(conn, server_side=True)
                except ssl.SSLError as e:
                    logger.debug(f'MQTT TLS handshake error from {addr}: {e}')
                    conn.close()
                    continue
                t = threading.Thread(target=self._handle_conn, args=(tls_conn, addr), daemon=True)
                t.start()
        finally:
            sock.close()

    def _recv_exact(self, conn, n):
        buf = b''
        while len(buf) < n:
            chunk = conn.recv(n - len(buf))
            if not chunk:
                return None
            buf += chunk
        return buf

    def _read_packet(self, conn):
        first = self._recv_exact(conn, 1)
        if not first:
            return None
        first_byte = first[0]
        packet_type = (first_byte >> 4) & 0x0F
        flags = first_byte & 0x0F

        length = 0
        multiplier = 1
        while True:
            b_bytes = self._recv_exact(conn, 1)
            if not b_bytes:
                return None
            b = b_bytes[0]
            length += (b & 127) * multiplier
            multiplier *= 128
            if (b & 128) == 0:
                break

        payload = b''
        if length > 0:
            payload = self._recv_exact(conn, length)
            if payload is None:
                return None

        return packet_type, flags, payload

    def _handle_conn(self, conn, addr):
        conn.settimeout(60.0)
        logger.info(f'MQTT connection from {addr}')
        try:
            while True:
                result = self._read_packet(conn)
                if result is None:
                    logger.info(f'MQTT client disconnected: {addr}')
                    return
                packet_type, flags, payload = result
                self._handle_packet(conn, addr, packet_type, flags, payload)
        except (ConnectionResetError, BrokenPipeError, OSError, socket.timeout):
            pass
        finally:
            try:
                conn.close()
            except Exception:
                pass

    def _handle_packet(self, conn, addr, packet_type, flags, payload):
        if packet_type == MQTT_CONNECT:
            self._handle_connect(conn, addr, payload)
        elif packet_type == MQTT_PUBLISH:
            self._handle_publish(conn, addr, flags, payload)
        elif packet_type == MQTT_PUBACK:
            if len(payload) >= 2:
                pid = struct.unpack_from('>H', payload)[0]
                logger.info(f'MQTT PUBACK: packetID={pid}')
        elif packet_type == MQTT_PUBREC:
            if len(payload) >= 2:
                pid = struct.unpack_from('>H', payload)[0]
                logger.info(f'MQTT PUBREC: packetID={pid}')
                conn.sendall(_build_packet(MQTT_PUBREL, 2, struct.pack('>H', pid)))
        elif packet_type == MQTT_PUBREL:
            if len(payload) >= 2:
                pid = struct.unpack_from('>H', payload)[0]
                logger.info(f'MQTT PUBREL: packetID={pid}')
                conn.sendall(_build_packet(MQTT_PUBCOMP, 0, struct.pack('>H', pid)))
        elif packet_type == MQTT_SUBSCRIBE:
            self._handle_subscribe(conn, addr, payload)
        elif packet_type == MQTT_UNSUBSCRIBE:
            self._handle_unsubscribe(conn, addr, payload)
        elif packet_type == MQTT_PINGREQ:
            conn.sendall(_build_packet(MQTT_PINGRESP, 0, b''))
        elif packet_type == MQTT_DISCONNECT:
            logger.info(f'MQTT client sent disconnect: {addr}')
            conn.close()

    def _handle_connect(self, conn, addr, payload):
        pos = 0
        protocol_name, pos = _read_mqtt_string(payload, pos)
        if protocol_name is None:
            return
        protocol_level = payload[pos]; pos += 1
        connect_flags = payload[pos]; pos += 1
        keep_alive = struct.unpack_from('>H', payload, pos)[0]; pos += 2
        client_id, pos = _read_mqtt_string(payload, pos)
        if client_id is None:
            return
        clean_session = bool(connect_flags & 0x02)
        logger.info(f'MQTT CONNECT: client={client_id}, clean={clean_session}')
        with self._lock:
            self._clients[client_id] = conn
        connack = _build_packet(MQTT_CONNACK, 0, bytes([0, 0]))
        conn.sendall(connack)
        if self.handler and hasattr(self.handler, 'on_connect'):
            self.handler.on_connect(conn, client_id, clean_session)

    def _handle_publish(self, conn, addr, flags, payload):
        pos = 0
        topic, pos = _read_mqtt_string(payload, pos)
        if topic is None:
            return
        qos = (flags >> 1) & 0x03
        packet_id = 0
        if qos > 0:
            packet_id = struct.unpack_from('>H', payload, pos)[0]
            pos += 2
        msg_payload = payload[pos:]
        logger.info(f'MQTT PUBLISH: topic={topic}, qos={qos}, payload={msg_payload.hex()}')
        if self.retain_messages and msg_payload:
            with self._lock:
                self._retained[topic] = msg_payload
        if self.handler and hasattr(self.handler, 'on_publish'):
            self.handler.on_publish(topic, qos, msg_payload, packet_id)
        if qos == 1:
            conn.sendall(_build_packet(MQTT_PUBACK, 0, struct.pack('>H', packet_id)))
        elif qos == 2:
            conn.sendall(_build_packet(MQTT_PUBREC, 0, struct.pack('>H', packet_id)))

    def _handle_subscribe(self, conn, addr, payload):
        if len(payload) < 2:
            return
        packet_id = struct.unpack_from('>H', payload)[0]
        pos = 2
        return_codes = []
        while pos < len(payload):
            topic, pos = _read_mqtt_string(payload, pos)
            if topic is None:
                break
            if pos < len(payload):
                qos = payload[pos]; pos += 1
                return_codes.append(qos)
                logger.info(f'MQTT SUBSCRIBE: packetID={packet_id}, topic={topic}, qos={qos}')
        response = struct.pack('>H', packet_id) + bytes(return_codes)
        conn.sendall(_build_packet(MQTT_SUBACK, 0, response))

    def _handle_unsubscribe(self, conn, addr, payload):
        if len(payload) < 2:
            return
        packet_id = struct.unpack_from('>H', payload)[0]
        pos = 2
        while pos < len(payload):
            topic, pos = _read_mqtt_string(payload, pos)
            if topic is None:
                break
            logger.info(f'MQTT UNSUBSCRIBE: packetID={packet_id}, topic={topic}')
        conn.sendall(_build_packet(MQTT_UNSUBACK, 0, struct.pack('>H', packet_id)))
