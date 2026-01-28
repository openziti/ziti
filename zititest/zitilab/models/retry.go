package models

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/ziti/util"
)

var RetryPolicy = func(task parallel.LabeledTask, attempt int, err error) parallel.ErrorAction {
	var apiErr util.ApiErrorPayload
	var msg string
	if errors.As(err, &apiErr) {
		if strings.HasPrefix(task.Type(), "delete.") { // If it's not found, it can't be deleted
			if apiErr.GetPayload().Error.Code == errorz.NotFoundCode {
				return parallel.ErrActionIgnore
			}
		} else if strings.HasPrefix(task.Type(), "create.") && attempt > 1 {
			if apiErr.GetPayload().Error.Code == errorz.CouldNotValidateCode {
				return parallel.ErrActionIgnore
			}
		}
		msg = apiErr.GetPayload().Error.Message
	}

	log := pfxlog.Logger().WithField("attempt", attempt).WithError(err).WithField("task", task.Label())

	var runtimeErr *runtime.APIError
	if errors.As(err, &runtimeErr) {
		if cp, ok := runtimeErr.Response.(runtime.ClientResponse); ok {
			body, _ := io.ReadAll(cp.Body())
			log.WithField("msg", cp.Message()).WithField("body", string(body)).Error("runtime error")
		}
	}

	if attempt > 3 {
		return parallel.ErrActionReport
	}
	if msg != "" {
		log = log.WithField("msg", msg)
	}
	log.Error("action failed, retrying")
	time.Sleep(time.Duration(attempt*10) * time.Second)
	return parallel.ErrActionRetry
}
