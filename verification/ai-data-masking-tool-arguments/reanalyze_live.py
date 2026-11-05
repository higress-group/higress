"""Recheck immutable live captures with the approved completion contract.

No network access, model calls, response replay, or changes to the original run.
"""
import argparse
import hashlib
import json
import shutil
from pathlib import Path

from check_results import check
from verify_live import body_from_tap, compare


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    original = args.input.resolve()
    integrity = check(original, False)
    source_manifest = json.loads((original / 'manifest.json').read_text())
    assert source_manifest['suite'] == 'live-model', 'Input is not a real-model run'
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    output.chmod(0o700)
    (output / 'source').mkdir()
    for path in Path(__file__).resolve().parent.glob('*.py'):
        shutil.copy2(path, output / 'source' / path.name)
    results, input_hashes = [], {}

    def read(path):
        input_hashes[str(path.relative_to(original))] = sha(path)
        return path.read_bytes()

    for old in json.loads(read(original / 'results.json')):
        case, mode = old['case'], old['mode']
        request = json.loads(read(original / 'cases' / (case + '.request')))
        body = read(original / 'cases' / (case + '.body'))
        stream = request['stream']
        assert hashlib.sha256(body).hexdigest() == old['body_sha256']
        if mode == 'direct':
            upstream_request, upstream = request, body
        else:
            traces = list((original / ('tap-' + case)).glob('trace*'))
            assert len(traces) == 1
            value = json.loads(read(traces[0]))
            trace = value.get('http_buffered_trace', value.get('httpBufferedTrace'))
            headers = {header['key'].lower(): header['value'] for header in trace['request']['headers']}
            assert headers.get('x-test-case') == case, 'Tap request correlation differs'
            request_bytes = body_from_tap(trace['request']['body'])
            upstream = body_from_tap(trace['response']['body'])
            assert request_bytes == read(original / 'cases' / (case + '.tap-upstream-request'))
            assert upstream == read(original / 'cases' / (case + '.tap-upstream-response'))
            assert hashlib.sha256(upstream).hexdigest() == old['upstream_body_sha256']
            upstream_request = json.loads(request_bytes)
        checked = compare(request, upstream_request, upstream, body, mode, stream)
        results.append({'case': case, 'mode': mode, 'stream': stream, 'original_coverage': old['coverage'],
            **checked, 'same_request_tap': mode != 'direct', 'body_sha256': hashlib.sha256(body).hexdigest()})
    ledger = json.loads(read(original / 'ledger.json'))
    result = {'input_run': original.name, 'input_sha256sums_sha256': sha(original / 'SHA256SUMS'),
        'input_integrity': integrity, 'source_manifest_sha256': sha(original / 'manifest.json'),
        'model_calls_new': 0, 'model_calls_in_original_ledger': sum(row['live_model_request'] for row in ledger),
        'original_health_calls': sum(row['category'] == 'health' for row in ledger),
        'formal_cases': len(results), 'covered': sum(row['coverage'] == 'covered' for row in results),
        'uncovered': sum(row['coverage'] == 'uncovered' for row in results), 'tools_executed': 0,
        'correction': 'Coverage accepts every present non-null finish_reason, per approved Design. Original run and classification are unchanged.',
        'full_t6_complete': False}
    for name, value in [('results.json', results), ('summary.json', result), ('input-hashes.json', input_hashes)]:
        (output / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    (output / 'SHA256SUMS').write_text(''.join(sha(path) + '  ' + str(path.relative_to(output)) + '\n'
        for path in sorted(output.rglob('*')) if path.is_file() and path.name != 'SHA256SUMS'))
    print(json.dumps(result, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
