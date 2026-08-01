.PHONY: format test vet build check compose-config

format:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./...

check: test vet build

compose-config:
	docker compose --env-file .env.example config --quiet
