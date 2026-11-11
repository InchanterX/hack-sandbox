import json
import queue
import subprocess
import sys
import threading
import unittest
from http.client import HTTPConnection
from pathlib import Path


class HttpProcessSmoke(unittest.TestCase):
    def test_real_process(self):
        root = Path(__file__).resolve().parents[2]
        process = subprocess.Popen(
            [
                sys.executable,
                "-u",
                str(root / "app/server.py"),
                "--host",
                "127.0.0.1",
                "--port",
                "0",
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
        )
        lines = queue.Queue()
        reader = threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True)
        reader.start()
        try:
            line = lines.get(timeout=10)
            self.assertTrue(line.startswith("LISTENING="), line)
            port = int(line.split("=", 1)[1])
            for path, expected_status, expected in (
                ("/healthz", 200, {"status": "ok"}),
                ("/", 200, {"message": "sandbox-repair smoke"}),
                ("/not-a-route", 404, {"error": "not_found"}),
                # Request targets match literally, including query and slash.
                ("/?demo=1", 404, {"error": "not_found"}),
                ("/healthz?demo=1", 404, {"error": "not_found"}),
                ("/healthz/", 404, {"error": "not_found"}),
                ("/not-a-route/nested", 404, {"error": "not_found"}),
            ):
                with self.subTest(path=path):
                    connection = HTTPConnection("127.0.0.1", port, timeout=5)
                    try:
                        connection.request("GET", path)
                        response = connection.getresponse()
                        self.assertEqual(response.status, expected_status)
                        self.assertEqual(response.getheader("Content-Type"), "application/json")
                        body = response.read()
                        self.assertEqual(int(response.getheader("Content-Length")), len(body))
                        self.assertEqual(json.loads(body), expected)
                    finally:
                        connection.close()
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            reader.join(timeout=2)
            process.stdout.close()
