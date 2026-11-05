"""Counterfactual assertions on the baseline, then the same assertions on the candidate."""
import json
from copy import deepcopy

from runtime_lab import Lab, arguments
from verify_runtime import BASE_CONFIG, parse, request_for


def assert_scope(scope, row, body, upstream, stream):
    if scope == 'definition':
        assert not row['upstream_received'], 'tool definition deny did not stop the request'
        assert '已被屏蔽' in body.decode(), 'missing refusal body'
    elif scope == 'request-mask':
        assert row['upstream_received'], 'valid request did not reach fixture'
        argument = json.loads(upstream['messages'][1]['tool_calls'][0]['function']['arguments'])
        assert argument['email'] == 'MASKED_TOKEN', 'request tool argument was not masked'
    elif scope == 'restore':
        actual = parse(body, stream)
        argument = json.loads(next(iter(actual['arguments'].values())))
        assert argument['email'] == 'DEMO_SECRET', 'allowlisted argument was not restored'
        assert argument['other'] == 'MASKED_TOKEN', 'non-allowlisted argument changed'
    else:
        if stream:
            actual = parse(body, True)
            assert not actual['arguments'] and actual['done'] == 0, 'decoded denied argument leaked to the client'
        else:
            assert '已被屏蔽' in body.decode(), 'decoded denied argument was not rejected'


def main():
    args = arguments(__doc__).parse_args()
    results = []
    with Lab(args, 'red-green') as lab:
        settings = deepcopy(BASE_CONFIG)
        settings.update(tool_arguments={'check': True, 'restore': True, 'restore_allowlist': [{'tool': 'lookup', 'paths': ['/email']}]}, tool_definitions={'check': True})
        for variant in ('baseline', 'candidate'):
            gateway = lab.start(variant, settings, variant + '.wasm')
            try:
                for scope, scenario in [('definition', 'definition-deny'), ('request-mask', 'request-arguments'), ('restore', 'normal'), ('deny', 'escaped-deny')]:
                    for stream in (True, False):
                        case = f'{variant}-{scope}-{int(stream)}'
                        row, body = lab.request(gateway, case, request_for(scenario, stream), scenario=scenario)
                        # A transport/bootstrap failure is never accepted as a
                        # failing regression assertion on the baseline.
                        assert row['curl_exit'] == 0 and row['status'] == '200', row
                        path = lab.cases / (case + '.upstream-request')
                        upstream = json.loads(path.read_bytes()) if path.exists() else None
                        if variant == 'baseline':
                            assert upstream is not None and (lab.cases / (case + '.upstream-response')).exists(), 'Baseline did not complete a fixture request'
                        try:
                            assert_scope(scope, row, body, upstream, stream)
                        except AssertionError as error:
                            assert variant == 'baseline', {'case': case, 'error': str(error)}
                            row.update(expected_red=True, assertion_error=str(error), passed=True)
                        else:
                            assert variant == 'candidate', 'Baseline unexpectedly satisfied the new scope assertion: ' + case
                            row.update(expected_red=False, candidate_green=True, passed=True)
                        results.append({**row, 'scope': scope, 'stream': stream})
                        lab.save('results.json', results)
            finally:
                lab.stop(gateway)
        lab.save('summary.json', {'expected_baseline_assertion_failures': sum(row['expected_red'] for row in results),
            'candidate_assertions_passed': sum(row.get('candidate_green', False) for row in results),
            'models': 0, 'full_plan_complete': False})


if __name__ == '__main__':
    main()
