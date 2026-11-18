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

# Hold OIDC request bodies during async session refresh

While the OIDC plugin waits for an async token callout (session refresh or
login redirect), it paused only the header phase. Requests carrying a body
could still flow upstream as soon as their first DATA frame arrived, before
the `X-Forwarded-Access-Token` header was injected, causing intermittent
upstream 401s. The plugin now returns `HeaderStopAllIterationAndWatermark`
so the whole request is held until the callout callback resumes it.
