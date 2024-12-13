package webapis

import (
	"fmt"
	"github.com/openziti/xweb/v2"
	log "github.com/sirupsen/logrus"
	"strings"
)

const (
	Binding = "zac"
)

type ZitiAdminConsoleFactory struct {
}

var _ xweb.ApiHandlerFactory = &ZitiAdminConsoleFactory{}

func NewZitiAdminConsoleFactory() *ZitiAdminConsoleFactory {
	return &ZitiAdminConsoleFactory{}
}

func (factory *ZitiAdminConsoleFactory) Validate(*xweb.InstanceConfig) error {
	return nil
}

func (factory *ZitiAdminConsoleFactory) Binding() string {
	return Binding
}

func (factory *ZitiAdminConsoleFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	locVal := options["location"]
	if locVal == nil || locVal == "" {
		return nil, fmt.Errorf("location must be supplied in the %s options", Binding)
	}

	loc, ok := locVal.(string)

	if !ok {
		return nil, fmt.Errorf("location must be a string for the %s options", Binding)
	}

	indexFileVal := options["indexFile"]
	indexFile := "index.html"

	if indexFileVal != nil {
		newFileVal, ok := indexFileVal.(string)

		if !ok {
			return nil, fmt.Errorf("indexFile must be a string for the %s options", Binding)
		}

		newFileVal = strings.TrimSpace(newFileVal)

		if newFileVal != "" {
			indexFile = newFileVal
		}
	}

	contextRoot := "/" + Binding
	spa := &GenericHttpHandler{
		HttpHandler: SpaHandler(loc, contextRoot, indexFile),
		BindingKey:  Binding,
		ContextRoot: contextRoot,
	}

	log.Infof("initializing ZAC SPA Handler from %s", locVal)
	return spa, nil
}
