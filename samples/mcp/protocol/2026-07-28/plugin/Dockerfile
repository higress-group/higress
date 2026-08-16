FROM docker.io/library/golang:1.24.4-alpine AS builder

ARG HIGRESS_REPOSITORY=https://github.com/higress-group/higress.git
ARG HIGRESS_REF=14c36d9bd70b3dc38237cda6175b3f9dede0dccd

RUN apk add --no-cache git ca-certificates
RUN git init /src \
    && git -C /src remote add origin "${HIGRESS_REPOSITORY}" \
    && git -C /src fetch --depth=1 origin "${HIGRESS_REF}" \
    && git -C /src checkout --detach FETCH_HEAD

WORKDIR /src/plugins/wasm-go/extensions/mcp-server
RUN GOOS=wasip1 GOARCH=wasm go build \
    -trimpath \
    -buildmode=c-shared \
    -o /out/plugin.wasm \
    .

FROM docker.io/library/alpine:3.20
COPY --from=builder /out/plugin.wasm /opt/mcp-server/plugin.wasm
CMD ["sh", "-c", "cp /opt/mcp-server/plugin.wasm /output/plugin.wasm && chmod 0444 /output/plugin.wasm"]
