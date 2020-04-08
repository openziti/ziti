package main

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-cmd/ziti-probe/subcmd"
	"github.com/netfoundry/ziti-foundation/util/debugz"
	"github.com/sirupsen/logrus"
)

func init() {
	pfxlog.Global(logrus.InfoLevel)
	pfxlog.SetPrefix("github.com/netfoundry/")
}

func main() {
	debugz.AddStackDumpHandler()
	subcmd.Execute()
}
