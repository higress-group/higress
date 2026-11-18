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

# Relay OIDC session cookies from the per-request recorder

The OIDC response phase read `Set-Cookie` values from a context slot written
by the shared oauth2-proxy handler, whose single context field can be
overwritten by a concurrent request between the header and response phases,
leaking one request's session cookies onto another request's response. The
response phase now reads the cookies recorded on the per-request response
recorder (the actual data source), keeping the old context slot only as a
legacy fallback.
