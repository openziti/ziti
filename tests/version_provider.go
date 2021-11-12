package tests

import (
	"github.com/openziti/foundation/common"
	"runtime"
	"time"
)

type VersionProviderTest struct {
}

func (v VersionProviderTest) Branch() string {
	return "local"
}

func (v VersionProviderTest) EncoderDecoder() common.VersionEncDec {
	return &common.StdVersionEncDec
}

func (v VersionProviderTest) Version() string {
	return "v0.0.0"
}

func (v VersionProviderTest) BuildDate() string {
	return time.Now().String()
}

func (v VersionProviderTest) Revision() string {
	return ""
}

func (v VersionProviderTest) AsVersionInfo() *common.VersionInfo {
	return &common.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() common.VersionProvider {
	return &VersionProviderTest{}
}
