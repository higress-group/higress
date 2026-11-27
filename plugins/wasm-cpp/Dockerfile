ARG BUILDER=higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/build-tools-proxy:release-1.19-ef344298e65eeb2d9e2d07b87eb4e715c2def613@sha256:f352bb6b751999babd802f5c8112840908e7210b7a0373668ea25c6313cf05dc
FROM $BUILDER as builder

ARG PLUGIN_NAME

WORKDIR /workspace

COPY . .

RUN bazel build //extensions/$PLUGIN_NAME:$PLUGIN_NAME.wasm

FROM scratch as output

ARG PLUGIN_NAME

COPY --from=builder /workspace/bazel-bin/extensions/$PLUGIN_NAME/$PLUGIN_NAME.wasm plugin.wasm
