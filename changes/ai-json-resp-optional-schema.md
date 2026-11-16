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

# Fix ai-json-resp requests without a JSON schema

Only enable JSON Schema compilation and validation when the optional `jsonSchema`
configuration is provided. Requests without a schema can still extract JSON and
retry invalid model responses instead of failing during request-header handling.
Explicit schemas retain their existing validation behavior, and invalid schema
types and missing service domains retain their original configuration errors.

Add configuration and request-lifecycle regression tests, including an empty
schema and invalid JSON responses without a schema.

Fixes [#5143](https://github.com/higress-group/higress/issues/5143).
