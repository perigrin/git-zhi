.PHONY: all build setup test clean cross-compile

all: build setup

build:
	go build -o git-zhi ./cmd/git-zhi/

setup: build
	./git-zhi setup

test:
	go test ./... -count=1

vet:
	go vet ./...

clean:
	rm -f git-zhi
	rm -f git-zhi-linux-amd64 git-zhi-linux-arm64 git-zhi-darwin-arm64 git-zhi-windows-amd64.exe
	go clean ./...

cross-compile:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o git-zhi-linux-amd64 ./cmd/git-zhi/
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o git-zhi-linux-arm64 ./cmd/git-zhi/
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o git-zhi-darwin-arm64 ./cmd/git-zhi/
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o git-zhi-windows-amd64.exe ./cmd/git-zhi/
