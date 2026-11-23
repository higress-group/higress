<!--
Copyright 2026 alibaba

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Fix Gateway API parent references in InferencePool status

Use the Gateway API group and kind (`gateway.networking.k8s.io/Gateway`) in
`InferencePool.status.parents[].parentRef`. Normal reconciliation corrects legacy
`networking.istio.io/Gateway` references for the same Gateway without leaving
duplicate entries.

Add regression coverage for the complete parent identity (`group`, `kind`,
`namespace`, and `name`), same- and cross-namespace references, legacy status
correction, and deduplication across repeated reconciliation.

References: [Bug #4658](https://github.com/higress-group/higress/issues/4658),
[Proposal #4659](https://github.com/higress-group/higress/issues/4659),
[Design #4661](https://github.com/higress-group/higress/issues/4661), and
[SPEC-4659001](https://github.com/higress-group/higress/issues/4659#issuecomment-5582077984).
