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

"""Check current upstream content parsing together with opt-in tool masking."""
from copy import deepcopy
import json

from runtime_lab import Lab, arguments, sha
from verify_runtime import configurations, parse


def requests():
    argument = '{"email":"DEMO_SECRET","other":"MASKED_TOKEN"}'
    history = {'role': 'assistant', 'content': None, 'tool_calls': [
        {'id': 'history-0', 'type': 'function',
         'function': {'name': 'lookup', 'arguments': argument}}]}
    parts = [
        {'type': 'text', 'text': 'DEMO_SECRET', 'cache_control': {'type': 'ephemeral'}},
        {'type': 'input_text', 'text': 'DEMO_SECRET'},
        {'type': 'image_url', 'image_url': {'url': 'https://example.com/DEMO_SECRET'}, 'text': 'DEMO_SECRET'},
        {'type': 'custom_part', 'text': 'DEMO_SECRET'},
    ]
    for label, content, denied in [
        ('null-history', 'DEMO_SECRET', False),
        ('content-parts', parts, False),
        ('same-argument-text', argument, False),
        ('bad-number', 42, True),
        ('bad-object', {'text': 'DEMO_SECRET'}, True),
        ('bad-part', [{'type': 'text', 'text': None}], True),
        ('denied-part', [{'type': 'input_text', 'text': 'BLOCKED'}], True),
    ]:
        yield label, {'model': 'public-fixture', 'messages': [
            {'role': 'user', 'content': content}, deepcopy(history)],
            'tools': [{'type': 'function', 'function': {'name': 'lookup', 'description': 'public fixture'}}]}, denied


def main():
    args = arguments(__doc__).parse_args()
    rows, baseline = [], {}
    with Lab(args, 'upstream-content-merge') as lab:
        for variant in ('baseline', 'candidate-off', 'candidate-false', 'candidate-false-limits', 'candidate-on'):
            gateway = lab.start(variant, configurations()[variant],
                                'baseline.wasm' if variant == 'baseline' else 'candidate.wasm')
            try:
                for label, request, denied in requests():
                    for stream in (False, True):
                        request['stream'] = stream
                        case = f'{variant}-{label}-{int(stream)}'
                        lab.save('cases/' + case + '.fixture.json', {
                            'calls': [{'name': 'lookup', 'id': 'call-0', 'arguments':
                                      '{"email":"MASKED_TOKEN","other":"MASKED_TOKEN"}'}]})
                        row, body = lab.request(gateway, case, request, delivery='events')
                        try:
                            assert row['curl_exit'] == 0 and row['status'] == '200', row
                            assert row['upstream_received'] != denied, 'Unexpected upstream access'
                            if denied:
                                assert '已被屏蔽' in body.decode(), 'Missing rejection body'
                            else:
                                upstream_bytes = (lab.cases / (case + '.upstream-request')).read_bytes()
                                upstream = json.loads(upstream_bytes)
                                first = upstream['messages'][0]['content']
                                if label == 'content-parts':
                                    assert first[0]['text'] == first[1]['text'] == 'MASKED_TOKEN'
                                    assert first[0]['cache_control'] == {'type': 'ephemeral'}
                                    assert first[2:] == request['messages'][0]['content'][2:]
                                else:
                                    assert first == request['messages'][0]['content'].replace('DEMO_SECRET', 'MASKED_TOKEN')
                                expected_history = request['messages'][1]['tool_calls'][0]['function']['arguments']
                                if variant == 'candidate-on':
                                    expected_history = expected_history.replace('DEMO_SECRET', 'MASKED_TOKEN')
                                assert upstream['messages'][1]['tool_calls'][0]['function']['arguments'] == expected_history
                                assert upstream['messages'][1]['content'] is None
                                actual = parse(body, stream)
                                assert actual['text'] == {('0', 'content'): 'checking DEMO_SECRET'}
                                arguments_out = [json.loads(value) for value in actual['arguments'].values()]
                                assert arguments_out == [{'email': 'DEMO_SECRET' if variant == 'candidate-on' else 'MASKED_TOKEN',
                                                          'other': 'MASKED_TOKEN'}]
                                assert actual['finish'] == [(0, 'tool_calls')] and actual['done'] == int(stream)
                                assert actual['usage'] == [{'total_tokens': 20}]
                                row['upstream_request_sha256'] = sha(upstream_bytes)
                            key = (label, stream)
                            fingerprint = (row['body_sha256'], row.get('upstream_request_sha256'))
                            if variant == 'baseline':
                                baseline[key] = fingerprint
                            elif variant != 'candidate-on':
                                assert fingerprint == baseline[key], 'Disabled candidate differs from current upstream'
                            row['passed'] = True
                        except Exception as error:
                            row.update(passed=False, error=str(error))
                            raise
                        finally:
                            rows.append(row)
                            lab.save('results.json', rows)
                print(json.dumps({'variant': variant, 'passed': len(rows)}), flush=True)
            finally:
                lab.stop(gateway)
        lab.save('summary.json', {'passed': len(rows), 'failed': 0, 'model_requests': 0})


if __name__ == '__main__':
    main()
