## Bug fixes

- Fix native Vertex Gemini tool-call conversion in `ai-proxy`: preserve all
  complete function calls, text, and thought signatures in JSON and streaming
  responses; match tool results by call ID and return parallel results together
  in the original call order, including calls to the same function.
