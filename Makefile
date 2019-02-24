.PHONY: all clean config linux-static linux darwin-static darwin

all: darwin linux

clean:
	rm -f go-gateway-*

GIT_HASH := $(shell git rev-parse --short HEAD)
GO_VER := $(shell go version | cut -d ' ' -f 3)
VER := $(shell echo ${GATEWAY_API_VER})
config:
	env GATEWAY_API_VER=$(VER) GATEWAY_API_GIT_HASH=$(GIT_HASH) GATEWAY_API_GO_VER=$(GO_VER) confd -log-level error -backend env -confdir ./confd/ -onetime

darwin-static: config
	env CGO_ENABLED=0 GOOS=darwin go build -a -installsuffix cgo -o go-gateway-darwin-x86_64 ./cmd

darwin: config
	env GOOS=darwin go build -o go-gateway-darwin ./cmd

linux-static: config
	env CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o go-gateway-linux-x86_64 ./cmd

linux: config
	env GOOS=linux go build -o go-gateway-linux ./cmd
