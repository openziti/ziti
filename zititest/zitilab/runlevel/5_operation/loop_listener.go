package zitilib_runlevel_5_operation

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
	"strings"
)

func Loop3Listener(host *model.Host, joiner chan struct{}, bindAddress string, extraArgs ...string) model.OperatingStage {
	return &loopListener{
		host:        host,
		joiner:      joiner,
		bindAddress: bindAddress,
		subcmd:      "loop3",
		extraArgs:   extraArgs,
	}
}

func LoopListener(host *model.Host, joiner chan struct{}, bindAddress string, extraArgs ...string) model.OperatingStage {
	return &loopListener{
		host:        host,
		joiner:      joiner,
		bindAddress: bindAddress,
		subcmd:      "loop2",
		extraArgs:   extraArgs,
	}
}

func (self *loopListener) Operate(run model.Run) error {
	ssh := lib.NewSshConfigFactory(self.host)
	if err := lib.RemoteKill(ssh, fmt.Sprintf("ziti-fabric-test %v listener", self.subcmd)); err != nil {
		return fmt.Errorf("error killing %v listeners (%w)", self.subcmd, err)
	}

	go self.run(run)
	return nil
}

func (self *loopListener) run(run model.Run) {
	defer func() {
		if self.joiner != nil {
			close(self.joiner)
			logrus.Debugf("closed joiner")
		}
	}()

	ssh := lib.NewSshConfigFactory(self.host)

	logFile := fmt.Sprintf("/home/%s/logs/%v-listener-%s.log", ssh.User(), self.subcmd, run.GetId())
	listenerCmd := fmt.Sprintf("/home/%s/fablab/bin/ziti-fabric-test %v listener -b %v %v >> %s 2>&1",
		ssh.User(), self.subcmd, self.bindAddress, strings.Join(self.extraArgs, " "), logFile)
	if output, err := lib.RemoteExec(ssh, listenerCmd); err != nil {
		logrus.Errorf("error starting loop listener [%s] (%v)", output, err)
	}
}

type loopListener struct {
	host        *model.Host
	joiner      chan struct{}
	bindAddress string
	subcmd      string
	extraArgs   []string
}
