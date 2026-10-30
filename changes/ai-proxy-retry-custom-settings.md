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

# Preserve ai-proxy custom settings on retries

Keep configured parameter overrides and defaults when retrying a failed AI
request with another provider token. Save the request body after applying
custom settings and before provider-specific conversion, so model mapping
and protocol conversion are not repeated on an already converted body.
