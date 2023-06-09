package edge

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"path/filepath"
)

func ReEnrollEdgeRouters(componentSpec string, concurrency int) model.Action {
	return &reEnrollEdgeRoutersAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *reEnrollEdgeRoutersAction) Execute(m *model.Model) error {
	return m.ForEachComponent(action.componentSpec, action.concurrency, func(c *model.Component) error {
		return action.reEnrollRouter(c)
	})
}

func (action *reEnrollEdgeRoutersAction) reEnrollRouter(c *model.Component) error {
	ssh := lib.NewSshConfigFactory(c.GetHost())

	jwtFileName := filepath.Join(model.ConfigBuild(), c.PublicIdentity+".jwt")

	args := []string{"re-enroll", "edge-router", c.PublicIdentity, "-j", "--jwt-output-file", jwtFileName}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	remoteJwt := "/home/ubuntu/fablab/cfg/" + c.PublicIdentity + ".jwt"
	if err := lib.SendFile(ssh, jwtFileName, remoteJwt); err != nil {
		return err
	}

	tmpl := "set -o pipefail; /home/ubuntu/fablab/bin/%s enroll /home/ubuntu/fablab/cfg/%s -j %s 2>&1 | tee /home/ubuntu/logs/%s.router.enroll.log "
	cmds := []string{
		"mkdir -p /home/ubuntu/logs",
		fmt.Sprintf(tmpl, c.BinaryName, c.ConfigName, remoteJwt, c.ConfigName),
	}

	if c.HasLocalOrAncestralTag("tunneler") {
		cmds = append(cmds,
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved")
	}

	return host.Exec(c.GetHost(), cmds...).Execute(c.GetModel())
}

type reEnrollEdgeRoutersAction struct {
	componentSpec string
	concurrency   int
}
