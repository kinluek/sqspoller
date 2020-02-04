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
	-v $(PWD):/sqspoller \
	-e DOCKER_NETWORK=${NETWORK} \
	-e ENVIRONMENT=CI \
	-e CGO_ENABLED=0 \
	--network ${NETWORK} \
	--entrypoint '/bin/sh' \
	golang:1.13-alpine \
    -c 'cd /sqspoller && go test -v -cover /sqspoller'

	docker network rm ${NETWORK}
endif

build_go_docker:
	docker build -t kinluek/go-docker:entry ./internal/testing/docker/

push_go_docker:
	docker push kinluek/go-docker:entry

update_go_docker: build_go_docker push_go_docker

localstack:
#	docker container run -d -p 4567-4584:4567-4584 -e SERVICES=sqs -e DEBUG=1 -e DATA_DIR=/tmp/localstack/data -e DOCKER_HOST=unix:///var/run/docker.sock -v $TMPDIR:/tmp/localstack localstack/localstack