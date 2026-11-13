# Cache-backed snapshot scan regression

Baseline: `36f2f1ac5c7305c5a46c296c617b45d1fbacb7d3`.
Environment: Go 1.26.3, macOS arm64, Apple M4 Pro; repository-pinned modules.

The test uses real IngressConfig.List, real annotation parsing and the registry
memory cache, with fake ingress-controller inputs. It checks cached/registry
plugin and service resources, cache changes, namespace guards, Gateway API
plugins and the MCP annotation refresh performed by route/config conversion.
The same tests fail on the baseline because plugin/service snapshots rescan
Ingresses. Baseline failure, fixed package results, race and vet output are saved.

From the repository root:

```sh
go test ./pkg/ingress/config ./pkg/ingress/translation ./pkg/ingress/mcp -count=1 -timeout=180s
go test ./pkg/ingress/config -run 'TestIndependentSnapshots|TestRouteSnapshotStill|TestPluginSnapshotPreserves' -race -count=1 -timeout=120s
go vet ./pkg/ingress/config ./pkg/ingress/translation ./pkg/ingress/mcp
go test ./pkg/ingress/config -run '^$' -bench '^BenchmarkIndependentPluginSnapshot$' -benchtime=100x -count=1 -timeout=90s
```

For a baseline comparison, copy snapshot_test.go into a separate checkout at the
baseline SHA and run the same commands. The benchmark results are summarized in
benchmark.json. Both variants use metadata-only ingress configs, empty plugin
output and default INFO logging. These numbers demonstrate removal of unrelated
scan work; they do not measure a live batch, gateway propagation, production CPU,
full route conversion or xDS ACK latency. Zero empty-snapshot allocations do not
imply populated snapshots allocate nothing. No production/Gateway incident fix
or kind/Higress runtime verification is claimed by these native tests.

No debounce or publication behavior is changed. Issue #5247 remains open for
end-to-end diagnosis. The source change is deliberately limited to bypassing
wrapper creation for the two existing converters that never consume wrappers.

Saved logs normalize tabs to spaces and strip trailing whitespace; assertions and results are unchanged.
