.PHONY: build test clean docker

VERSION := 0.1.0
BINARY  := letchat
GOFLAGS := -ldflags="-s -w"

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/letchat/

test:
	go test -v -timeout 60s ./tests/

test-short:
	go test -v -short -timeout 30s ./tests/

clean:
	rm -f $(BINARY)

docker:
	docker build -t letchat:$(VERSION) .

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

install: build
	cp $(BINARY) /usr/local/bin/

init: build
	./$(BINARY) init
