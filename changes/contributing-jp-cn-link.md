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

# Fix the Chinese contributing-guide link in CONTRIBUTING_JP.md

`CONTRIBUTING_JP.md` linked 中文 to `./CONTRIBUTING.md`, which does not exist.
Point it at `./CONTRIBUTING_CN.md` so the language switcher resolves.

References: Issue #4793.
