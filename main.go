package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/open-api-go/protoc-gen-go_api/goapi"
	"google.golang.org/protobuf/proto"

	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

var (
	showVersion = flag.Bool("version", false, "print the version and exit")
	omitempty   = flag.Bool("omitempty", true, "omit if google.api is empty")
)

func main() {
	reqBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	var genReq plugin.CodeGeneratorRequest
	if err := proto.Unmarshal(reqBytes, &genReq); err != nil {
		log.Fatal(err)
	}

	genResp, err := goapi.Gen(&genReq)
	if err != nil {
		genResp.Error = proto.String(err.Error())
	}

	genResp.SupportedFeatures = proto.Uint64(uint64(plugin.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL))

	outBytes, err := proto.Marshal(genResp)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(outBytes); err != nil {
		log.Fatal(err)
	}
}
