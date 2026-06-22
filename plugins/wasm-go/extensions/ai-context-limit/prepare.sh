#!/bin/sh
# prepare.sh — Download BPE vocabulary for ai-context-limit plugin.
# Called by Makefile, root Dockerfile, and root Makefile local-build.

set -e

BPE_DIR="bpe"
BPE_FILE="${BPE_DIR}/o200k_base.tiktoken"
BPE_URL="https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"

if [ -f "$BPE_FILE" ]; then
  exit 0
fi

mkdir -p "$BPE_DIR"
echo "Downloading o200k_base.tiktoken..."

if command -v curl >/dev/null 2>&1; then
  curl -sSfL -o "$BPE_FILE" "$BPE_URL"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "$BPE_FILE" "$BPE_URL"
else
  echo "Error: curl or wget is required to download BPE vocabulary" >&2
  exit 1
fi

echo "Downloaded ${BPE_FILE} ($(wc -c < "$BPE_FILE" | tr -d ' ') bytes)"
