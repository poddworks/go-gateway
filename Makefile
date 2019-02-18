.PHONY: all clean config linux linux-native

all: linux

clean:
	rm -f go-gateway go-gateway-Linux-*

GIT_HASH := $(shell git rev-parse --short HEAD)
GO_VER := $(shell go version | cut -d ' ' -f 3)
VER := $(shell echo ${GATEWAY_API_VER})
config:
	env GATEWAY_API_VER=$(VER) GATEWAY_API_GIT_HASH=$(GIT_HASH) GATEWAY_API_GO_VER=$(GO_VER) confd -log-level error -backend env -confdir ./confd/ -onetime

linux-native: config
	go build -o go-gateway ./cmd

linux: config
	env CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o go-gateway-Linux-x86_64 ./cmd
