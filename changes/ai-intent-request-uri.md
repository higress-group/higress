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

# Preserve the configured AI intent service request URI

Retain the complete query string from `llm.proxyUrl` when building the intent
service request, including API-version parameters and repeated query keys.
Default an empty URL path to `/` and exclude URL fragments from HTTP requests.

Keep query values out of the existing path-only logs. Add native and Wasm host
regressions that inspect the dispatched HTTP callout URI and complete its
response callback.
