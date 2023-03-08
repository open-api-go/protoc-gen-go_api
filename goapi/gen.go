package goapi

import plugin "github.com/golang/protobuf/protoc-gen-go/plugin"

func Gen(req *plugin.CodeGeneratorRequest) (*plugin.CodeGeneratorResponse, error) {
	opts, err := parseOptions(req.Parameter)
	if err != nil {
		return nil, err
	}
	_ = opts

	return nil, nil
}
