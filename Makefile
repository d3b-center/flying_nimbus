
BINARY_NAME := flying-nimbus

## Run
run:
	go run .

clean:
	go clean -cache
	go clean -modcache
	rm -f $(BINARY_NAME)

tidy:
	go mod tidy

## Build Binary
build:
	go build -o $(BINARY_NAME)
