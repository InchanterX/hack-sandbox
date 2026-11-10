import json
import threading
import unittest
from http.client import HTTPConnection
from http.server import ThreadingHTTPServer

from app.server import Handler


class HttpContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=5)

    def test_routes(self):
        for path, expected_status, expected in (
            ("/healthz", 200, {"status": "ok"}),
            ("/not-a-route", 404, {"error": "not_found"}),
        ):
            with self.subTest(path=path):
                connection = HTTPConnection("127.0.0.1", self.server.server_port, timeout=5)
                try:
                    connection.request("GET", path)
                    response = connection.getresponse()
                    self.assertEqual(response.status, expected_status)
                    self.assertEqual(response.getheader("Content-Type"), "application/json")
                    self.assertEqual(json.loads(response.read()), expected)
                finally:
                    connection.close()
