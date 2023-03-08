package goapi

var (
	fn = map[string]interface{}{}
)

type FileData struct {
	Version   string         // 版本号
	Source    string         // 源文件
	GoPackage string         // Go报名
	Services  []*ServiceData // 服务数据
}

type ServiceData struct {
	ServName string        // 服务名，不带Service的
	Methods  []*MethodData // 方法数据
}

type MethodData struct {
	ServName string // 所属服务名
	MethName string // 方法名
	Comment  string // 注释。只取头注释
	ReqTyp   string // 请求类型名
	ResTyp   string // 返回类型名
}
