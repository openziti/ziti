package edge

import (
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/openziti/fablab/kernel/model"
	zitilibActions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/pkg/errors"
)

func SyncModelRouterIds(routerSpec string) model.Action {
	return &syncModelEdgeStateAction{
		routerSpec: routerSpec,
	}
}

func (action *syncModelEdgeStateAction) Execute(run model.Run) error {
	routerComponents := run.GetModel().SelectComponents(action.routerSpec)
	if len(routerComponents) == 0 {
		return errors.Errorf("no router components found for selector '%v'", action.routerSpec)
	}

	output, err := zitilibActions.EdgeExecWithOutput(run.GetModel(), "list", "edge-routers", "--output-json", "true limit none")
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
			if c.Id == routerName {
				routerId = strings.ReplaceAll(routerId, ".", ":")
				c.Tags = append(c.Tags, "edgeId:"+routerId)
			}
		}
	}

	return nil
}

type syncModelEdgeStateAction struct {
	routerSpec string
}

func SyncModelControllerIds(ctrlSpec string) model.Action {
	return model.ActionFunc(func(run model.Run) error {
		return run.GetModel().ForEachComponent(ctrlSpec, 1, func(c *model.Component) error {
			c.Tags = append(c.Tags, "edgeId:"+c.Id)
			return nil
		})
	})
}
