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

# Preserve buffered ai-search streaming answers

Flush head-reference content when an SSE response ends with a finish reason,
`[DONE]`, or HTTP end-of-stream, including an unterminated final SSE event.
Preserve all buffered reasoning text and split `</think>` tags, and insert
references only once even when a network chunk contains several SSE events.

Add regression coverage for short and Unicode answers, reasoning streams,
terminal metadata, fragmented SSE events, and unchanged tail-reference behavior.

Related: [Bug #2359](https://github.com/higress-group/higress/issues/2359).
