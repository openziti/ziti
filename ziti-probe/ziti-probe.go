//go:build all

package main

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/ziti/ziti-probe/subcmd"
	"github.com/sirupsen/logrus"
)

func init() {
	pfxlog.GlobalInit(logrus.InfoLevel, pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor())
}

func main() {
	debugz.AddStackDumpHandler()
	subcmd.Execute()
}
