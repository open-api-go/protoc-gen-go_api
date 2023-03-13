package goapi

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/open-api-go/protoc-gen-go_api/pbinfo"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
)

var httpPatternVarRegex = regexp.MustCompile(`{([a-zA-Z0-9_.]+?)(=[^{}]+)?}`)

var (
	descInfo pbinfo.Info
)

const (
	emptyValue = "google.protobuf.Empty"
	// protoc puts a dot in front of name, signaling that the name is fully qualified.
	emptyType               = "." + emptyValue
	lroType                 = ".google.longrunning.Operation"
	httpBodyType            = ".google.api.HttpBody"
	alpha                   = "alpha"
	beta                    = "beta"
	disableDeadlinesVar     = "GOOGLE_API_GO_EXPERIMENTAL_DISABLE_DEFAULT_DEADLINE"
	fieldTypeBool           = descriptor.FieldDescriptorProto_TYPE_BOOL
	fieldTypeString         = descriptor.FieldDescriptorProto_TYPE_STRING
	fieldTypeBytes          = descriptor.FieldDescriptorProto_TYPE_BYTES
	fieldTypeMessage        = descriptor.FieldDescriptorProto_TYPE_MESSAGE
	fieldLabelRepeated      = descriptor.FieldDescriptorProto_LABEL_REPEATED
	defaultPollInitialDelay = "time.Second" // 1 second
	defaultPollMaxDelay     = "time.Minute" // 1 minute

	bodyJSON  = "json"
	bodyFORM  = "form"
	bodyMULTI = "multi"
)

var wellKnownTypes = []string{
	".google.protobuf.FieldMask",
	".google.protobuf.Timestamp",
	".google.protobuf.Duration",
	".google.protobuf.DoubleValue",
	".google.protobuf.FloatValue",
	".google.protobuf.Int64Value",
	".google.protobuf.UInt64Value",
	".google.protobuf.Int32Value",
	".google.protobuf.UInt32Value",
	".google.protobuf.BoolValue",
	".google.protobuf.StringValue",
	".google.protobuf.BytesValue",
	".google.protobuf.Value",
	".google.protobuf.ListValue",
}

type httpInfo struct {
	verb, url, body, format string
}

func initRest(req *plugin.CodeGeneratorRequest) {
	descInfo = pbinfo.Of(req.GetProtoFile())
}

