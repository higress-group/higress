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

"""Observe FAIL_CLOSED on an established VM using a separate diagnostic module."""
from runtime_lab import Lab, arguments
from verify_runtime import request_for


def main():
    parser = arguments(__doc__)
    args = parser.parse_args()
    with Lab(args, 'intentional-diagnostic-vm-failure') as lab:
        lab.manifest.update(diagnostic_module=True, intentional_crash_expected=True,
            limitations=['Deliberate diagnostic instrumentation, not a crash of the unmodified candidate.',
                'No production artifact or checkout source was changed.'])
        lab.save('manifest.json', lab.manifest)

        def closed(config):
            plugin = config['static_resources']['listeners'][0]['filter_chains'][0]['filters'][0]['typed_config']['http_filters'][0]['typed_config']['config']
            plugin.pop('fail_open', None)
            plugin['failure_policy'] = 'FAIL_CLOSED'

        gateway = lab.start('fail-closed', {'tool_arguments': {'check': True}}, modify=closed)
        results = []
        row, _ = lab.request(gateway, 'before-host-failure', request_for('normal', True))
        assert row['curl_exit'] == 0 and row['status'] == '200' and row['upstream_received'], row
        results.append(row)
        for case in ('trigger-host-failure', 'after-host-failure', 'still-failed'):
            row, _ = lab.request(gateway, case, request_for('normal', True))
            results.append(row)
            lab.save('results.json', results)
            assert row['curl_exit'] == 0 and row['status'] == '503' and not row['upstream_received'], row
        stats = lab.command(['docker', 'exec', gateway.container, 'curl', '-sf', 'http://127.0.0.1:15000/stats?filter=wasm']).stdout
        (lab.output / 'intentional-failure.wasm-stats').write_text(stats)
        crash = [int(line.rsplit(': ', 1)[-1]) for line in stats.splitlines() if 'crash_total' in line]
        assert crash and max(crash) >= 1, stats
        for line in stats.splitlines():
            if 'worker_fail_open_skip_total' in line:
                assert line.rsplit(': ', 1)[-1] == '0', line
        lab.save('summary.json', {'before_failure_upstream_received': True, 'failed_vm_returns_503': True,
            'later_requests_closed': True, 'intentional_crash_observed': True, 'fail_open_skip': 0,
            'candidate_runtime_crash_claim': False, 'models': 0, 'full_t5_complete': False})


if __name__ == '__main__':
    main()
