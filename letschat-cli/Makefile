.PHONY: build test clean docker

VERSION := 0.2.0
BINARY  := letchat
GOFLAGS := -ldflags="-s -w" -tags fts5

build:
	CGO_ENABLED=1 go build $(GOFLAGS) -o $(BINARY) ./cmd/letchat/

build-seedbot:
	CGO_ENABLED=1 go build $(GOFLAGS) -o seedbot ./cmd/seedbot/

test:
	CGO_ENABLED=1 go test -v -tags fts5 -timeout 60s ./tests/

test-short:
	CGO_ENABLED=1 go test -v -tags fts5 -short -timeout 30s ./tests/

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
