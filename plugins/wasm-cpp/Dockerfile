ARG BUILDER=higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/build-tools-proxy:release-1.19-ef344298e65eeb2d9e2d07b87eb4e715c2def613
FROM $BUILDER AS builder

ARG PLUGIN_NAME

WORKDIR /workspace

COPY . .

RUN bazel build //extensions/$PLUGIN_NAME:$PLUGIN_NAME.wasm

FROM scratch AS output

ARG PLUGIN_NAME

COPY --from=builder /workspace/bazel-bin/extensions/$PLUGIN_NAME/$PLUGIN_NAME.wasm plugin.wasm