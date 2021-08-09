package main

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/debugz"
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
