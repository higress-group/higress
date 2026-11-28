# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Compare streaming denial responses through the pinned Higress Envoy gateway."""
import json
import sys
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

# Reuse the repository's pinned gateway, host fixture and owned-resource cleanup.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / 'ai-data-masking-tool-arguments'))
from runtime_lab import Lab, arguments, sha


def main():
    args = arguments(__doc__).parse_args()
    config = {'deny_openai': True, 'deny_raw': True, 'deny_words': ['BLOCKED'],
              'deny_code': 403, 'deny_message': 'blocked',
              'deny_raw_message': 'raw-denial', 'deny_content_type': 'text/plain'}
    results = []
    with Lab(args, 'stream-denial') as lab:
        for variant in ('baseline', 'candidate'):
            gateway = lab.start(variant, config, module=variant + '.wasm')
            for mode in ('stream-1', 'stream-2', 'stream-3', 'json', 'raw'):
                stream = mode.startswith('stream')
                case = variant + '-' + mode
                body = (b'BLOCKED' if mode == 'raw' else json.dumps({
                    'stream': stream, 'messages': [{'role': 'user', 'content': 'BLOCKED'}],
                }).encode())
                content_type = 'text/plain' if mode == 'raw' else 'application/json'
                request = Request(f'http://127.0.0.1:{gateway.port}/v1/chat/completions',
                                  data=body, headers={'Content-Type': content_type, 'X-Test-Case': case})
                (lab.cases / (case + '.request')).write_bytes(body)
                try:
                    response = urlopen(request, timeout=15)
                except HTTPError as error:
                    response = error
                with response:
                    status, headers, response_body = response.code, dict(response.headers), response.read()
                (lab.cases / (case + '.body')).write_bytes(response_body)
                lab.save('cases/' + case + '.headers.json', headers)
                done = response_body.count(b'data: [DONE]\n\n')
                record = {'case': case, 'status': status, 'content_type': headers['content-type'],
                          'body_sha256': sha(response_body), 'done_count': done,
                          'fixed_stream_contract_passes': done == 1 if stream else None}
                results.append(record)
                lab.save('results.json', results)
                assert status == 403, record
                assert not (lab.cases / (case + '.upstream-request')).exists(), case
                if stream:
                    assert headers['content-type'] == 'text/event-stream;charset=UTF-8', record
                    frames = response_body.decode().strip().split('\n\n')
                    events = [json.loads(frame.removeprefix('data:'))
                              for frame in frames if frame.removeprefix('data:').strip() != '[DONE]']
                    text = ''.join(choice.get('delta', {}).get('content', '')
                                   for event in events for choice in event.get('choices', []))
                    assert text == 'blocked', record
                    assert done == (0 if variant == 'baseline' else 1), record
                    if variant == 'candidate':
                        assert response_body.endswith(b'data: [DONE]\n\n'), record
                elif mode == 'json':
                    assert headers['content-type'] == 'application/json', record
                    assert json.loads(response_body)['choices'][0]['message']['content'] == 'blocked'
                    assert done == 0, record
                else:
                    assert headers['content-type'] == 'text/plain', record
                    assert response_body == b'raw-denial', record
                print(json.dumps(record), flush=True)
            lab.stop(gateway)
    cleanup = json.loads((args.output / 'cleanup.json').read_text())
    assert cleanup['network_absent']
    assert all(cleanup['containers_absent'].values())
    assert all(cleanup['ports_not_listening'].values())
    print('PASS: baseline missing-DONE reproduced; fixed stream and nonstream controls passed; resources cleaned')


if __name__ == '__main__':
    main()
