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

# Add e2e conformance coverage for the ai-token-ratelimit plugin

Add a paired Go/YAML conformance case that runs the compiled ai-token-ratelimit
Wasm plugin against an in-cluster redis and the preloaded llm-mock backend. The
case verifies that non-streaming response usage is counted against per-minute
budgets and that an exhausted budget is rejected with 429, `Too many requests`,
and the `X-TokenRateLimit-Reset` header, that budgets are isolated per limit
key, and that streaming requests pass through.
