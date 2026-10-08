import http.server, socketserver, sys, os, functools
os.chdir(sys.argv[1])
h = functools.partial(http.server.SimpleHTTPRequestHandler, directory=sys.argv[1])
h.log_message = lambda *a: None
with socketserver.TCPServer(("127.0.0.1", 0), h) as s:
    open(sys.argv[2], "w").write(str(s.server_address[1]))
    s.serve_forever()
