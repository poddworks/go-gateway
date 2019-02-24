#!/bin/bash

go-shell() {
  project=`basename $(pwd)`
  docker run -it --rm -v ${local_dir}:/go/src/github.com/poddworks/${project} -w /go/src/github.com/poddworks/${project} golang:1-devtools bash
}

make() {
  project=`basename $(pwd)`
  docker run -it --rm -v ${local_dir}:/go/src/github.com/poddworks/${project} -w /go/src/github.com/poddworks/${project} golang:1-devtools make ${@}
}

dep() {
  project=`basename $(pwd)`
  docker run -it --rm -v ${local_dir}:/go/src/github.com/poddworks/${project} -w /go/src/github.com/poddworks/${project} golang:1-devtools dep ${@}
}

go() {
  project=`basename $(pwd)`
  docker run -it --rm -v ${local_dir}:/go/src/github.com/poddworks/${project} -w /go/src/github.com/poddworks/${project} golang:1-devtools go ${@}
}

case $1 in
init)
    docker build -t golang:1-devtools . -f-<<EOF
FROM golang:1

RUN curl -fsSL -o /usr/local/bin/confd https://github.com/kelseyhightower/confd/releases/download/v0.16.0/confd-0.16.0-linux-amd64 && \
  chmod +x /usr/local/bin/confd

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-amd64 && \
  chmod +x /usr/local/bin/dep
EOF
    ;;
*)
    export local_dir=${1:-$(pwd)}
    # Do nothing, loading as build tool
    ;;
esac
