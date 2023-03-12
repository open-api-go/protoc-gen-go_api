package goapi

import (
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"google.golang.org/protobuf/runtime/protoiface"
)

var (
	comments = make(map[protoiface.MessageV1]string)
)

func initComment(req *plugin.CodeGeneratorRequest) {
	for _, f := range req.GetProtoFile() {
		for _, loc := range f.GetSourceCodeInfo().GetLocation() {
			if loc.LeadingComments == nil {
				continue
			}

			// p is an array with format [f1, i1, f2, i2, ...]
			// - f1 refers to the protobuf field tag
			// - if field refer to by f1 is a slice, i1 refers to an element in that slice
			// - f2 and i2 works recursively.
			// So, [6, x] refers to the xth service defined in the file,
			// since the field tag of Service is 6.
			// [6, x, 2, y] refers to the yth method in that service,
			// since the field tag of Method is 2.
			p := loc.Path
			switch {
			case len(p) == 2 && p[0] == 6:
				comments[f.Service[p[1]]] = *loc.LeadingComments
			case len(p) == 4 && p[0] == 6 && p[2] == 2:
				comments[f.Service[p[1]].Method[p[3]]] = *loc.LeadingComments
			}
		}
	}
}

func getComment(m protoiface.MessageV1) string {
	c, ok := comments[m]
	if !ok {
		return ""
	}
	if c[len(c)-1] == '\n' {
		return c[:len(c)-1]
	}
	return c
}
