package build

var defaultBuildInfo = defaultInfo{}
var info Info = defaultBuildInfo

func GetBuildInfo() Info {
	return info
}

func InitBuildInfo(buildInfo Info) {
	if info == defaultBuildInfo {
		info = buildInfo
	}
}

type Info interface {
	Version() string
	Revision() string
	BuildDate() string
	Branch() string
}

type defaultInfo struct{}

func (d defaultInfo) Version() string {
	return "unknown"
}

func (d defaultInfo) Revision() string {
	return "unknown"
}

func (d defaultInfo) BuildDate() string {
	return "unknown"
}

func (d defaultInfo) Branch() string {
	return "unknown"
}
