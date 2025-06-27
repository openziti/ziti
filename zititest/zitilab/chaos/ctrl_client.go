package chaos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/rest_client/inspect"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/zitirest"
	"gopkg.in/yaml.v3"
	"io"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

func NewCtrlClients(run model.Run, selector string) (*CtrlClients, error) {
	result := &CtrlClients{
		ctrlMap: map[string]ctrlClient{},
	}
	if err := result.init(run, selector); err != nil {
		return nil, err
	}
	return result, nil
}

type ctrlClient struct {
	c       *model.Component
	clients *zitirest.Clients
}

type CtrlClients struct {
	ctrlMap map[string]ctrlClient
	sync.Mutex
}

func (self *CtrlClients) init(run model.Run, selector string) error {
	self.ctrlMap = map[string]ctrlClient{}
	ctrls := run.GetModel().SelectComponents(selector)
	resultC := make(chan struct {
		err     error
		c       *model.Component
		clients *zitirest.Clients
	}, len(ctrls))

	for _, ctrl := range ctrls {
		go func() {
			clients, err := EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
			resultC <- struct {
				err     error
				c       *model.Component
				clients *zitirest.Clients
			}{
				err:     err,
				c:       ctrl,
				clients: clients,
			}
		}()
	}

	for i := 0; i < len(ctrls); i++ {
		result := <-resultC
		if result.err != nil {
			return result.err
		}
		self.ctrlMap[result.c.Id] = ctrlClient{
			c:       result.c,
			clients: result.clients,
		}
	}
	return nil
}

func (self *CtrlClients) getRandomCtrl() *zitirest.Clients {
	ctrls := slices.Collect(maps.Values(self.ctrlMap))
	return ctrls[rand.Intn(len(ctrls))].clients
}

func (self *CtrlClients) getCtrl(id string) *zitirest.Clients {
	if result, ok := self.ctrlMap[id]; ok {
		return result.clients
	}
	return nil
}

func (self *CtrlClients) EnsureCtrlAuthed(id string, maxTimeSinceLastAuth time.Duration) error {
	ctrl, ok := self.ctrlMap[id]
	if !ok {
		return fmt.Errorf("controller with id '%s' not found", id)
	}

	username := ctrl.c.MustStringVariable("credentials.edge.username")
	password := ctrl.c.MustStringVariable("credentials.edge.password")
	return ctrl.clients.AuthenticateIfNeeded(username, password, maxTimeSinceLastAuth)
}

func (self *CtrlClients) Inspect(ctrlId, appRegex, targetDir, format string, requestValues ...string) error {
	if err := self.EnsureCtrlAuthed(ctrlId, 5*time.Minute); err != nil {
		return err
	}

	ctrl := self.getCtrl(ctrlId)
	if ctrl == nil {
		return fmt.Errorf("controller with id '%s' not found", ctrlId)
	}

	inspectOk, err := ctrl.Fabric.Inspect.Inspect(&inspect.InspectParams{
		Request: &rest_model.InspectRequest{
			AppRegex:        &appRegex,
			RequestedValues: requestValues,
		},
		Context: context.Background(),
	})

	if err != nil {
		return err
	}

	if err = os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	result := inspectOk.Payload
	if *result.Success {
		fmt.Printf("Results: (%d)\n", len(result.Values))
		for _, value := range result.Values {
			appId := stringz.OrEmpty(value.AppID)
			name := stringz.OrEmpty(value.Name)
			var out io.Writer
			var file *os.File
			fmt.Printf("output result to: %v.%v\n", appId, name)
			fileName := filepath.Join(targetDir, fmt.Sprintf("%v.%v", appId, name))
			file, err = os.Create(fileName)
			if err != nil {
				return err
			}
			out = file
			if err = self.prettyPrint(out, format, value.Value, 0); err != nil {
				if closeErr := file.Close(); closeErr != nil {
					return errors.Join(err, closeErr)
				}
				return err
			}
			if file != nil {
				if err = file.Close(); err != nil {
					return err
				}
			}
		}
	} else {
		fmt.Printf("\nEncountered errors: (%d)\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("\t%v\n", err)
		}
	}

	return nil
}

func (self *CtrlClients) prettyPrint(o io.Writer, format string, val interface{}, indent uint) error {
	if strVal, ok := val.(string); ok {
		if strings.IndexByte(strVal, '\n') > 0 {
			lines := strings.Split(strVal, "\n")
			if _, err := fmt.Fprintln(o, lines[0]); err != nil {
				return err
			}
			for _, line := range lines[1:] {
				for i := uint(0); i < indent; i++ {
					if _, err := fmt.Fprintf(o, " "); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(o, line); err != nil {
					return err
				}
			}
		} else {
			if _, err := fmt.Fprintf(o, "%v\n", val); err != nil {
				return err
			}
		}
		return nil
	}

	if format == "yaml" {
		return yaml.NewEncoder(o).Encode(val)
	}

	if format == "json" {
		enc := json.NewEncoder(o)
		enc.SetIndent("", "    ")
		return enc.Encode(val)
	}
	return fmt.Errorf("unsupported format %v", format)
}
