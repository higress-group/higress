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

# Expose the resolved upstream request to REST-to-MCP templates

Header and body templates can now use `.request.method`, `.request.url`,
`.request.uri`, `.request.path`, `.request.query`, `.request.scheme`, and
`.request.host` after path and query arguments have been resolved. This gives
upstream signing schemes such as HMAC access to the canonical request target.
