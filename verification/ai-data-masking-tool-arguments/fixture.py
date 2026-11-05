"""Synthetic OpenAI upstream. No model, credentials or tool execution."""
import argparse
import json
import hashlib
import traceback
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def encode(value):
    return ('data: ' + json.dumps(value, ensure_ascii=False, separators=(',', ':')) + '\n\n').encode()


def response(request, scenario):
    mask = 'MASKED_TOKEN'
    argument = '{ "email":"MASKED_TOKEN", "other":"MASKED_TOKEN", "n":1e2, "u":"\\u4f60\\u597D" }'
    if scenario == 'request-arguments':
        argument = request['messages'][1]['tool_calls'][0]['function']['arguments']
    elif scenario == 'malformed-arguments':
        argument = '{"email":"MASKED_TOKEN"'
    elif scenario == 'escaped-deny':
        argument = '{"BLO\\u0043KED":"ok"}'
    elif scenario == 'deny-value':
        argument = '{"email":"BLOCKED"}'
    elif scenario == 'large':
        size = request.get('_fixture_size', 4096)
        prefix, suffix = '{"email":"MASKED_TOKEN","padding":"', '"}'
        argument = prefix + 'x' * (size - len(prefix.encode()) - len(suffix.encode())) + suffix
        assert len(argument.encode()) == size
    elif scenario == 'nonallowed-malformed':
        argument = 'not JSON MASKED_TOKEN'
    elif scenario == 'null-arguments':
        argument = None
    elif scenario == 'empty-arguments':
        argument = ''
    elif scenario == 'root-string':
        argument = '"MASKED_TOKEN"'
    name = 'external' if scenario == 'nonallowed-malformed' else 'lookup'
    if not request.get('stream', False):
        message = {'role': 'assistant', 'content': 'checking MASKED_TOKEN'}
        call = {'id': 'public-call-0', 'type': 'function', 'function': {'name': name, 'arguments': argument}}
        if scenario == 'legacy':
            message['function_call'] = call['function']
        else:
            message['tool_calls'] = [call]
        value = {'id': 'public-fixture', 'choices': [{'index': 0, 'message': message, 'finish_reason': 'tool_calls'}], 'usage': {'total_tokens': 20}}
        if scenario == 'json-error':
            value = {'error': {'message': 'synthetic upstream failure'}}
        return [json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode()], 'application/json'
    parts = []

    def emit(delta, finish=None, choice=0, **extra):
        parts.append(encode({'id': 'public-fixture', 'choices': [{'index': choice, 'delta': delta, 'finish_reason': finish}], **extra}))

    if scenario == 'json-error':
        return [b'{"error":{"message":"synthetic upstream failure"}}'], 'application/json'
    if scenario == 'controls':
        parts.append(b': keepalive\r\n\r\n')
    emit({'role': 'assistant'})
    text = 'checking MASKED_TOKEN'
    emit({'content': text[:12]})
    if scenario == 'legacy':
        emit({'function_call': {'name': name}})
    else:
        emit({'tool_calls': [{'index': '0' if scenario == 'string-index' else 0, 'id': 'public-call-0', 'type': 'function', 'function': {'name': name[:3]}}]})
        emit({'tool_calls': [{'index': 0, 'function': {'name': name[3:]}}]})
    emit({'content': text[12:]})
    if scenario == 'bad-sse':
        parts.extend([b'data: {bad JSON}\n\n', encode({'choices': [{'index': 0, 'delta': {'content': 'must stay rejected'}}]})])
    elif scenario == 'upstream-error':
        emit({'tool_calls': [{'index': 0, 'function': {'arguments': '{"email":"MASK'}}]})
        parts.append(encode({'error': {'message': 'synthetic upstream failure'}}))
        return parts, 'text/event-stream'
    else:
        for offset in range(0, max(len(argument or ''), 1), 7 if scenario != 'large' else 4096):
            fragment = argument[offset:offset + (7 if scenario != 'large' else 4096)] if isinstance(argument, str) else None
            function = {'arguments': fragment}
            if scenario == 'legacy':
                emit({'function_call': function})
            else:
                call = {'index': 0, 'function': function}
                if scenario == 'id-only' and offset > 0:
                    call = {'id': 'public-call-0', 'function': function}
                if scenario == 'bad-choice':
                    emit({'tool_calls': [call]}, choice='0')
                elif scenario == 'missing-index':
                    call.pop('index', None)
                    emit({'tool_calls': [call]})
                else:
                    emit({'tool_calls': [call]})
    if scenario == 'name-suffix':
        emit({'tool_calls': [{'index': 0, 'function': {'name': '_other'}}]})
    finish = '' if scenario == 'empty-finish' else 'tool_calls'
    if scenario not in ('no-finish', 'http-end'):
        emit({}, finish=finish)
    parts.append(encode({'choices': [], 'usage': {'total_tokens': 20}, 'custom': 'once'}))
    if scenario not in ('no-finish', 'http-end'):
        parts.append(b'data: [DONE]\n\n')
    if scenario == 'http-end':
        parts[-1] = parts[-1].rstrip(b'\n')
    if scenario == 'controls':
        parts[1] = b'event: message\r\nid: public\r\nretry: 1000\r\n' + parts[1].replace(b'\n', b'\r\n')
    return parts, 'text/event-stream'



