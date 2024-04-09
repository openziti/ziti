/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	openApiRuntime "github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_model"
	fabric_rest_client "github.com/openziti/ziti/controller/rest_client"
	"gopkg.in/resty.v1"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Use a 2-second timeout with a retry count of 5
func NewClient() *resty.Client {
	return resty.
		New().
		SetTimeout(2 * time.Second).
		SetRetryCount(5).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
}

func PrettyPrintResponse(resp *resty.Response) string {
	out := resp.String()
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(out), "", "    "); err == nil {
		return prettyJSON.String()
	}
	return out
}

func OutputJson(out io.Writer, data []byte) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "    "); err == nil {
		if _, err := fmt.Fprint(out, prettyJSON.String()); err != nil {
			panic(err)
		}
	} else {
		if _, err := fmt.Fprint(out, data); err != nil {
			panic(err)
		}
	}
}

func ControllerDetailEntity(api API, entityType, entityId string, logJSON bool, out io.Writer, timeout int, verbose bool) (*gabs.Container, error) {
	restClientIdentity, err := LoadSelectedIdentityForApi(api)
	if err != nil {
		return nil, err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return nil, err
	}

	queryUrl := baseUrl + "/" + path.Join(entityType, entityId)

	resp, err := req.Get(queryUrl)

	if err != nil {
		return nil, fmt.Errorf("unable to list entities at %v in Ziti Edge Controller at %v. Error: %v", queryUrl, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error listing %v in Ziti Edge Controller. Status code: %v, Server returned: %v",
			queryUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logJSON {
		OutputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", queryUrl, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerListSubEntities will list entities of the given type in the given Edge Controller
func EdgeControllerListSubEntities(entityType, subType, entityId string, filter string, logJSON bool, out io.Writer, timeout int, verbose bool) (*gabs.Container, error) {
	params := url.Values{}
	if filter != "" {
		params.Add("filter", filter)
	}
	return EdgeControllerList(entityType+"/"+entityId+"/"+subType, params, logJSON, out, timeout, verbose)
}

// EdgeControllerList will list entities of the given type in the given Edge Controller
func EdgeControllerList(path string, params url.Values, logJSON bool, out io.Writer, timeout int, verbose bool) (*gabs.Container, error) {
	return ControllerList("edge", path, params, logJSON, out, timeout, verbose)
}

// ControllerList will list entities of the given type in the given Edge Controller
func ControllerList(api API, path string, params url.Values, logJSON bool, out io.Writer, timeout int, verbose bool) (*gabs.Container, error) {
	restClientIdentity, err := LoadSelectedIdentityForApi(api)
	if err != nil {
		return nil, err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return nil, err
	}

	queryUrl := baseUrl + "/" + path

	if len(params) > 0 {
		queryUrl += "?" + params.Encode()
	}

	resp, err := req.Get(queryUrl)

	if err != nil {
		return nil, fmt.Errorf("unable to list entities at %v in Ziti Controller at %v. Error: %v", queryUrl, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error listing %v in Ziti Edge Controller. Status code: %v, Server returned: %v",
			queryUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logJSON {
		OutputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", queryUrl, resp.String())
	}

	return jsonParsed, nil
}

var _ http.RoundTripper = &edgeTransport{}

type edgeTransport struct {
	*http.Transport
	RequestFunc  func(*http.Request) error
	ResponseFunc func(*http.Response, error)
}

func (edgeTransport *edgeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if edgeTransport.RequestFunc != nil {
		if err := edgeTransport.RequestFunc(r); err != nil {
			return nil, err
		}
	}

	resp, err := edgeTransport.Transport.RoundTrip(r)

	if edgeTransport.ResponseFunc != nil {
		edgeTransport.ResponseFunc(resp, err)
	}

	return resp, err
}

type ApiErrorPayload interface {
	Error() string
	GetPayload() *rest_model.APIErrorEnvelope
}

type RestApiError struct {
	ApiErrorPayload
}

func formatApiError(error *rest_model.APIError) string {
	cause := ""
	if error.Cause != nil {
		if error.Cause.APIError.Code != "" {
			cause = formatApiError(&error.Cause.APIError)
		} else if error.Cause.APIFieldError.Field != "" {
			cause = fmt.Sprintf("INVALID_FIELD - %s [%s] %s", error.Cause.APIFieldError.Field, error.Cause.APIFieldError.Value, error.Cause.APIFieldError.Reason)
		}
	}

	if cause != "" {
		cause = ": " + cause
	}
	return fmt.Sprintf("%s - %s%s", error.Code, error.Message, cause)
}

func (a RestApiError) Error() string {
	if payload := a.ApiErrorPayload.GetPayload(); payload != nil {

		if payload.Error == nil {
			return fmt.Sprintf("could not read API error, payload.error was nil: %v", a.ApiErrorPayload.Error())
		}
		return formatApiError(payload.Error)
	}

	return fmt.Sprintf("could not read API error, payload was nil: %v", a.ApiErrorPayload.Error())
}

func WrapIfApiError(err error) error {
	if apiErrorPayload, ok := err.(ApiErrorPayload); ok {
		return &RestApiError{apiErrorPayload}
	}

	return err
}

type ClientOpts interface {
	OutputRequestJson() bool
	OutputResponseJson() bool
	OutputWriter() io.Writer
	ErrOutputWriter() io.Writer
}

func NewEdgeManagementClient(clientOpts ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error) {
	restClientIdentity, err := LoadSelectedIdentity()
	if err != nil {
		return nil, err
	}
	return restClientIdentity.NewEdgeManagementClient(clientOpts)
}

func NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error) {
	restClientIdentity, err := LoadSelectedIdentity()
	if err != nil {
		return nil, err
	}
	return restClientIdentity.NewFabricManagementClient(clientOpts)
}

type EdgeManagementAuth struct {
	Token string
}

func (e EdgeManagementAuth) AuthenticateRequest(request openApiRuntime.ClientRequest, registry strfmt.Registry) error {
	return request.SetHeaderParam("zt-session", e.Token)
}

// ControllerCreate will create entities of the given type in the given Edge Controller
func ControllerCreate(api API, entityType string, body string, out io.Writer, logRequestJson, logResponseJson bool, timeout int, verbose bool) (*gabs.Container, error) {
	restClientIdentity, err := LoadSelectedRWIdentityForApi(api)
	if err != nil {
		return nil, err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return nil, err
	}

	url := baseUrl + "/" + entityType
	if logRequestJson {
		fmt.Printf("%v to %v\n", "POST", url)
		OutputJson(out, []byte(body))
		fmt.Println()
	}

	resp, err := req.SetBody(body).Post(url)

	if err != nil {
		return nil, fmt.Errorf("unable to create %v instance in Ziti Edge Controller at %v. Error: %v", entityType, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("error creating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, baseUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logResponseJson {
		OutputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", baseUrl, resp.String())
	}

	return jsonParsed, nil
}

// ControllerDelete will delete entities of the given type in the given Controller
func ControllerDelete(api API, entityType string, id string, body string, out io.Writer, logRequestJson bool, logResponseJson bool, timeout int, verbose bool) (error, *int) {
	restClientIdentity, err := LoadSelectedRWIdentityForApi(api)
	if err != nil {
		return err, nil
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return err, nil
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return err, nil
	}

	entityPath := entityType + "/" + id
	fullUrl := baseUrl + "/" + entityPath

	if logRequestJson {
		fmt.Printf("%v to %v\n", "POST", fullUrl)
		OutputJson(out, []byte(body))
		fmt.Println()
	}

	if body != "" {
		req = req.SetBody(body)
	}

	resp, err := req.Delete(fullUrl)

	if err != nil {
		return fmt.Errorf("unable to delete %v instance in Ziti Edge Controller at %v. Error: %v", entityPath, baseUrl, err), nil
	}

	if resp.StatusCode() != http.StatusOK {
		statusCode := resp.StatusCode()
		return fmt.Errorf("error deleting %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityPath, baseUrl, resp.Status(), PrettyPrintResponse(resp)), &statusCode
	}

	if logResponseJson {
		OutputJson(out, resp.Body())
	}

	return nil, nil
}

// ControllerUpdate will update entities of the given type in the given Edge Controller
func ControllerUpdate(api API, entityType string, body string, out io.Writer, method string, logRequestJson, logResponseJSON bool, timeout int, verbose bool) (*gabs.Container, error) {
	restClientIdentity, err := LoadSelectedRWIdentityForApi(api)
	if err != nil {
		return nil, err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return nil, err
	}

	url := baseUrl + "/" + entityType

	if logRequestJson {
		fmt.Printf("%v to %v\n", method, url)
		OutputJson(out, []byte(body))
		fmt.Println()
	}

	resp, err := req.SetBody(body).Execute(method, url)

	if err != nil {
		return nil, fmt.Errorf("unable to update %v instance in Ziti Edge Controller at %v. Error: %v", entityType, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusAccepted {
		return nil, fmt.Errorf("error updating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, baseUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logResponseJSON {
		OutputJson(out, resp.Body())
	}

	if len(resp.Body()) == 0 {
		return nil, nil
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", baseUrl, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerVerify will verify entities of the given type in the given Edge Controller
func EdgeControllerVerify(entityType, id, body string, out io.Writer, logJSON bool, timeout int, verbose bool) error {
	restClientIdentity, err := LoadSelectedRWIdentity()
	if err != nil {
		return err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi("edge")
	if err != nil {
		return err
	}

	client, err := restClientIdentity.NewClient(time.Duration(timeout)*time.Second, verbose)
	if err != nil {
		return err
	}

	req := restClientIdentity.NewRequest(client)

	resp, err := req.
		SetHeader("Content-Type", "text/plain").
		SetBody(body).
		Post(baseUrl + "/" + entityType + "/" + id + "/verify")

	if err != nil {
		return fmt.Errorf("unable to verify %v instance [%s] in Ziti Edge Controller at %v. Error: %v", entityType, id, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error verifying %v instance (%v) in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, id, baseUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logJSON {
		OutputJson(out, resp.Body())
	}

	return nil
}

func EdgeControllerRequest(entityType string, out io.Writer, logJSON bool, timeout int, verbose bool, doRequest func(*resty.Request, string) (*resty.Response, error)) (*gabs.Container, error) {
	restClientIdentity, err := LoadSelectedRWIdentity()
	if err != nil {
		return nil, err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi("edge")
	if err != nil {
		return nil, err
	}

	request, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return nil, err
	}

	resp, err := doRequest(request, baseUrl+"/"+entityType)

	if err != nil {
		return nil, fmt.Errorf("unable to [%s] %v instance in Ziti Edge Controller at %v. Error: %v", request.Method, entityType, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error performing request [%s] %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			request.Method, entityType, baseUrl, resp.Status(), PrettyPrintResponse(resp))
	}

	if logJSON {
		OutputJson(out, resp.Body())
	}

	if resp.Body() == nil {
		return nil, nil
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", baseUrl, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerGetManagementApiBasePath accepts host as `http://domain:port` and attempts to
// determine the proper path that should be used to access the Edge Management API. Depending
// on the version of the Edge Controller the API may be monolith on `/edge/<version>` and `/` or split into
// `/edge/management/<version>` and `/edge/client/<version>`.
func EdgeControllerGetManagementApiBasePath(host string, cert string) string {
	client := NewClient()

	client.SetHostURL(host)

	if cert != "" {
		client.SetRootCertificate(cert)
	}

	// check v1 path first
	resp, err := client.R().Get("/edge/client/v1/version")

	if err != nil || resp.StatusCode() != http.StatusOK {
		// if v1 path fails, fall back to removed /version path
		resp, err = client.R().Get("/version")

		if err != nil || resp.StatusCode() != http.StatusOK {
			return host
		}
	}

	data, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return host
	}

	// controller w/ APIs split
	if data.ExistsP("data.apiVersions.edge-management") {
		if path, ok := data.Path("data.apiVersions.edge-management.v1.path").Data().(string); !ok {
			return host
		} else {
			return host + path
		}
	}

	return host
}
