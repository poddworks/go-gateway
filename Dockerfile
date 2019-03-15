FROM alpine
MAINTAINER YI-HUNG JEN <yihungjen@gmail.com>

RUN apk --no-cache add ca-certificates

COPY go-gateway-linux-x86_64 /go-gateway

EXPOSE 8080

ENTRYPOINT ["/go-gateway"]
CMD ["--help"]
