.PHONY: build test clean docker

VERSION := 1.0.0-beta.1
BINARY  := clawnet
GOFLAGS := -ldflags="-s -w" -tags fts5

build:
	CGO_ENABLED=1 go build $(GOFLAGS) -o $(BINARY) ./cmd/clawnet/

build-dev:
	CGO_ENABLED=1 go build -ldflags="-s -w" -tags fts5,dev -o $(BINARY)-dev ./cmd/clawnet/

build-seedbot:
	CGO_ENABLED=1 go build $(GOFLAGS) -o seedbot ./cmd/seedbot/

test:
	CGO_ENABLED=1 go test -v -tags fts5 -timeout 60s ./tests/

test-short:
	CGO_ENABLED=1 go test -v -tags fts5 -short -timeout 30s ./tests/

clean:
	rm -f $(BINARY)

docker:
	docker build -t clawnet:$(VERSION) .

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
