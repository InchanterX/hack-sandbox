import json
import queue
import subprocess
import sys
import threading
import unittest
from pathlib import Path
from urllib.request import urlopen


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
            with urlopen(f"http://127.0.0.1:{port}/healthz", timeout=5) as response:
                self.assertEqual(json.load(response), {"status": "ok"})
            with urlopen(f"http://127.0.0.1:{port}/", timeout=5) as response:
                self.assertIsInstance(json.load(response)["message"], str)
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            reader.join(timeout=2)
            process.stdout.close()