func genRestMethodCode(fd *descriptor.FileDescriptorProto, serv *descriptor.ServiceDescriptorProto, meth *descriptor.MethodDescriptorProto) (string, error) {
	code := strings.Builder{}

	httpInfo := getHTTPInfo(meth)
	// 处理path和params里面带有{xxx}的字段。但是gin的路由是:xxx形式，到时候可能需要转一下才行
	ps := baseURL(httpInfo)
	fmtStr := httpInfo.url
	// TODO(noahdietz): handle more complex path urls involving = and *,
	// e.g. v1beta1/repeat/{info.f_string=first/*}/{info.f_child.f_string=second/**}:pathtrailingresource
	fmtStr = httpPatternVarRegex.ReplaceAllStringFunc(fmtStr, func(s string) string { return "%v" })
	rawURL := fmt.Sprintf("%s%s", "%s", fmtStr)
	if len(ps) > 0 {
		code.WriteString(fmt.Sprintf("rawURL := fmt.Sprintf(%q, c.addr, %s)\n", rawURL, strings.Join(ps, ",")))
	} else {
		code.WriteString(fmt.Sprintf("rawURL := fmt.Sprintf(%q, c.addr)\n", rawURL))
	}

	// 还有一些，没有写在uri里面的，从结构体里面解析
	query := queryString(meth)
	if len(query) > 0 {
		param, err := getQueryStringContent(strings.Join(query, "\n\t"))
		if err != nil {
			return "", err
		}
		code.WriteString(param)
	}
	// 处理body
	body := "nil"
	format := bodyJSON
	verb := strings.ToUpper(httpInfo.verb)
	if httpInfo.body != "" {
		format = httpInfo.format
		if verb == http.MethodGet || verb == http.MethodDelete {
			return "", fmt.Errorf("invalid use of body parameter for a get/delete method %q", meth.GetName())
		}
		body = "in"
		if httpInfo.body != "*" {
			body = fmt.Sprintf("in%s", fieldGetter(httpInfo.body))
		}
	}
	if body != "nil" {
		switch format {
		case bodyFORM:
			forms := bodyForm(meth, httpInfo)
			if len(forms) > 0 {
				form, err := getBodyFormContent(strings.Join(forms, "\n\t"))
				if err != nil {
					return "", err
				}
				code.WriteString(form)
			}
		case bodyMULTI:
			forms := bodyForm(meth, httpInfo)
			if len(forms) > 0 {
				form, err := getMultipartContent(strings.Join(forms, "\n\t"))
				if err != nil {
					return "", err
				}
				code.WriteString(form)
			}
		default:
			code.WriteString(fmt.Sprintf("\topts = append(opts, grequests.JSON(%s))\n", body))
		}
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
	info := httpInfo{}
	body := httpRule.GetBody()
	if len(body) == 0 {
		info.body = ""
	}
	bs := strings.Split(body, ",")
	if len(bs) == 1 {
		info.body = body
		info.format = "json"
	} else {
		info.body = bs[0]
		info.format = bs[1]
	}

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

func baseURL(info *httpInfo) []string {
	tokens := []string{}
	// Can't just reuse pathParams because the order matters
	for _, path := range httpPatternVarRegex.FindAllStringSubmatch(info.url, -1) {
		// In the returned slice, the zeroth element is the full regex match,
		// and the subsequent elements are the sub group matches.
		// See the docs for FindStringSubmatch for further details.
		tokens = append(tokens, fmt.Sprintf("in%s", fieldGetter(path[1])))
	}
	return tokens
}

func bodyForm(m *descriptor.MethodDescriptorProto, info *httpInfo) []string {

	queryParams := map[string]*descriptor.FieldDescriptorProto{}
	request := descInfo.Type[m.GetInputType()].(*descriptor.DescriptorProto)
	if info.body != "*" {
		bodyField := lookupField(m.GetInputType(), info.body)
		request = descInfo.Type[bodyField.GetTypeName()].(*descriptor.DescriptorProto)
	}

	// Possible query parameters are all leaf fields in the request or body.
	pathToLeaf := getLeafs(request, nil)
	// Iterate in sorted order to
	for path, leaf := range pathToLeaf {
		// If, and only if, a leaf field is not a path parameter or a body parameter,
		// it is a query parameter.
		if lookupField(request.GetName(), leaf.GetName()) == nil {
			queryParams[path] = leaf
		}
	}

	return formParams("bodyForms", queryParams)
}

func queryString(m *descriptor.MethodDescriptorProto) []string {
	queryParams := queryParams(m)
	return formParams("params", queryParams)
}

func formParams(keyName string, queryParams map[string]*descriptor.FieldDescriptorProto) []string {
	// We want to iterate over fields in a deterministic order
	// to prevent spurious deltas when regenerating gapics.
	fields := make([]string, 0, len(queryParams))
	for p := range queryParams {
		fields = append(fields, p)
	}
	sort.Strings(fields)
	params := make([]string, 0, len(fields))

	for _, path := range fields {
		field := queryParams[path]
		required := isRequired(field)
		accessor := fieldGetter(path)
		singularPrimitive := field.GetType() != fieldTypeMessage &&
			field.GetType() != fieldTypeBytes &&
			field.GetLabel() != fieldLabelRepeated
		// key用命名的
		key := path

		var paramAdd string
		// Handle well known protobuf types with special JSON encodings.
		if strContains(wellKnownTypes, field.GetTypeName()) {
			b := strings.Builder{}
			b.WriteString(fmt.Sprintf("%s, err := json.Marshal(in%s)\n", field.GetJsonName(), accessor))
			b.WriteString("if err != nil {\n")
			b.WriteString("  return nil, err\n")
			b.WriteString("}\n")
			b.WriteString(fmt.Sprintf("%s[%q] = string(%s)", keyName, key, field.GetJsonName()))
			paramAdd = b.String()
		} else {
			paramAdd = fmt.Sprintf("%s[%q] = fmt.Sprintf(%q, in%s)", keyName, key, "%v", accessor)
		}

		// Only required, singular, primitive field types should be added regardless.
		if required && singularPrimitive {
			// Use string format specifier here in order to allow %v to be a raw string.
			params = append(params, paramAdd)
			continue
		}

		if field.GetLabel() == fieldLabelRepeated {
			// It's a slice, so check for len > 0, nil slice returns 0.
			params = append(params, fmt.Sprintf("if items := in%s; len(items) > 0 {", accessor))
			b := strings.Builder{}
			b.WriteString("for _, item := range items {\n")
			b.WriteString(fmt.Sprintf("  %s[%q] = fmt.Sprintf(%q, item)\n", keyName, key, "%v"))
			b.WriteString("}")
			paramAdd = b.String()

		} else if field.GetProto3Optional() {
			// Split right before the raw access
			toks := strings.Split(path, ".")
			toks = toks[:len(toks)-1]
			parentField := fieldGetter(strings.Join(toks, "."))
			directLeafField := directAccess(path)
			params = append(params, fmt.Sprintf("if in%s != nil && in%s != nil {", parentField, directLeafField))
		} else {
			// Default values are type specific
			switch field.GetType() {
			// Degenerate case, field should never be a message because that implies it's not a leaf.
			case fieldTypeMessage, fieldTypeBytes:
				params = append(params, fmt.Sprintf("if in%s != nil {", accessor))
			case fieldTypeString:
				params = append(params, fmt.Sprintf(`if in%s != "" {`, accessor))
			case fieldTypeBool:
				params = append(params, fmt.Sprintf(`if in%s {`, accessor))
			default: // Handles all numeric types including enums
				params = append(params, fmt.Sprintf(`if in%s != 0 {`, accessor))
			}
		}
		params = append(params, fmt.Sprintf("\t%s", paramAdd))
		params = append(params, "}")
	}

	return params
}

func queryParams(m *descriptor.MethodDescriptorProto) map[string]*descriptor.FieldDescriptorProto {
	queryParams := map[string]*descriptor.FieldDescriptorProto{}
	info := getHTTPInfo(m)
	if info == nil {
		return queryParams
	}
	if info.body == "*" {
		// The entire request is the REST body.
		return queryParams
	}

	pathParams := pathParams(m)
	// Minor hack: we want to make sure that the body parameter is NOT a query parameter.
	pathParams[info.body] = &descriptor.FieldDescriptorProto{}

	request := descInfo.Type[m.GetInputType()].(*descriptor.DescriptorProto)
	// Body parameters are fields present in the request body.
	// This may be the request message itself or a subfield.
	// Body parameters are not valid query parameters,
	// because that means the same param would be sent more than once.
	bodyField := lookupField(m.GetInputType(), info.body)

	// Possible query parameters are all leaf fields in the request or body.
	pathToLeaf := getLeafs(request, bodyField)
	// Iterate in sorted order to
	for path, leaf := range pathToLeaf {
		// If, and only if, a leaf field is not a path parameter or a body parameter,
		// it is a query parameter.
		if _, ok := pathParams[path]; !ok && lookupField(request.GetName(), leaf.GetName()) == nil {
			queryParams[path] = leaf
		}
	}

	return queryParams
}

// Returns a map from fully qualified path to field descriptor for all the leaf fields of a message 'm',
// where a "leaf" field is a non-message whose top message ancestor is 'm'.
// e.g. for a message like the following
//
//	message Mollusc {
//	    message Squid {
//	        message Mantle {
//	            int32 mass_kg = 1;
//	        }
//	        Mantle mantle = 1;
//	    }
//	    Squid squid = 1;
//	}
//
// The one entry would be
// "squid.mantle.mass_kg": *descriptor.FieldDescriptorProto...
func getLeafs(msg *descriptor.DescriptorProto, excludedFields ...*descriptor.FieldDescriptorProto) map[string]*descriptor.FieldDescriptorProto {
	pathsToLeafs := map[string]*descriptor.FieldDescriptorProto{}

	contains := func(fields []*descriptor.FieldDescriptorProto, field *descriptor.FieldDescriptorProto) bool {
		for _, f := range fields {
			if field == f {
				return true
			}
		}
		return false
	}

	// We need to declare and define this function in two steps
	// so that we can use it recursively.
	var recurse func([]*descriptor.FieldDescriptorProto, *descriptor.DescriptorProto)

	handleLeaf := func(field *descriptor.FieldDescriptorProto, stack []*descriptor.FieldDescriptorProto) {
		elts := []string{}
		for _, f := range stack {
			elts = append(elts, f.GetName())
		}
		elts = append(elts, field.GetName())
		key := strings.Join(elts, ".")
		pathsToLeafs[key] = field
	}

	handleMsg := func(field *descriptor.FieldDescriptorProto, stack []*descriptor.FieldDescriptorProto) {
		if field.GetLabel() == descriptor.FieldDescriptorProto_LABEL_REPEATED {
			// Repeated message fields must not be mapped because no
			// client library can support such complicated mappings.
			// https://cloud.google.com/endpoints/docs/grpc-service-config/reference/rpc/google.api#grpc-transcoding
			return
		}
		if contains(excludedFields, field) {
			return
		}
		// Short circuit on infinite recursion
		if contains(stack, field) {
			return
		}

		subMsg := descInfo.Type[field.GetTypeName()].(*descriptor.DescriptorProto)
		recurse(append(stack, field), subMsg)
	}

	recurse = func(
		stack []*descriptor.FieldDescriptorProto,
		m *descriptor.DescriptorProto,
	) {
		for _, field := range m.GetField() {
			if field.GetType() == fieldTypeMessage && !strContains(wellKnownTypes, field.GetTypeName()) {
				handleMsg(field, stack)
			} else {
				handleLeaf(field, stack)
			}
		}
	}

	recurse([]*descriptor.FieldDescriptorProto{}, msg)
	return pathsToLeafs
}

func pathParams(m *descriptor.MethodDescriptorProto) map[string]*descriptor.FieldDescriptorProto {
	pathParams := map[string]*descriptor.FieldDescriptorProto{}
	info := getHTTPInfo(m)
	if info == nil {
		return pathParams
	}

	// Match using the curly braces but don't include them in the grouping.
	re := regexp.MustCompile("{([^}]+)}")
	for _, p := range re.FindAllStringSubmatch(info.url, -1) {
		// In the returned slice, the zeroth element is the full regex match,
		// and the subsequent elements are the sub group matches.
		// See the docs for FindStringSubmatch for further details.
		param := strings.Split(p[1], "=")[0]
		field := lookupField(m.GetInputType(), param)
		if field == nil {
			continue
		}
		pathParams[param] = field
	}

	return pathParams
}

func lookupField(msgName, field string) *descriptor.FieldDescriptorProto {
	var desc *descriptor.FieldDescriptorProto
	msg := descInfo.Type[msgName]

	// If the message doesn't exist, fail cleanly.
	if msg == nil {
		return desc
	}

	msgProto := msg.(*descriptor.DescriptorProto)
	msgFields := msgProto.GetField()

	// Split the key name for nested fields, and traverse the message chain.
	for _, seg := range strings.Split(field, ".") {
		// Look up the desired field by name, stopping if the leaf field is
		// found, continuing if the field is a nested message.
		for _, f := range msgFields {
			if f.GetName() == seg {
				desc = f

				// Search the nested message for the next segment of the
				// nested field chain.
				if f.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
					msg = descInfo.Type[f.GetTypeName()]
					msgProto = msg.(*descriptor.DescriptorProto)
					msgFields = msgProto.GetField()
				}
				break
			}
		}
	}
	return desc
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

// isRequired returns if a field is annotated as REQUIRED or not.
func isRequired(field *descriptor.FieldDescriptorProto) bool {
	if field.GetOptions() == nil {
		return false
	}

	eBehav := proto.GetExtension(field.GetOptions(), annotations.E_FieldBehavior)

	behaviors := eBehav.([]annotations.FieldBehavior)
	for _, b := range behaviors {
		if b == annotations.FieldBehavior_REQUIRED {
			return true
		}
	}

	return false
}

// Given a chained description for a field in a proto message,
// e.g. squid.mantle.mass_kg
// return the string description of the go expression
// describing direct access to the terminal field
// i.e. .GetSquid().GetMantle().MassKg
//
// This is used for determining field presence for terminal optional fields.
func directAccess(field string) string {
	return buildAccessor(field, true)
}
