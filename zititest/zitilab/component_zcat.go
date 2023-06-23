package zitilab

import (
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"strings"
)

var _ model.ComponentType = (*ZCatType)(nil)

type ZCatMode int

type ZCatType struct {
	Version   string
	LocalPath string
}

func (self *ZCatType) Dump() any {
	return map[string]string{
		"type_id":    "zcat",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZCatType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZCatType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "ziti") && strings.Contains(s, "zcat ")
	}
}

func (self *ZCatType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	factory := lib.NewSshConfigFactory(c.GetHost())
	pids, err := lib.FindProcesses(factory, self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZCatType) Stop(_ model.Run, c *model.Component) error {
	factory := lib.NewSshConfigFactory(c.GetHost())
	return lib.RemoteKillFilterF(factory, self.getProcessFilter(c))
}
