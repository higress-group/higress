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

# Finalize registry changes after watcher construction failures

When a registry replacement cannot be constructed, remove the stopped registry's
stale services and notify consumers after other watchers complete their initial
readiness callbacks. Successful registry and proxy changes can still finish
while reconciliation returns the construction error.

Track pending watcher readiness across retries after timeouts, and restrict
watcher service deletions to the cached value's current registry type and name.
This preserves the existing single-value, last-publisher cache model for shared
hosts.

Add regression coverage for missing authentication Secrets, retry recovery,
mixed updates, notifications, delayed readiness, and shared-host handover.

References: Issue #4394.
