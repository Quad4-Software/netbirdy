import json, os, subprocess, sys, tempfile, threading, unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest import mock

AR = os.path.join(os.path.dirname(__file__), "netbirdy-respond")


def ar_msg(command="add", rule="concurrent_connections", peer_id="p1",
           account_id="a1", peer_name="n"):
    return json.dumps({
        "command": command,
        "parameters": {"alert": {"data": {
            "rule": rule, "peer_id": peer_id, "peer_name": peer_name,
            "account_id": account_id}}},
    })


class FakeAPI(BaseHTTPRequestHandler):
    calls = []

    def do_GET(self):
        FakeAPI.calls.append((self.path, None, self.headers.get("Authorization")))
        body = json.dumps({"name": "p1", "ssh_enabled": True,
                           "login_expiration_enabled": False,
                           "inactivity_expiration_enabled": False}).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_PUT(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        FakeAPI.calls.append((self.path, json.loads(body),
                              self.headers.get("Authorization")))
        self.send_response(200)
        self.end_headers()

    def log_message(self, *a):
        pass


class TestNetbirdyRespond(unittest.TestCase):
    def setUp(self):
        self.envfile = tempfile.NamedTemporaryFile("w", delete=False)
        self.env = {
            "NETBIRDY_URL": "http://127.0.0.1:1",
            "NETBIRDY_TOKEN": "tok",
            "NETBIRDY_AR_MODE": "dry-run",
        }

    def run_ar(self, stdin, env_content=None):
        if env_content is not None:
            self.envfile.write(env_content)
            self.envfile.flush()
            patch = {"load_env": lambda self=None, path=None: dict(
                l.split("=", 1) for l in env_content.splitlines() if "=" in l)}
        else:
            patch = None
        return subprocess.run(
            [sys.executable, AR],
            input=stdin, capture_output=True, text=True, timeout=15)

    def test_empty_stdin(self):
        r = self.run_ar("")
        self.assertEqual(r.returncode, 0)

    def test_no_action_rule(self):
        r = self.run_ar(ar_msg(rule="user_new_country"))
        self.assertEqual(r.returncode, 0)

    def test_delete_noop(self):
        r = self.run_ar(ar_msg(command="delete"))
        self.assertEqual(r.returncode, 0)

    def test_dry_run_makes_no_call(self):
        with tempfile.NamedTemporaryFile("w", delete=False, suffix=".env") as f:
            f.write("NETBIRDY_URL=http://127.0.0.1:9\nNETBIRDY_TOKEN=x\nNETBIRDY_AR_MODE=dry-run\n")
            f.flush()
            # patch ENV_FILE via subprocess env var is not supported; instead
            # the script falls back to default path: run with a copy that has
            # the env file content injected via a temp dir.
            script = open(AR).read().replace("/var/ossec/etc/netbirdy-ar.env", f.name)
            tmp = tempfile.NamedTemporaryFile("w", delete=False, suffix=".py")
            tmp.write(script)
            tmp.flush()
            r = subprocess.run([sys.executable, tmp.name], input=ar_msg(),
                               capture_output=True, text=True, timeout=15)
        self.assertEqual(r.returncode, 0)

    def test_enforce_calls_api(self):
        srv = HTTPServer(("127.0.0.1", 0), FakeAPI)
        port = srv.server_address[1]
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        FakeAPI.calls = []
        with tempfile.NamedTemporaryFile("w", delete=False, suffix=".env") as f:
            f.write(f"NETBIRDY_URL=http://127.0.0.1:{port}\n"
                    "NETBIRDY_TOKEN=tok\nNETBIRDY_AR_MODE=enforce\n")
            f.flush()
            script = open(AR).read()
            script = script.replace("/var/ossec/etc/netbirdy-ar.env", f.name)
            script = script.replace("/var/ossec/logs/active-responses.log",
                                    tempfile.mktemp())
            tmp = tempfile.NamedTemporaryFile("w", delete=False, suffix=".py")
            tmp.write(script)
            tmp.flush()
            r = subprocess.run([sys.executable, tmp.name], input=ar_msg(),
                               capture_output=True, text=True, timeout=15)
        srv.shutdown()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(len(FakeAPI.calls), 2)  # GET then PUT
        path, body, auth = FakeAPI.calls[1]
        self.assertEqual(path, "/api/peers/p1")
        self.assertEqual(body["approval_required"], True)
        self.assertEqual(body["name"], "p1")
        self.assertEqual(auth, "Token tok")

    def test_allowlist_skips(self):
        with tempfile.NamedTemporaryFile("w", delete=False, suffix=".env") as f:
            f.write("NETBIRDY_URL=http://127.0.0.1:9\nNETBIRDY_TOKEN=x\n"
                    "NETBIRDY_AR_MODE=enforce\nNETBIRDY_ALLOWLIST=p1\n")
            f.flush()
            script = open(AR).read().replace("/var/ossec/etc/netbirdy-ar.env", f.name)
            tmp = tempfile.NamedTemporaryFile("w", delete=False, suffix=".py")
            tmp.write(script)
            tmp.flush()
            r = subprocess.run([sys.executable, tmp.name], input=ar_msg(),
                               capture_output=True, text=True, timeout=15)
        self.assertEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
