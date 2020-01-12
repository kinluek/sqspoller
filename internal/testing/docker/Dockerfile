# go image with docker installed to run tests.
# bind docker sockets on start up, to connect host docker to container docker.
FROM golang:1.13-alpine

MAINTAINER Si Chan <github.com/kinluek>

WORKDIR /go/code

RUN apk add docker
RUN apk update
RUN apk add openrc --no-cache
RUN rc-update add docker boot

ENTRYPOINT CGO_ENABLED=0 go test -v ./...