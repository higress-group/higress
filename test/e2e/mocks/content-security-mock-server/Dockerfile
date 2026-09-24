FROM golang:1.21.0-alpine3.17 AS builder

# Build Args
ARG BUILD
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.mod
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN if [[ "${BUILD}" != "CI" ]]; then go env -w GOPROXY=https://goproxy.io,direct; fi
RUN go env
RUN go mod download

# Copy the go source
COPY main.go .

# Build
RUN env
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="${LDFLAGS}" -a -o content-security-mock main.go

FROM alpine:3.20.5
WORKDIR /app
COPY --from=builder /app/content-security-mock .
EXPOSE 8090
CMD ["/app/content-security-mock"]
