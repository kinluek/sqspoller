PWD := ${CURDIR}

test:
	CGO_ENABLED=0 go test -v ./...

# provide a docker network name for this command eg: make NETWORK=test dockertest
dockertest:
	docker container run \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-v $(PWD):/go/code \
	-e DOCKER_NETWORK=${NETWORK} \
	--network ${NETWORK} \
	kinluek/go-docker:entry

localstack:
#	docker container run -d -p 4567-4584:4567-4584 -e SERVICES=sqs -e DEBUG=1 -e DATA_DIR=/tmp/localstack/data -e DOCKER_HOST=unix:///var/run/docker.sock -v $TMPDIR:/tmp/localstack localstack/localstack