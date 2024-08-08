package zac

import (
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/common/spa_handler"
	log "github.com/sirupsen/logrus"
)

const (
	Binding = "zac"
	DefaultLocation = "/ziti-console"
)

type ZitiAdminConsoleFactory struct {
}

var _ xweb.ApiHandlerFactory = &ZitiAdminConsoleFactory{}

func NewZitiAdminConsoleFactory() *ZitiAdminConsoleFactory {
	return &ZitiAdminConsoleFactory{}
}

func (factory ZitiAdminConsoleFactory) Validate(*xweb.InstanceConfig) error {
	return nil
}

func (factory ZitiAdminConsoleFactory) Binding() string {
	return Binding
}

func (factory ZitiAdminConsoleFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	loc := options["location"]
	if loc == nil || loc == "" {
		log.Fatal("location must be supplied in " + Binding + " options")
	}
	indexFile := options["indexFile"]
	if indexFile == nil || indexFile == "" {
		indexFile = "index.html"
	}
	spa := &spa_handler.SinglePageAppHandler{
		HttpHandler: spa_handler.SpaHandler(loc.(string), "/"+Binding, indexFile.(string)),
		BindingKey:  Binding,
	}

	log.Infof("initializing ZAC SPA Handler from %s", loc)
	return spa, nil
}
