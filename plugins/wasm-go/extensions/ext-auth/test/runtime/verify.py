#!/usr/bin/env python3
# Copyright (c) 2026 Alibaba Group Holding Ltd.
# Licensed under the Apache License, Version 2.0.
"""Linux/Docker proxy-Wasm regression; only synthetic credentials and loopback.

Exit successfully only when every expected baseline/fixed assertion holds.
Writes bootstrap, client results, logs, metrics and cleanup evidence to --output.
"""
import argparse
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import shlex
import socket
import subprocess
import threading
import time
import urllib.request
import uuid


def sha(data):
    return hashlib.sha256(data).hexdigest()


def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def bootstrap(listener, admin, mock):
    cluster = 'outbound|8080||auth.example'
    config = {'http_service': {'endpoint_mode': 'forward_auth',
        'endpoint': {'service_name': 'auth.example', 'service_port': 8080,
                     'path': '/auth', 'request_method': 'POST'}, 'timeout': 1000,
        'authorization_request': {'allowed_headers': [{'exact': 'x-api-key'}, {'exact': 'x-test-id'}],
            'preserve_headers_on_internal_redirect': ['authorization', 'x-api-key']}}}
    policy = {'custom_response_matcher': {'matcher_list': {'matchers': [{
        'predicate': {'single_predicate': {'input': {'name': 'status', 'typed_config': {
            '@type': 'type.googleapis.com/envoy.type.matcher.v3.HttpResponseStatusCodeClassMatchInput'}},
            'value_match': {'exact': '5xx'}}},
        'on_match': {'action': {'name': 'redirect', 'typed_config': {
            '@type': 'type.googleapis.com/udpa.type.v1.TypedStruct',
            'type_url': 'type.googleapis.com/envoy.extensions.http.custom_response.redirect_policy.v3.RedirectPolicy',
            'value': {'use_original_request_uri': True, 'use_original_request_body': True,
                'max_internal_redirects': 1, 'only_redirect_upstream_code': False,
                'keep_original_response_code': False,
                'request_headers_to_add': [{'header': {'key': 'x-test-fallback', 'value': 'true'}, 'append': False}]}}}}}]}}}
    routes = []
    for role in ['backup', 'primary']:
        match = {'prefix': '/request'}
        if role == 'backup':
            match['headers'] = [{'name': 'x-test-fallback', 'string_match': {'exact': 'true'}}]
        route = {'name': role, 'match': match, 'route': {'cluster': cluster, 'timeout': '5s'},
            'request_headers_to_add': [{'header': {'key': k, 'value': v}, 'append': False} for k, v in [
                ('x-test-role', role), ('authorization', 'Bearer fixture-' + role), ('x-api-key', 'fixture-' + role)]]}
        if role == 'primary':
            route['typed_per_filter_config'] = {'envoy.filters.http.custom_response': {
                '@type': 'type.googleapis.com/udpa.type.v1.TypedStruct',
                'type_url': 'type.googleapis.com/envoy.extensions.filters.http.custom_response.v3.CustomResponse',
                'value': policy}}
        routes.append(route)
    return {'admin': {'access_log_path': '/dev/null', 'address': {'socket_address': {'address': '127.0.0.1', 'port_value': admin}}},
        'static_resources': {'listeners': [{'name': 'test', 'address': {'socket_address': {'address': '127.0.0.1', 'port_value': listener}},
            'filter_chains': [{'filters': [{'name': 'envoy.filters.network.http_connection_manager', 'typed_config': {
                '@type': 'type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager',
                'stat_prefix': 'redirect_identity',
                'route_config': {'name': 'test', 'virtual_hosts': [{'name': 'test', 'domains': ['*'], 'routes': routes}]},
                'access_log': [{'name': 'envoy.access_loggers.stdout', 'typed_config': {
                    '@type': 'type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog',
                    'log_format': {'text_format_source': {'inline_string': '%REQ(X-TEST-ID)% %ROUTE_NAME% %RESPONSE_CODE%\n'}}}}],
                'http_filters': [
                    {'name': 'envoy.filters.http.custom_response', 'typed_config': {
                        '@type': 'type.googleapis.com/envoy.extensions.filters.http.custom_response.v3.CustomResponse'}},
                    {'name': 'ext-auth-runtime', 'typed_config': {
                        '@type': 'type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm',
                        'config': {'name': 'ext-auth-runtime', 'vm_config': {'vm_id': 'ext-auth-runtime',
                            'runtime': 'envoy.wasm.runtime.v8', 'code': {'local': {'filename': '/test/plugin.wasm'}}},
                            'configuration': {'@type': 'type.googleapis.com/google.protobuf.StringValue', 'value': json.dumps(config)}}}},
                    {'name': 'envoy.filters.http.router', 'typed_config': {'@type': 'type.googleapis.com/envoy.extensions.filters.http.router.v3.Router'}}]
            }}]}]}], 'clusters': [{'name': cluster, 'type': 'STATIC', 'connect_timeout': '1s',
                'load_assignment': {'cluster_name': cluster, 'endpoints': [{'lb_endpoints': [{'endpoint': {
                    'address': {'socket_address': {'address': '127.0.0.1', 'port_value': mock}}}}]}]}}]}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--wasm', required=True, type=Path)
    parser.add_argument('--gateway-image', required=True)
    parser.add_argument('--docker', default='docker', help='Docker command, e.g. "sudo -n docker"')
    parser.add_argument('--expect', choices=['legacy', 'fixed'], required=True)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    assert '@sha256:' in args.gateway_image, 'pin the gateway image by digest'
    args.output.mkdir(parents=True, exist_ok=False)
    docker = shlex.split(args.docker)
    def run(*command):
        return subprocess.check_output(docker + list(command), stderr=subprocess.STDOUT).decode()
    records = []
    class Handler(BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'
        def log_message(self, *_): pass
        def handle(self):
            try: super().handle()
            except ConnectionResetError: pass  # Envoy closes pooled sockets on shutdown.
        def do_POST(self):
            if self.headers.get('Transfer-Encoding', '').lower() == 'chunked':
                chunks = []
                while True:
                    length = int(self.rfile.readline().split(b';')[0].strip(), 16)
                    if not length:
                        while self.rfile.readline().strip(): pass
                        break
                    chunks.append(self.rfile.read(length))
                    assert self.rfile.read(2) == b'\r\n'
                raw = b''.join(chunks)
            else:
                raw = self.rfile.read(int(self.headers.get('Content-Length', '0')))
            auth, key = self.headers.get('authorization', ''), self.headers.get('x-api-key', '')
            role = 'auth' if self.path == '/auth' else self.headers.get('x-test-role')
            status = (200 if (auth == 'Bearer fixture-client' if auth else key == 'fixture-client') else 401) if role == 'auth' else (
                int(self.headers.get('x-test-status', '503')) if role == 'primary' else 200)
            correct = (auth == 'Bearer fixture-' + role and key == 'fixture-' + role) if role != 'auth' else status == 200
            records.append({'case': self.headers.get('x-test-id'), 'role': role, 'status': status,
                            'credentials_correct': correct, 'body_sha256': sha(raw)})
            response = raw if role != 'auth' and status == 200 else b'{}'
            self.send_response(status)
            self.send_header('Content-Length', str(len(response)))
            self.end_headers()
            self.wfile.write(response)
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    listener, admin = port(), port()
    while listener == admin: admin = port()
    config = bootstrap(listener, admin, server.server_port)
    (args.output / 'envoy.json').write_text(json.dumps(config, indent=2))
    container = 'ext-auth-runtime-' + uuid.uuid4().hex[:12]
    results = []
    report = {'expected_variant': args.expect, 'gateway_image': args.gateway_image,
              'wasm_sha256': sha(args.wasm.read_bytes()), 'config_sha256': sha(json.dumps(config, sort_keys=True).encode()),
              'policy_sha256': sha(json.dumps(bootstrap(0, 0, 0), sort_keys=True).encode())}
    started = False
    try:
        report['envoy_version'] = run('run', '--rm', '--entrypoint', '/usr/local/bin/envoy', args.gateway_image, '--version').strip()
        started = True
        run('run', '-d', '--name', container, '--network', 'host', '--entrypoint', '/usr/local/bin/envoy',
            '-v', str(args.wasm.resolve()) + ':/test/plugin.wasm:ro',
            '-v', str((args.output / 'envoy.json').resolve()) + ':/test/envoy.json:ro',
            args.gateway_image, '-c', '/test/envoy.json', '--concurrency', '1', '--disable-hot-restart', '--log-level', 'warning')
        for _ in range(30):
            try:
                urllib.request.urlopen(f'http://127.0.0.1:{admin}/ready', timeout=1).close(); break
            except OSError: time.sleep(.2)
        else: raise AssertionError('Envoy did not become ready')
        connection = http.client.HTTPConnection('127.0.0.1', listener, timeout=10)
        client_socket = None
        try:
            for iteration in range(3):
                for auth_mode in ['authorization', 'x-api-key']:
                    for status in [200, 503]:
                        case = f'{iteration}-{auth_mode}-{status}'
                        # This is an identity regression, not an AI request-body
                        # reconstruction test. Redirect an empty request; separately
                        # check binary transparency on the non-redirected path.
                        payload = b'\x00\xfforiginal request body' if status == 200 else b''
                        headers = {'x-test-id': case, 'x-test-status': str(status),
                                   auth_mode: ('Bearer ' if auth_mode == 'authorization' else '') + 'fixture-client'}
                        connection.request('POST', '/request', body=payload, headers=headers)
                        if client_socket is None: client_socket = connection.sock
                        assert connection.sock is client_socket, 'TCP connection was not reused'
                        response = connection.getresponse(); body = response.read()
                        expected = 401 if args.expect == 'legacy' and status == 503 else 200
                        assert response.status == expected, (case, response.status, expected)
                        events = [r for r in records if r['case'] == case]
                        providers = [r for r in events if r['role'] != 'auth']
                        assert all(r['credentials_correct'] and r['body_sha256'] == sha(payload) for r in providers)
                        assert len([r for r in events if r['role'] == 'auth']) == (2 if status == 503 else 1)
                        assert [r['role'] for r in providers] == (['primary', 'backup'] if status == 503 and args.expect == 'fixed' else ['primary'])
                        assert expected != 200 or body == payload
                        results.append({'case': case, 'status': response.status, 'body_sha256': sha(body), 'events': events})
                    # This new HTTP request uses the SAME TCP connection after a valid
                    # request/redirect. A connection-lifespan identity would accept it.
                    case = f'{iteration}-{auth_mode}-invalid'
                    connection.request('POST', '/request', body=b'bad', headers={
                        'x-test-id': case, auth_mode: 'invalid', 'x-hi-original-auth': 'Bearer fixture-client'})
                    assert connection.sock is client_socket, 'invalid-request isolation did not reuse TCP connection'
                    response = connection.getresponse(); response.read()
                    assert response.status == 401
                    assert not any(r['role'] != 'auth' for r in records if r['case'] == case)
                    results.append({'case': case, 'status': response.status})
        finally:
            connection.close()
        report['results'] = results
        report['same_tcp_connection_verified'] = True
        report['assertions_passed'] = True
        (args.output / 'stats.txt').write_bytes(urllib.request.urlopen(f'http://127.0.0.1:{admin}/stats', timeout=2).read())
    finally:
        report['results'] = results
        report['events'] = records
        if started and run('ps', '-aq', '--filter', 'name=^/' + container + '$').strip():
            try:
                run('stop', '--time', '5', container)
                (args.output / 'envoy.log').write_text(run('logs', container))
            finally: run('rm', '-f', container)
        server.shutdown(); server.server_close(); thread.join(timeout=5)
        report['container_removed'] = not run('ps', '-aq', '--filter', 'name=^/' + container + '$').strip()
        report['mock_thread_stopped'] = not thread.is_alive()
        for name, value in [('listener', listener), ('admin', admin), ('mock', server.server_port)]:
            with socket.socket() as sock:
                report[name + '_port_closed'] = sock.connect_ex(('127.0.0.1', value)) != 0
        (args.output / 'report.json').write_text(json.dumps(report, indent=2))
        assert report['container_removed'] and report['mock_thread_stopped']
        assert all(report[name + '_port_closed'] for name in ['listener', 'admin', 'mock'])
    print(json.dumps({'assertions_passed': True, 'cases': len(results), 'wasm_sha256': report['wasm_sha256']}))


if __name__ == '__main__': main()
