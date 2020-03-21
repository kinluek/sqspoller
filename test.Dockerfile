FROM golang:1.14-alpine

WORKDIR /code

CMD CGO_ENABLED=0 go test -tags=integration -v -p 1