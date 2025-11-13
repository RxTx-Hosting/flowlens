.PHONY: all build clean generate install

BIN_NAME=flowlens
BPF_SRC=bpf/flow_monitor.c
BPF_OBJ=bpf/flow_monitor.o

all: generate build

generate:
	go generate ./pkg/ebpf

build: generate
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BIN_NAME) ./cmd/flowlens

clean:
	rm -f $(BIN_NAME)
	rm -f $(BPF_OBJ)
	rm -f pkg/ebpf/flowmonitor_*.go
	rm -f pkg/ebpf/flowmonitor_*.o

install: build
	sudo cp $(BIN_NAME) /usr/local/bin/
	sudo mkdir -p /etc/flowlens
	sudo cp config.example.yaml /etc/flowlens/config.yaml.example

deps:
	go install github.com/cilium/ebpf/cmd/bpf2go@latest
	go mod download
