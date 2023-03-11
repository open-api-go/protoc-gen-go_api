package goapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
)

var httpPatternVarRegex = regexp.MustCompile(`{([a-zA-Z0-9_.]+?)(=[^{}]+)?}`)

type httpInfo struct {
	verb, url, body string
}

func initRest(req *plugin.CodeGeneratorRequest) {
}

func genRestMethodCode(fd *descriptor.FileDescriptorProto, serv *descriptor.ServiceDescriptorProto, meth *descriptor.MethodDescriptorProto) (string, error) {
	code := strings.Builder{}

	httpInfo := getHTTPInfo(meth)
	ps := baseURL(httpInfo)
	code.WriteString(fmt.Sprintf("rawURL := c.addr + fmt.Sprintf(%s)\n", ps))
	body := "nil"
	verb := strings.ToUpper(httpInfo.verb)
	if httpInfo.body != "" {
		if verb == http.MethodGet || verb == http.MethodDelete {
			return "", fmt.Errorf("invalid use of body parameter for a get/delete method %q", meth.GetName())
		}
		body = "in"
		if httpInfo.body != "*" {
			body = fmt.Sprintf("in%s", fieldGetter(httpInfo.body))
		}
	}
	if body != "nil" {
		code.WriteString(fmt.Sprintf("\topts = append(opts, grequests.JSON(%s))\n", body))
	}
	code.WriteString(fmt.Sprintf("\treturn c.session.%s(rawURL,opts...)", upperFirst(httpInfo.verb)))
	return code.String(), nil
}

func getHTTPInfo(m *descriptor.MethodDescriptorProto) *httpInfo {
	if m == nil || m.GetOptions() == nil {
		return nil
	}

	eHTTP := proto.GetExtension(m.GetOptions(), annotations.E_Http)

	httpRule := eHTTP.(*annotations.HttpRule)
	info := httpInfo{body: httpRule.GetBody()}

	switch httpRule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		info.verb = "get"
		info.url = httpRule.GetGet()
	case *annotations.HttpRule_Post:
		info.verb = "post"
		info.url = httpRule.GetPost()
	case *annotations.HttpRule_Patch:
		info.verb = "patch"
		info.url = httpRule.GetPatch()
	case *annotations.HttpRule_Put:
		info.verb = "put"
		info.url = httpRule.GetPut()
	case *annotations.HttpRule_Delete:
		info.verb = "delete"
		info.url = httpRule.GetDelete()
	}

	return &info
}

func baseURL(info *httpInfo) string {
	fmtStr := info.url
	// TODO(noahdietz): handle more complex path urls involving = and *,
	// e.g. v1beta1/repeat/{info.f_string=first/*}/{info.f_child.f_string=second/**}:pathtrailingresource
	fmtStr = httpPatternVarRegex.ReplaceAllStringFunc(fmtStr, func(s string) string { return "%v" })

	tokens := []string{fmt.Sprintf("%q", fmtStr)}
	// Can't just reuse pathParams because the order matters
	for _, path := range httpPatternVarRegex.FindAllStringSubmatch(info.url, -1) {
		// In the returned slice, the zeroth element is the full regex match,
		// and the subsequent elements are the sub group matches.
		// See the docs for FindStringSubmatch for further details.
		tokens = append(tokens, fmt.Sprintf("in%s", fieldGetter(path[1])))
	}
	return strings.Join(tokens, ",")
}

// Given a chained description for a field in a proto message,
// e.g. squid.mantle.mass_kg
// return the string description of the go expression
// describing idiomatic access to the terminal field
// i.e. .GetSquid().GetMantle().GetMassKg()
//
// This is the normal way to retrieve values.
func fieldGetter(field string) string {
	return buildAccessor(field, false)
}

func buildAccessor(field string, rawFinal bool) string {
	// Corner case if passed the result of strings.Join on an empty slice.
	if field == "" {
		return ""
	}

	var ax strings.Builder
	split := strings.Split(field, ".")
	idx := len(split)
	if rawFinal {
		idx--
	}
	for _, s := range split[:idx] {
		fmt.Fprintf(&ax, ".Get%s()", snakeToCamel(s))
	}
	if rawFinal {
		fmt.Fprintf(&ax, ".%s", snakeToCamel(split[len(split)-1]))
	}
	return ax.String()
}

// snakeToCamel converts snake_case and SNAKE_CASE to CamelCase.
func snakeToCamel(s string) string {
	var sb strings.Builder
	up := true
	for _, r := range s {
		if r == '_' {
			up = true
		} else if up && unicode.IsDigit(r) {
			sb.WriteRune('_')
			sb.WriteRune(r)
			up = false
		} else if up {
			sb.WriteRune(unicode.ToUpper(r))
			up = false
		} else {
			sb.WriteRune(unicode.ToLower(r))
		}
	}
	return sb.String()
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	r, w := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[w:]
}
