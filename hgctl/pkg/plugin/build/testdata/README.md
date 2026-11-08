# Output cleanup regression evidence

Baseline: `485b56238c7f05cb1afcf858bb67c59981bc6a46`.
Environment: Go 1.26.3, macOS arm64. No Docker daemon or gateway is needed to
reproduce the metadata-error path. Both binaries used the same pinned modules.

Build baseline and fixed binaries in their respective checkouts:

```sh
cd hgctl
go build -o /tmp/hgctl-baseline ./cmd/hgctl # baseline checkout
# In the fixed checkout:
go build -o /tmp/hgctl-fixed ./cmd/hgctl
python3 pkg/plugin/build/testdata/verify_cleanup.py /tmp/hgctl-baseline /tmp/hgctl-fixed
go test ./pkg/plugin/build -count=1 -timeout=90s
go test ./pkg/plugin/build -race -count=1 -timeout=90s
go vet ./pkg/plugin/build
```

The script creates private temporary directories, writes a sentinel, and invokes
the actual CLI with a missing model directory. Both variants exit 1 while
generating metadata, before Docker work. Baseline deletes the sentinel; fixed
preserves it byte-for-byte. Temporary fixtures are removed on exit. Actual
binary hashes and outputs are in runtime-comparison.json. baseline-regression.log
shows the same fixed regression test failing on the baseline source.

The unit test also checks removal of invocation-owned temporary files and the
entrypoint, repeated cleanup, and image output. Live SIGINT/Docker failure and
successful container publication were not exercised. This repair deliberately
preserves output directories, including partial products; it does not promise
atomic publication or rollback of overwritten artifacts.
