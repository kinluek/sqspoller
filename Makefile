# Runs all the unit and integration tests using docker-compose.
# Make sure docker is installed and running.
test:
	docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
	docker-compose -f docker-compose.test.yml down --volumes

