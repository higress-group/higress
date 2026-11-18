# Install namespace runtime comparison

Baseline: `485b56238c7f05cb1afcf858bb67c59981bc6a46`.
Go 1.26.3, macOS arm64; dedicated kind cluster, Kubernetes v1.35.0. The exact
kind/client/server versions and node image digests are included beside this file.
Only the repository CRDs were installed; no Higress gateway is necessary to
observe the CLI's Kubernetes resource namespace. This is not a gateway dataplane
test. The cluster and test containers were removed; cleanup output is included.

Build both source variants with `go build -o <binary> ./cmd/hgctl` from hgctl/.
Run from the fixed repository root (use a fresh dedicated cluster/name):

```sh
kind create cluster --name hgctl-namespace-5229 --image kindest/node:v1.35.0 \
  --kubeconfig /tmp/hgctl-5229-kubeconfig --wait 60s
kubectl --kubeconfig /tmp/hgctl-5229-kubeconfig apply -f api/kubernetes/customresourcedefinitions.gen.yaml
kubectl --kubeconfig /tmp/hgctl-5229-kubeconfig create namespace higress-system
kubectl --kubeconfig /tmp/hgctl-5229-kubeconfig create namespace custom-ns
python3 hgctl/pkg/plugin/install/testdata/verify_namespace.py \
  /tmp/hgctl-baseline /tmp/hgctl-fixed /tmp/hgctl-5229-kubeconfig
kind delete cluster --name hgctl-namespace-5229
(cd hgctl && go test ./pkg/plugin/install ./pkg/kubernetes -count=1 -timeout=90s)
(cd hgctl && go vet ./pkg/plugin/install ./pkg/kubernetes)
```

The verifier records actual resource namespaces after real hgctl command parsing,
validation and API creation. It exercises explicit -n, omitted/mismatched
manifest namespaces, option-file namespace, flag-over-option precedence and the
default. Baseline installs all six cases into higress-system; fixed installs
five custom cases into custom-ns and leaves the default in higress-system.
Each fixture is deleted after checking. Binary hashes and actual CLI output
are recorded in runtime-comparison.json; no kubeconfig/credentials are published.
The same regression tests fail on baseline (baseline-regression.log).

Installation-from-source uses the same corrected doInstall boundary, but an
interactive Docker/source installation was not executed. Unit tests separately
check namespace correction, the default, and global namespace preservation.
