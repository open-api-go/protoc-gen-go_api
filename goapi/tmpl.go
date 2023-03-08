package goapi

import (
	"bytes"
	"html/template"
	"log"
)

var goapiTmpl = `// Code generated by protoc-gen-go_api(github.com/open-api-go/protoc-gen-go_api version={{ .Version }}). DO NOT EDIT.
// source: {{ .Source }}

package {{ .GoPackage }}

import (
	context "context"
	grequests "github.com/open-api-go/grequests"
)

{{ range .Services }}
// Client API for {{ .ServName }} service

type {{ .ServName }}Service interface {
{{ range .Methods }}
	// {{ .MethName }} {{ .Comment }}
	{{ .MethName }}(ctx context.Context, in *{{ .ReqTyp }}, opts ...grequests.RequestOption) (*grequest.Response, error)
{{ end }}
}

type {{ unexport .ServName }}Service struct {
	addr string // start with http/https
}

func New{{ .ServName }}Service(addr string) {{ .ServName }}Service {
	return &{{ unexport .ServName }}Service{
		addr: addr,
	}
}

{{ range .Methods }}
func (c *{{ unexport .ServName }}Service) {{ .MethName }}(ctx context.Context, in *{{ .ReqTyp }}, opts ...grequests.RequestOption) (*grequest.Response, error) {

}
{{ end }}

{{ end }}

`

func getGoapiContent(data *FileData) (string, error) {
	cm, err := template.New("goapi_tmpl").Funcs(fn).Parse(goapiTmpl)
	if err != nil {
		log.Println("parse goapi template err: ", err)
		return "", err
	}
	bs := new(bytes.Buffer)
	err = cm.Execute(bs, data)
	if err != nil {
		log.Println("execute goapi template err: ", err)
		return "", err
	}
	return bs.String(), nil
}
