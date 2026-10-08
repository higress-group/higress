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

# Fix Related Resources relative links in skill plugin README

`higress-openclaw-integration/scripts/plugin/README.md` pointed `../SKILL.md`
and `../../higress-auto-router/SKILL.md` at paths that do not exist. Use
`../../SKILL.md` for the parent skill and `../../../higress-auto-router/SKILL.md`
for the sibling skill so both links resolve.

References: Issue #4795.
