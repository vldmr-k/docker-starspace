IMAGE_NAME := recomendation
CONTAINER_NAME := starspace
HOST_PORT := 8900
CONTAINER_PORT := 8000
CPUS := 0.7

.PHONY: build run stop restart

build:
	docker build --target dev -t $(IMAGE_NAME) .

start: build
	docker run -d \
		--name $(CONTAINER_NAME) \
		--cpus="$(CPUS)" \
		-v "$(PWD)":/app \
		-v ./data:/data \
		-p $(HOST_PORT):$(CONTAINER_PORT) \
		$(IMAGE_NAME)

stop:
	docker stop $(CONTAINER_NAME) || true
	docker rm $(CONTAINER_NAME) || true

restart: stop start