def custom_response(request, spec):
    """Response controls live in an evidence file, outside the masked request."""
    if 'raw' in spec:
        return [spec['raw'].encode()], spec.get('content_type', 'text/event-stream'), {}
    calls = spec.get('calls', [])
    text = spec.get('content', 'checking MASKED_TOKEN')
    finish = spec.get('finish_reason', 'tool_calls')
    usage = spec.get('usage', {'total_tokens': 20})
    if not request.get('stream', False):
        choices = []
        for choice in sorted({call.get('choice', 0) for call in calls} or {0}):
            message = {'role': 'assistant', 'content': text}
            modern = []
            for ordinal, call in enumerate(calls):
                if call.get('choice', 0) != choice:
                    continue
                function = {'name': call.get('name', 'lookup')}
                if 'arguments' in call:
                    function['arguments'] = call['arguments']
                if call.get('legacy'):
                    message['function_call'] = function
                else:
                    modern.append({'id': call.get('id', 'call-' + str(ordinal)), 'type': 'function', 'function': function})
            if modern:
                message['tool_calls'] = modern
            choices.append({'index': choice, 'message': message, 'finish_reason': finish})
        return [json.dumps({'choices': choices, 'usage': usage}, ensure_ascii=False, separators=(',', ':')).encode()], 'application/json', {}
    parts, gates = [], {}

    def emit(choice, delta, reason=None):
        parts.append(encode({'id': 'public-fixture', 'choices': [{'index': choice, 'delta': delta, 'finish_reason': reason}]}))

    def metadata(call, ordinal):
        function = {'name': call.get('name', 'lookup')}
        if call.get('legacy'):
            emit(call.get('choice', 0), {'function_call': function})
        else:
            emit(call.get('choice', 0), {'tool_calls': [{'index': call.get('index', ordinal), 'id': call.get('id', 'call-' + str(ordinal)), 'type': 'function', 'function': function}]})

    def arguments_part(call, ordinal, fragment):
        function = {'arguments': fragment}
        if call.get('legacy'):
            emit(call.get('choice', 0), {'function_call': function})
        else:
            emit(call.get('choice', 0), {'tool_calls': [{'index': call.get('index', ordinal), 'function': function}]})

    emit(0, {'role': 'assistant', 'content': text})
    fragment_size = spec.get('fragment_size', 4096)
    if spec.get('finish_each'):
        for ordinal, call in enumerate(calls):
            metadata(call, ordinal)
            argument = call.get('arguments')
            if isinstance(argument, str):
                for at in range(0, max(len(argument), 1), fragment_size):
                    arguments_part(call, ordinal, argument[at:at + fragment_size])
            elif 'arguments' in call:
                arguments_part(call, ordinal, argument)
            emit(call.get('choice', 0), {}, finish)
            if spec.get('gate_calls') and ordinal + 1 < len(calls):
                gates[len(parts) - 1] = ordinal
    else:
        for ordinal, call in enumerate(calls):
            metadata(call, ordinal)
        if spec.get('gate_before_arguments') and calls:
            gates[len(parts)-1]='start'
        maximum = max((len(call['arguments']) for call in calls if isinstance(call.get('arguments'), str)), default=1)
        for at in range(0, max(maximum, 1), fragment_size):
            for ordinal, call in enumerate(calls):
                argument = call.get('arguments')
                if isinstance(argument, str) and (at < len(argument) or at == 0 and not argument):
                    arguments_part(call, ordinal, argument[at:at + fragment_size])
                elif at == 0 and 'arguments' in call and not isinstance(argument, str):
                    arguments_part(call, ordinal, argument)
        if finish is not None:
            for choice in sorted({call.get('choice', 0) for call in calls} or {0}):
                emit(choice, {}, finish)
    if usage is not None:
        parts.append(encode({'choices': [], 'usage': usage}))
    if spec.get('done', finish is not None):
        parts.append(b'data: [DONE]\n\n')
    if spec.get('tail_no_blank'):
        parts[-1] = parts[-1].rstrip(b'\n')
    return parts, 'text/event-stream', gates


