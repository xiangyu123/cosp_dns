all: build_all

build_all:
	rm -rf ./build
	mkdir -p ./build
	#env GOOS=linux GOARCH=amd64    go build -o ./build/blibee-dnsproxy-go-linux-amd64
	#env GOOS=linux GOARCH=arm64    go build -o ./build/blibee-dnsproxy-go-linux-arm64
	#env GOOS=linux GOARCH=arm      go build -o ./build/blibee-dnsproxy-go-linux-arm
	#env GOOS=linux GOARCH=mips     go build -o ./build/blibee-dnsproxy-go-linux-mips
	#env GOOS=linux GOARCH=mipsle   go build -o ./build/blibee-dnsproxy-go-linux-mipsle
	#env GOOS=linux GOARCH=mips64   go build -o ./build/blibee-dnsproxy-go-linux-mips64
	#env GOOS=linux GOARCH=mips64le go build -o ./build/blibee-dnsproxy-go-linux-mips64le
	env GOOS=darwin GOARCH=amd64   go build -o ./build/blibee-dnsproxy-go-macos-amd64
	#env GOOS=darwin GOARCH=arm64   go build -o ./build/blibee-dnsproxy-go-macos-arm64

test:
	go test ./freedns

.PHONY: build_all test
