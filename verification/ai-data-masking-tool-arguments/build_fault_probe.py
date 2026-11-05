"""Build an isolated diagnostic Wasm that deliberately traps on one test case.

Never use this module as a candidate or production artifact. Tracked source is
exported to a separate target directory; the checkout is never modified.
"""
import argparse
import difflib
import hashlib
import io
import json
import subprocess
import tarfile
from pathlib import Path


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', type=Path, required=True)
    parser.add_argument('--revision', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--builder', default='higress5213-builder')
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    repo = args.repo.resolve()
    revision = subprocess.check_output(['git', 'rev-parse', args.revision], cwd=repo, text=True).strip()
    archive = subprocess.check_output(['git', 'archive', revision, 'plugins/wasm-rust'], cwd=repo)
    destination = repo / 'plugins/wasm-rust/target/fault-probe-source'
    destination.mkdir(exist_ok=False)
    with tarfile.open(fileobj=io.BytesIO(archive)) as stream:
        stream.extractall(destination, filter='data')
    relative = 'plugins/wasm-rust/extensions/ai-data-masking/src/ai_data_masking.rs'
    source = destination / relative
    before = source.read_text()
    anchor = '    ) -> HeaderAction {\n        if has_request_body() {'
    assert before.count(anchor) == 1
    after = before.replace(anchor, '    ) -> HeaderAction {\n        if self.get_http_request_header("x-test-case").as_deref() == Some("trigger-host-failure") {\n            panic!("intentional owned FAIL_CLOSED verification probe");\n        }\n        if has_request_body() {')
    source.write_text(after)
    diff = ''.join(difflib.unified_diff(before.splitlines(True), after.splitlines(True), fromfile='a/' + relative, tofile='b/' + relative)).encode()
    (output / 'instrumentation.diff').write_bytes(diff)
    command = ['docker', 'exec', '-w', '/src/target/fault-probe-source/plugins/wasm-rust/extensions/ai-data-masking',
        '-e', 'CARGO_TARGET_DIR=/src/target/fault-probe-wasm', args.builder,
        'cargo', 'build', '--locked', '--offline', '--release', '--target', 'wasm32-wasip1']
    with (output / 'build.log').open('w') as log:
        result = subprocess.run(command, stdout=log, stderr=subprocess.STDOUT)
    manifest = {'diagnostic_only': True, 'source_revision': revision, 'archive_sha256': digest(archive),
        'instrumentation_sha256': digest(diff), 'modified_source_sha256': digest(source.read_bytes()),
        'command': command, 'exit_code': result.returncode}
    if result.returncode == 0:
        module = repo / 'plugins/wasm-rust/target/fault-probe-wasm/wasm32-wasip1/release/ai_data_masking.wasm'
        body = module.read_bytes()
        (output / 'fault-probe.wasm').write_bytes(body)
        manifest.update(wasm_sha256=digest(body), wasm_bytes=len(body))
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps(manifest), flush=True)
    raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
