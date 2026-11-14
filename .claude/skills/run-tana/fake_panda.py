# Fake Panda for an isolated Tana stack. It serves the favorites, feed and
# sitemap endpoints that the collector calls, so no request reaches real Panda.
#
# Usage: python3 -I fake_panda.py <port> <control-dir>
#
# Control files in <control-dir>, read on each request (whitespace-separated
# category numbers; delete the file to clear it):
#   fail_parse  categories that get an unknown page layout -> sync fails
#   fail_503    categories that get HTTP 503 -> waiting_cooldown, then retry
import http.server
import os
import sys
import urllib.parse

PORT = int(sys.argv[1])
CONTROL = sys.argv[2]


def categories(name):
    try:
        with open(os.path.join(CONTROL, name)) as f:
            return {int(x) for x in f.read().split()}
    except (OSError, ValueError):
        return set()


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        sys.stderr.write("fake-panda %s\n" % (fmt % args))

    def send(self, code, body, ctype="text/html"):
        data = body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        url = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(url.query)
        if url.path.startswith("/favorites"):
            category = int(query.get("favcat", ["0"])[0])
            if category in categories("fail_503"):
                return self.send(503, "busy")
            if category in categories("fail_parse"):
                return self.send(200, "<html><body>unexpected layout</body></html>")
            # The parser needs the category name in a favcat option, the
            # 100-per-page setting, and an explicit empty result.
            options = "".join('<option value="%d">Favorites %d</option>' % (i, i) for i in range(10))
            return self.send(200, '<html><body><select name="favcat">%s</select>'
                             '<select name="f_perpage"><option value="100" selected>100</option></select>'
                             '<p>No favorites found.</p></body></html>' % options)
        if url.path.startswith("/feed"):
            return self.send(200, '<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>fake</title></feed>',
                             "application/atom+xml")
        if url.path.startswith("/sitemap"):
            return self.send(200, '<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></sitemapindex>',
                             "application/xml")
        self.send(404, "not found")

    def do_POST(self):
        self.send(200, '{"gmetadata":[]}', "application/json")


http.server.ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
