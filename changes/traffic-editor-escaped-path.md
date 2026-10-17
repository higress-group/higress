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

# Preserve escaped request paths when editing query parameters

Keep the escaped request path in traffic-editor's editing context. Adding,
replacing, or deleting query parameters must not decode escaped slashes,
question marks, hash signs, or percent signs and change the upstream resource.

Add regression coverage for repeated query edits, multi-valued query parameters,
encoded UTF-8, and replacement paths containing escaped delimiters.
