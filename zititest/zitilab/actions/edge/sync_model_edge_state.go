package edge

import (
	"github.com/Jeffail/gabs"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/pkg/errors"
	"strings"
)

func SyncModelEdgeState(componentSpec string) model.Action {
	return &syncModelEdgeStateAction{
		componentSpec: componentSpec,
	}
}

func (action *syncModelEdgeStateAction) Execute(m *model.Model) error {
	routerComponents := m.SelectComponents(action.componentSpec)
	if len(routerComponents) == 0 {
		return errors.Errorf("no router components found for selector '%v'", action.componentSpec)
	}

	output, err := zitilib_actions.EdgeExecWithOutput(m, "list", "edge-routers", "--output-json", "true limit none")
	if err != nil {
		return err
	}

	l, err := gabs.ParseJSON([]byte(output))
	if err != nil {
		return err
	}

	data := l.Path("data")
	if data == nil {
		return nil
	}

	routers, err := data.Children()
	if err != nil {
		return err
	}

	for _, router := range routers {
		routerId := router.S("id").Data().(string)
		routerName := router.S("name").Data().(string)

		for _, c := range routerComponents {
			if c.PublicIdentity == routerName {
				routerId = strings.ReplaceAll(routerId, ".", ":")
				c.Tags = append(c.Tags, "edgeId:"+routerId)
			}
		}
	}

	return nil
}

type syncModelEdgeStateAction struct {
	componentSpec string
}
