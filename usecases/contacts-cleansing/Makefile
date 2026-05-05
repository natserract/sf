docker-up:
	docker compose -f docker-compose.yml up -d

docker-reset:
	docker compose -f docker-compose.yml down -v

test:
	go test ./...

.PHONY: build-validate build-validate-windows release-validate-all

build-validate:
	mkdir -p dist
	go build -o dist/smfc-validate ./cmd/smfc-validate

build-validate-windows:
	mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -o dist/smfc-validate-windows-amd64.exe ./cmd/smfc-validate

release-validate-all:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -o dist/smfc-validate-darwin-arm64 ./cmd/smfc-validate
	GOOS=darwin GOARCH=amd64 go build -o dist/smfc-validate-darwin-amd64 ./cmd/smfc-validate
	GOOS=windows GOARCH=amd64 go build -o dist/smfc-validate-windows-amd64.exe ./cmd/smfc-validate
	GOOS=windows GOARCH=arm64 go build -o dist/smfc-validate-windows-arm64.exe ./cmd/smfc-validate