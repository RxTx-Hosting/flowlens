FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y clang llvm make git libbpf-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make generate && make build

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y libbpf1 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/flowlens /usr/local/bin/flowlens

ENTRYPOINT ["/usr/local/bin/flowlens"]
CMD ["--config=/etc/flowlens/config.yaml"]
