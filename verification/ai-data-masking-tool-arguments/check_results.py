"""Offline integrity, recorded outcome, cleanup and decoded body checks.

This does not rerun a gateway or convert incomplete coverage into a pass.
"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path


def digest(path, compressed=False):
    value = hashlib.sha256()
    with (gzip.open(path, 'rb') if compressed else path.open('rb')) as source:
        while piece := source.read(1048576):
            value.update(piece)
    return value.hexdigest()


def case_file(root, case, suffix):
    path = root / 'cases' / (case + '.' + suffix)
    if not path.exists() and Path(str(path) + '.gz').exists():
        return Path(str(path) + '.gz'), True
    return path, False


def check(root, allow_failure):
    root = root.resolve()
    listed = set()
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        expected, relative = line.split('  ', 1)
        path = (root / relative).resolve()
        path.relative_to(root)
        assert relative not in listed, 'Duplicate checksum entry: ' + relative
        listed.add(relative)
        assert digest(path) == expected, 'Checksum mismatch: ' + relative
    actual = {str(path.relative_to(root)) for path in root.rglob('*') if path.is_file() and path.name != 'SHA256SUMS'}
    assert actual == listed, {'unlisted': sorted(actual - listed), 'missing': sorted(listed - actual)}
    manifest = json.loads((root / 'manifest.json').read_text())
    for label in ('baseline', 'candidate'):
        field = label + '_wasm_sha256'
        if field in manifest:
            assert digest(root / (label + '.wasm')) == manifest[field], label + ' Wasm mismatch'
    for name, expected in manifest.get('scripts_sha256', {}).items():
        assert digest(root / 'source' / name) == expected, 'Script snapshot mismatch: ' + name
    for field, name in (('fixture_sha256', 'fixture.py'), ('runner_sha256', 'verify_runtime.py')):
        if field in manifest:
            assert digest(root / 'source' / name) == manifest[field], 'Script snapshot mismatch: ' + name
    cleanup = json.loads((root / 'cleanup.json').read_text())
    if 'containers_absent' in cleanup:
        assert all(cleanup['containers_absent'].values()), cleanup
        assert cleanup['network_absent'] and all(cleanup['ports_not_listening'].values()), cleanup
    else:
        assert not cleanup.get('containers_remaining') and not cleanup.get('network_remaining'), cleanup
    failure = (root / 'failure.json').exists() or (root / 'failure.txt').exists()
    if not allow_failure:
        assert not failure, 'Recorded failed attempt; use --allow-recorded-failure only to check its integrity'
        assert (root / 'summary.json').exists(), 'No completed summary'
    verified_bodies, rows = 0, []
    if (root / 'results.json').exists():
        rows = json.loads((root / 'results.json').read_text())
        for row in rows:
            if not allow_failure:
                assert row.get('passed', True) and row.get('failed', 0) == 0, row
            if 'case' not in row:
                continue
            for field, suffixes in {
                'request_sha256': ('request', 'request.json'), 'body_sha256': ('body',),
                'client_body_sha256': ('body',), 'upstream_request_sha256': ('upstream-request',),
                'upstream_response_sha256': ('upstream-response',), 'upstream_body_sha256': ('tap-upstream-response',),
            }.items():
                expected = row.get(field)
                if expected is None:
                    continue
                candidates = [case_file(root, row['case'], suffix) for suffix in suffixes]
                path, compressed = next((item for item in candidates if item[0].exists()), (None, False))
                # Missing upstream response is intentionally represented by an
                # empty body when a request was denied before reaching it.
                if path is None and field == 'upstream_response_sha256' and expected == hashlib.sha256(b'').hexdigest():
                    continue
                assert path is not None, (row['case'], field, 'body missing')
                assert digest(path, compressed) == expected, (row['case'], field)
                verified_bodies += 1
    for file in root.glob('*.results.json'):
        for row in json.loads(file.read_text()):
            if not allow_failure:
                assert row.get('passed', False), row
            for field, suffix in (('request_sha256', 'request'), ('body_sha256', 'body')):
                if field in row:
                    path, compressed = case_file(root, row['case'], suffix)
                    assert digest(path, compressed) == row[field], (row['case'], field)
                    verified_bodies += 1
    verified_fixture_inputs = 0
    for file in (root / 'cases').glob('*.fixture-observation.json'):
        observation = json.loads(file.read_text())
        if not allow_failure:
            assert 'error' not in observation, observation
        case = file.name.removesuffix('.fixture-observation.json')
        path, compressed = case_file(root, case, 'fixture.json')
        assert digest(path, compressed) == observation['sha256'], 'Fixture input mismatch: ' + case
        verified_fixture_inputs += 1
    return {'run': root.name, 'files_verified': len(listed), 'decoded_body_hashes_verified': verified_bodies,
        'fixture_inputs_verified': verified_fixture_inputs,
        'recorded_failure': failure, 'result_rows': len(rows), 'status': 'integrity checked'}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('runs', type=Path, nargs='+')
    parser.add_argument('--allow-recorded-failure', action='store_true')
    args = parser.parse_args()
    for run in args.runs:
        print(json.dumps(check(run, args.allow_recorded_failure)), flush=True)
