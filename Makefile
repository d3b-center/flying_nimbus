run:
	go run main.go
build:
	go clean -cache
	go clean -modcache
	go mod tidy
	go build
test:
	go test ./...