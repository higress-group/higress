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

"""Pinned gateway comparison for bounded SSE framing and protocol sniffing."""
from copy import deepcopy
import json

from runtime_lab import Lab, arguments
from verify_runtime import BASE_CONFIG, parse, request_for


def cases():
    prefix, suffix = 'data: {"choices":[],"padding":"', '"}'
    frame = prefix + 'x' * (1030 - len(prefix + suffix)) + suffix
    assert len(frame.encode()) == 1030
    yield 'inherited-limit', frame, None, False
    for limit in (16, 1029, 1030, 1031):
        yield 'unterminated-' + str(limit), frame, limit, limit >= 1030
    yield 'closed-over-limit', frame + '\n\n', 1030, False
    yield 'oversized-then-done', frame + '\n\ndata: [DONE]\n\n', 64, False
    small = 'data: {"choices":[]}\n\n'
    yield 'batched', small * 100, len(small.encode()), True
    yield 'undecided-prefix', ' ' * 1030, 16, False
    yield 'json-error', json.dumps({'error': {'message': 'x' * 1030}}, separators=(',', ':')), 16, True


def main():
    args = arguments(__doc__).parse_args()
    rows = []
    with Lab(args, 'unparsed-buffer') as lab:
        for variant, raw_mode in ((variant, raw) for raw in (False, True) for variant in ('baseline', 'candidate')):
            for label, wire, limit, allowed in cases():
                settings = deepcopy(BASE_CONFIG)
                if raw_mode:
                    settings.update(deny_openai=False, deny_raw=True)
                settings['tool_arguments'] = {'check': True, 'limits': {
                    'max_call_bytes': 16, 'max_buffer_bytes': 16}}
                if limit is not None:
                    settings['tool_arguments']['limits']['max_unparsed_bytes'] = limit
                group = variant + '-' + ('raw-' if raw_mode else 'text-') + label
                gateway = lab.start(group, settings, variant + '.wasm')
                try:
                    for delivery in ('single', 'events', 'fragment17', 'prefix1'):
                        case = group + '-' + delivery
                        lab.save('cases/' + case + '.fixture.json', {'raw': wire})
                        row, body = lab.request(gateway, case, request_for('normal', True),
                                                scenario='missing-content-type' if label in ('undecided-prefix', 'json-error') else 'normal',
                                                delivery=delivery)
                        try:
                            assert row['curl_exit'] == 0 and row['status'] == '200' and row['upstream_received'], row
                            original = (lab.cases / (case + '.upstream-response')).read_bytes()
                            assert original == wire.encode(), 'Fixture sent different bytes'
                            if allowed:
                                assert body == original or body == original + b'\n\n', 'Allowed event changed'
                                row['expected_baseline_failure'] = False
                            elif variant == 'baseline':
                                assert body, 'Baseline did not forward the oversized wire event'
                                row['expected_baseline_failure'] = True
                            else:
                                assert body == b'', 'Rejected raw bytes reached the client'
                                assert parse(body, True)['done'] == 0
                                row['expected_baseline_failure'] = False
                            row.update(passed=True, raw_mode=raw_mode, wire_bytes=len(original), max_unparsed_bytes=limit or 16)
                        except Exception as error:
                            row.update(passed=False, error=str(error))
                            raise
                        finally:
                            rows.append(row)
                            lab.save('results.json', rows)
                finally:
                    lab.stop(gateway)
                print(json.dumps({'group': group, 'passed': len(rows)}), flush=True)
        lab.save('summary.json', {'passed': len(rows), 'failed': 0,
            'baseline_scope_failures': sum(row['expected_baseline_failure'] for row in rows), 'model_requests': 0})


if __name__ == '__main__':
    main()