class FixtureServer(ThreadingHTTPServer):
    request_queue_size = 128

class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def log_message(self, *_):
        pass

    def do_POST(self):
        if self.headers.get('Transfer-Encoding', '').lower() == 'chunked':
            pieces = []
            while True:
                length = int(self.rfile.readline().split(b';')[0], 16)
                if length == 0:
                    while self.rfile.readline() != b'\r\n':
                        pass
                    break
                pieces.append(self.rfile.read(length))
                assert self.rfile.read(2) == b'\r\n'
            raw = b''.join(pieces)
        else:
            raw = self.rfile.read(int(self.headers['Content-Length']))
        case = self.headers['X-Test-Case']
        if not case or any(character not in 'abcdefghijklmnopqrstuvwxyz0123456789-_' for character in case):
            self.send_error(400)
            return
        destination = self.server.evidence
        (destination / (case + '.upstream-request')).write_bytes(raw)
        request = json.loads(raw)
        specification=destination / (case+'.fixture.json')
        if specification.exists():
            observed = specification.read_bytes()
            observation = {'bytes': len(observed), 'sha256': hashlib.sha256(observed).hexdigest()}
            try:
                parts,content_type,gates=custom_response(request,json.loads(observed))
            except Exception:
                observation['error'] = traceback.format_exc()
                (destination / (case + '.fixture-observation.json')).write_text(json.dumps(observation) + '\n')
                raise
            (destination / (case + '.fixture-observation.json')).write_text(json.dumps(observation) + '\n')
        else:
            parts,content_type=response(request,self.headers.get('X-Scenario','normal'))
            gates={}
        wire = b''.join(parts)
        (destination / (case + '.upstream-response')).write_bytes(wire)
        delivery = self.headers.get('X-Delivery', 'single')
        writes = parts if delivery == 'events' or gates else [wire]
        if delivery == 'fragment17':
            writes = [wire[at:at + 17] for at in range(0, len(wire), 17)]
        elif delivery == 'prefix1':
            writes = [wire[:1]] + [wire[at:at + 17] for at in range(1, len(wire), 17)]
        self.send_response(200)
        if self.headers.get('X-Scenario') != 'missing-content-type':
            self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', str(len(wire)))
        self.end_headers()
        times = []
        try:
            for position,piece in enumerate(writes):
                self.wfile.write(piece)
                self.wfile.flush()
                times.append({'bytes':len(piece),'monotonic':time.monotonic()})
                if position in gates:
                    gate=gates[position]
                    acknowledgement=destination / (case+'.ack'+str(gate))
                    deadline=time.monotonic()+20
                    while not acknowledgement.exists() and time.monotonic()<deadline:
                        time.sleep(.005)
                    times.append({'gate':gate,'acknowledged':acknowledgement.exists(),'monotonic':time.monotonic()})
                    if not acknowledgement.exists():
                        break
        except (BrokenPipeError, ConnectionResetError):
            times.append({'client_disconnected': True})
        (destination / (case + '.writes.json')).write_text(json.dumps(times) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--evidence', type=Path, required=True)
    arguments = parser.parse_args()
    arguments.evidence.mkdir(parents=True, exist_ok=True)
    server = FixtureServer(('0.0.0.0', 18080), Handler)
    server.evidence = arguments.evidence
    server.serve_forever()
