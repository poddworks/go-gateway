FROM scratch
MAINTAINER YI-HUNG JEN <yihungjen@gmail.com>

COPY ca-certificates.crt /etc/ssl/certs/
COPY gateway-api-Linux-x86_64 /gateway-api
ENTRYPOINT ["/gateway-api"]
CMD ["--help"]
