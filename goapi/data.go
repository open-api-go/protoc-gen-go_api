package goapi

import "strings"

var (
	fn = map[string]interface{}{
		"unexport": unexport,
		"html":     html,
	}
)

type FileData struct {
	Version   string         // 版本号
	Source    string         // 源文件
	GoPackage string         // Go包名
	Services  []*ServiceData // 服务数据
}

type ServiceData struct {
	PkgName  string        // package name
	ServName string        // 服务名，不带Service的
	Methods  []*MethodData // 方法数据
}

type MethodData struct {
	ServName string // 所属服务名
	MethName string // 方法名
	Comment  string // 注释。只取头注释
	ReqTyp   string // 请求类型名
	ResTyp   string // 返回类型名
	ReqCode  string // 请求代码
}

var (
	noClientStream = `return nil, fmt.Errorf("%s not yet supported for REST clients")`
	noServerStream = `return nil, fmt.Errorf("%s not yet supported for REST servers")`
)

// unexport 把首字母转小写
func unexport(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func html(s string) string {
	return s
}
