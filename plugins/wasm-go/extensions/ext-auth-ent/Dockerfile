# AI Token Report Plugin Docker Image

FROM scratch

# Copy the wasm file (Higress expects plugin.wasm)
COPY main.wasm /plugin/plugin.wasm

# Label the image with plugin metadata
LABEL maintainer="Wujie Onepiece Team"
LABEL description="Enterprise short-token ext-auth variant (path-prefix gate, OPE-8474)"
LABEL version="1.0.0"
LABEL plugin-name="ext-auth-ent"
