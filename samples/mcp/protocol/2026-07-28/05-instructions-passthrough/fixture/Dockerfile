FROM docker.io/library/golang:1.25.0-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

FROM docker.io/library/alpine:3.20
COPY --from=builder /out/server /demo/server
EXPOSE 8080
ENTRYPOINT ["/demo/server"]
