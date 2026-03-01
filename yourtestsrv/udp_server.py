import socket
import threading
import time
import random
import logging
from concurrent.futures import ThreadPoolExecutor

logger = logging.getLogger(__name__)


class UDPServer:
    def __init__(self, port, bind='0.0.0.0', drop_rate=0.0, delay=0.0, handler=None):
        self.port = port
        self.bind = bind or '0.0.0.0'
        self.drop_rate = drop_rate
        self.delay = delay
        self.handler = handler

    def listen_and_serve(self, stop_event):
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((self.bind, self.port))
        sock.settimeout(1.0)
        logger.info(f'UDP server listening on {self.bind}:{self.port}')
        executor = ThreadPoolExecutor(max_workers=32)
        try:
            while not stop_event.is_set():
                try:
                    data, addr = sock.recvfrom(65535)
                except socket.timeout:
                    continue
                except OSError:
                    break
                executor.submit(self._handle_packet, sock, addr, data)
        finally:
            sock.close()
            executor.shutdown(wait=False)

    def _handle_packet(self, sock, addr, data):
        if self.drop_rate > 0 and random.random() < self.drop_rate:
            logger.info(f'UDP packet dropped from {addr}')
            return
        if self.delay > 0:
            time.sleep(self.delay)
        logger.info(f'UDP received from {addr}: {data.hex()}')
        if self.handler:
            response = self.handler(addr, data)
        else:
            response = data
        if response:
            try:
                sock.sendto(response, addr)
            except OSError:
                pass
