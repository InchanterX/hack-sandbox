"""Small HTTP fixture: the smoke task changes MESSAGE, not the infrastructure."""

import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MESSAGE = "seed"


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            status, body = 200, {"status": "ok"}
        elif self.path == "/":
            status, body = 200, {"message": MESSAGE}
        else:
            status, body = 404, {"error": "not_found"}
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8080)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"LISTENING={server.server_port}", flush=True)
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
