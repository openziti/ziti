package edge

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"path/filepath"
	"strings"
)

func InitEdgeRouters(componentSpec string, concurrency int) model.Action {
	return &initEdgeRoutersAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *initEdgeRoutersAction) Execute(m *model.Model) error {
	return m.ForEachComponent(action.componentSpec, action.concurrency, func(c *model.Component) error {
		if err := zitilib_actions.EdgeExec(m, "delete", "edge-router", c.PublicIdentity); err != nil {
			pfxlog.Logger().
				WithError(err).
				WithField("router", c.PublicIdentity).
				Warn("unable to delete router (may not be present")
		}

		return action.createAndEnrollRouter(c)
	})
}

func (action *initEdgeRoutersAction) createAndEnrollRouter(c *model.Component) error {
	ssh := lib.NewSshConfigFactory(c.GetHost())

	jwtFileName := filepath.Join(model.ConfigBuild(), c.PublicIdentity+".jwt")

	attributes := strings.Join(c.Tags, ",")

	args := []string{"create", "edge-router", c.PublicIdentity, "-j", "--jwt-output-file", jwtFileName, "-a", attributes}

	isTunneler := c.HasLocalOrAncestralTag("tunneler")
	if isTunneler {
		args = append(args, "--tunneler-enabled")
	}

	if c.HasLocalOrAncestralTag("no-traversal") {
		args = append(args, "--no-traversal")
	}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	if isTunneler {
		if err := zitilib_actions.EdgeExec(c.GetModel(), "update", "identity", c.PublicIdentity, "-a", attributes); err != nil {
			return err
		}
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

	if isTunneler {
		cmds = append(cmds,
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved")
	}

	return host.Exec(c.GetHost(), cmds...).Execute(c.GetModel())
}

type initEdgeRoutersAction struct {
	componentSpec string
	concurrency   int
}
