# Build a tiny emulator that listens on the device-plugin status unix socket
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY main.go .
COPY go.mod .
RUN apk add --no-cache git && \
    go build -o /out/emulator main.go

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/emulator /usr/local/bin/emulator
ENTRYPOINT ["/usr/local/bin/emulator"]
