FROM golang:1.24-alpine AS builder

RUN apk add --no-cache clang llvm make git libbpf-dev linux-headers

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make generate && make build

FROM alpine:latest

RUN apk add --no-cache libbpf

COPY --from=builder /build/flowlens /usr/local/bin/flowlens

ENTRYPOINT ["/usr/local/bin/flowlens"]
CMD ["--config=/etc/flowlens/config.yaml"]
