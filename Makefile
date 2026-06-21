build:
	go build -ldflags "-X main.version=${shell git describe --tags 2>/dev/null}" -o ./bin/wheeler ./cmd/main.go