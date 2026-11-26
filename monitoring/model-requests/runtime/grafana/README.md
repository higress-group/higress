# Remote Grafana browser validation

Run only in the assigned Mac packaging-host slot. This extends the existing
`runtime/` fixture, uses the repository `dashboard.json`, and reuses the five
previously built Wasm files and existing Higress/Prometheus/Python images. It does
not use the local machine's Docker or browser.

The remote directory must be
`/Users/wujieai/ope9626-validation/higress/RUN/runtime/grafana`. The parent runtime
needs its existing scripts, Wasm files, `bootstrap-tags.json`,
`metric-relabelings.json`, `dashboard.json`, and `verify-export.py`. Use a fresh RUN directory with no previous `prometheus-data`. When the shared
slot is released and assigned to OPE-11003, run there:

```sh
OPE9626_RUNTIME_SLOT=OPE-11003 bash run-remote.sh
```

The four service containers sum to 2 CPUs / 2048 MiB. The backfill container runs
first and finishes before services start. All published ports bind to loopback;
Grafana anonymous access and signup are disabled. A random task-only password is
read from a mounted file, never saved in the reports. One existing native Chrome
instance uses a separate task profile. Node heap is 128 MiB, renderer V8 heap is
256 MiB and renderer concurrency is limited to two. These heap limits are not a
hard limit on total native browser RSS; `resources.jsonl` records native process
RSS separately from the container budget. Host disk below 20 GiB, memory free
below 20%, or one-minute load above 8 stops the task.

`import.py` creates a real Prometheus datasource and imports the actual dashboard
through `/api/dashboards/import` with its `DS_PROMETHEUS` input. `browser.mjs` uses
Chrome DevTools Protocol to operate the real Grafana login page, load the actual
dashboard, record browser-originated datasource query requests/responses, and
capture native browser screenshots. It does not serve a mock Grafana page or
substitute API responses. No browser package installation is needed.

Two distinct data paths are retained:

- **Live gateway:** the real Wasm filters and Envoy export are scraped by real
  Prometheus under `job=higress-fixture`. Eight live HTTP/SSE probes check actual terminal-counter increments, and
  real query checks are rerun. Synthetic provider/report-receiver boundaries
  remain; this is not a deployed client or billing-service test.
- **Historical display fixture:** `prepare.py` generates explicitly synthetic
  counters under `job=hour-fixture`; Prometheus `promtool tsdb create-blocks-from
  openmetrics` imports them into a task-only TSDB before startup. Twenty-six hours of series expose the old inclusive-left-boundary extra bucket;
  the last three completed hours cover mixed results (overall 30%, gpt-4 60%, unknown share 10%), failures
  only (0%), and no increments (no rate value), plus an earlier absent hour. An
  unknown-source series verifies source filtering. This is necessary because the
  dashboard's default excludes the current incomplete hour. These counters are
  not shifted real requests, real business history, or billing evidence.

Browser assertions cover login, default source/model, hour-aligned range and
one-hour step, actual canvas rendering, query errors, source/model filtering,
known percentages, zero/absent-hour semantics, and the imported count/percent
units. Screenshots and DOM/query records supplement these assertions; API success
alone is insufficient.

The shell trap removes only task container names (including their anonymous
volumes) and the task network. On final delivery archive the evidence, remove the
task password, native Chrome profile and task TSDB directory, confirm no task
container/network/volume remains, then release the shared slot to OPE-11002. Do
not prune images or alter any unrelated container or global Docker setting.

Production Operator discovery, pilot-agent 15020 aggregation, actual filter-chain
ordering, other protocol converters, real multi-Pod restarts and capacity remain
separate acceptance steps.

Grafana 11.3 rounds a relative `/h` upper bound inclusively to the hour end;
the Prometheus 1h query step aligns returned samples to complete-hour boundaries.
The baseline comparison proves 25 buckets versus 24 after changing the range to
`now-23h/h → now/h-1h+1s`, including the newest completed hour H. Raw end H+999ms
aligns to H at the 1h query step. Management URLs encode plus as `%2B`.

For the targeted disconnect timing check, run `run-disconnect-remote.sh` in the
same exclusive slot after the Grafana containers have been removed. It uses only
gateway/mock, a fixture-only 3s route, and closes after a complete first SSE event.
The normal fixture route remains 300ms and production monitor code is unchanged.

`OPE9626_RENDER_ONLY=1` runs only Prometheus+Grafana for screenshot diagnosis;
it does not run or claim the live gateway/target checks. It still uses real
Grafana queries against the explicit historical TSDB fixture.
