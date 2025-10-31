package tests

import (
	"errors"
	"fmt"

	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

type CustomAssertions struct {
	*require.Assertions
}

func NewCustomAssertions(t require.TestingT) *CustomAssertions {
	return &CustomAssertions{require.New(t)}
}

func (c *CustomAssertions) ApiError(err error) {
	restApiError := &util.RestApiError{}
	if errors.As(err, restApiError) {
		return
	}
	c.Fail(fmt.Sprintf("expected error to be an api error, got %T", err))
}

func (c *CustomAssertions) ApiErrorWithCode(err error, code string) {
	// if not wrapped
	if apiErrorPayload, ok := err.(rest_util.ApiErrorPayload); ok {
		if apiErrorPayload == nil {
			c.Fail("expected ApiErrorPayload to not be nil")
		}

		payload := apiErrorPayload.GetPayload()
		if payload == nil {
			c.Fail("expected RestAPIError to have payload, got nil")
			return
		}

		if payload.Error == nil {
			c.Fail("expected RestApiError payload to have an error, got nil")
			return
		}

		if payload.Error.Code != code {
			c.Fail(fmt.Sprintf("expected RestApiError payload to have code %s, got %s", code, payload.Error.Code))
			return
		}

		//success
		return
	}

	//if wrapped
	restApiFormattedError := &rest_util.APIFormattedError{}
	if errors.As(err, &restApiFormattedError) {
		if restApiFormattedError.Code != code {
			c.Fail(fmt.Sprintf("expected RestApiError payload to have code %s, got %s", code, restApiFormattedError.Code))
			return
		}

		//success
		return
	}

	c.Fail(fmt.Sprintf("expected error to be an RestApiError, got %T", err))
}
