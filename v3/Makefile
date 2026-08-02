.PHONY: format test vet build check compose-config stage8-static

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

stage8-static:
	bash -n ops/*.sh
	SAKURA_V3_COLOR=blue SAKURA_V3_COLOR_API_PORT=18080 SAKURA_V3_COLOR_WEB_PORT=18088 \
	SAKURA_V3_IMAGE=example/app:3 SAKURA_V3_WEB_IMAGE=example/web:3 \
	SAKURA_V3_DATABASE_URL=postgres://sakura:test@postgres:5432/sakura SAKURA_V3_REDIS_PASSWORD=test \
	SAKURA_V3_CREDENTIAL_MASTER_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
	SAKURA_V3_INTERNAL_BOT_TOKEN=test-token docker compose -f deploy/compose.color.yaml config --quiet
