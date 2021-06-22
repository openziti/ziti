package health

import (
	"encoding/json"
	"fmt"
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/openziti/fabric/xweb"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

const (
	Binding = "health-checks"
)

var _ xweb.WebHandlerFactory = &HealthCheckApiFactory{}

func NewHealthCheckApiFactory(healthChecker gosundheit.Health) *HealthCheckApiFactory {
	return &HealthCheckApiFactory{
		healthChecker: healthChecker,
	}
}

type HealthCheckApiFactory struct {
	healthChecker gosundheit.Health
}

func (factory HealthCheckApiFactory) Validate(*xweb.Config) error {
	return nil
}

func (factory HealthCheckApiFactory) Binding() string {
	return Binding
}

func (factory HealthCheckApiFactory) New(_ *xweb.WebListener, options map[interface{}]interface{}) (xweb.WebHandler, error) {
	return &HealthCheckApiHandler{
		healthChecker: factory.healthChecker,
		options:       options,
	}, nil
}

type HealthCheckApiHandler struct {
	options       map[interface{}]interface{}
	healthChecker gosundheit.Health
}

func (self *HealthCheckApiHandler) Binding() string {
	return Binding
}

func (self *HealthCheckApiHandler) Options() map[interface{}]interface{} {
	return self.options
}

func (self *HealthCheckApiHandler) RootPath() string {
	return "/health-checks"
}

func (self *HealthCheckApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath())
}

func (self *HealthCheckApiHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	output := map[string]interface{}{}
	output["meta"] = map[string]interface{}{}

	data := map[string]interface{}{}
	output["data"] = data

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")

	results, healthy := self.healthChecker.Results()
	data["healthy"] = healthy
	var checks []map[string]interface{}

	shortFormat := request.URL.Query().Get("type") == "short"

	for id, result := range results {
		check := map[string]interface{}{}
		checks = append(checks, check)
		check["id"] = id
		check["healthy"] = result.IsHealthy()
		if !shortFormat {
			check["lastCheckDuration"] = fmt.Sprintf("%v", result.Duration)
			check["lastCheckTime"] = result.Timestamp.UTC().Format(time.RFC3339)

			if result.Error != nil {
				check["err"] = result.Error
				check["consecutiveFailures"] = result.ContiguousFailures
			}

			if result.TimeOfFirstFailure != nil {
				check["failingSince"] = fmt.Sprintf("%v", *result.TimeOfFirstFailure)
			}
		}
	}

	data["checks"] = checks
	if err := encoder.Encode(output); err != nil {
		logrus.WithError(err).Error("failure encoding health check results")
	}
}
