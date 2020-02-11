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
	GetVersion() string
	GetRevision() string
	GetBuildDate() string
}

type defaultInfo struct{}

func (d defaultInfo) GetVersion() string {
	return "unknown"
}

func (d defaultInfo) GetRevision() string {
	return "unknown"
}

func (d defaultInfo) GetBuildDate() string {
	return "unknown"
}
