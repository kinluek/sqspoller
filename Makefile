PWD := ${CURDIR}

test:
	CGO_ENABLED=0 go test -v ./...

# provide a docker network name for this command eg: make NETWORK=test dockertest
dockertest:
ifndef NETWORK
	@echo Warning: Please provide NETWORK argument.
	@echo example: make NETWORK=example dockertest
else
	# create docker network
	docker network create ${NETWORK}

	# run code in docker container, and provide network name
	# for sibling containers to be connected to.
	docker container run \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-v $(PWD):/go/code \
	-e DOCKER_NETWORK=${NETWORK} \
	--network ${NETWORK} \
	kinluek/go-docker:entry

	# run code in docker container, provide the NETWORK ENV
	docker network rm ${NETWORK}
endif

localstack:
#	docker container run -d -p 4567-4584:4567-4584 -e SERVICES=sqs -e DEBUG=1 -e DATA_DIR=/tmp/localstack/data -e DOCKER_HOST=unix:///var/run/docker.sock -v $TMPDIR:/tmp/localstack localstack/localstack