package edge

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"github.com/pkg/errors"
)

func GetEntityId(m *model.Model, entityType string, name string) (string, error) {
	output, err := cli.Exec(m, "edge", "list", entityType, "--output-json",
		fmt.Sprintf(`name="%v" limit none`, name))
	if err != nil {
		return "", err
	}

	l, err := gabs.ParseJSON([]byte(output))
	if err != nil {
		return "", err
	}

	data := l.Path("data")
	if data == nil {
		return "", nil
	}

	entities, err := data.Children()
	if err != nil {
		return "", err
	}

	for _, entity := range entities {
		return entity.S("id").Data().(string), nil
	}

	return "", errors.Errorf("no entity of type %v found with name %v", entityType, name)
}
