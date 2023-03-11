srv = protoc-gen-go_api
build:
	go fmt ./...
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -trimpath -o ./bin/${srv}


test:
	go fmt ./...
	go install
	cd testdata && make build && cd ../