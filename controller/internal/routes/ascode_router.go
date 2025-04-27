package routes

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	mgmtclient "github.com/openziti/edge-api/rest_management_api_client"
	ascodeops "github.com/openziti/edge-api/rest_management_api_server/operations/ascode"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/ziti/cmd/ascode/exporter"
	"github.com/openziti/ziti/ziti/cmd/ascode/importer"
)

func init() {
	r := &AscodeRouter{}
	env.AddRouter(r)
}

type AscodeRouter struct{}

func (r *AscodeRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.AscodeExportAscodeHandler = ascodeops.ExportAscodeHandlerFunc(func(params ascodeops.ExportAscodeParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Export(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AscodeImportAscodeHandler = ascodeops.ImportAscodeHandlerFunc(func(params ascodeops.ImportAscodeParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Import(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *AscodeRouter) Export(ae *env.AppEnv, rc *response.RequestContext, params ascodeops.ExportAscodeParams) {
	var types []string
	if params.ExportRequest.Types != "" {
		types = strings.Split(params.ExportRequest.Types, ",")
	}

	format := "yaml"
	if params.ExportRequest.Format != "" {
		format = strings.ToLower(params.ExportRequest.Format)
	}

	address := ae.GetConfig().Edge.Api.Address
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	transport := httptransport.New(address, "/edge/management/v1", []string{"https", "http"})
	transport.Transport = tr
	token := rc.SessionToken
	if rc.IsJwtToken {
		transport.DefaultAuthentication = httptransport.BearerToken(token)
	} else {
		transport.DefaultAuthentication = httptransport.APIKeyAuth(env.ZitiSession, "header", token)
	}
	client := mgmtclient.New(transport, strfmt.Default)

	var buf bytes.Buffer
	exp := &exporter.Exporter{
		Out:    *bufio.NewWriter(&buf),
		Err:    io.Discard,
		Client: client,
	}
	result, err := exp.Execute(types)
	if err != nil {
		rc.RespondWithError(err)
		return
	}

	var output []byte
	switch format {
	case "json":
		output, err = json.MarshalIndent(result, "", "  ")
	case "yaml":
		output, err = yaml.Marshal(result)
	default:
		rc.RespondWithError(fmt.Errorf("unsupported format %q", format))
		return
	}
	if err != nil {
		rc.RespondWithError(err)
		return
	}

	filename := fmt.Sprintf("ziti-export-%s.%s", time.Now().UTC().Format("20060102-150405"), format)
	rc.ResponseWriter.Header().Set("Content-Type", "application/"+format)
	rc.ResponseWriter.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = rc.ResponseWriter.Write(output)
}

func (r *AscodeRouter) Import(ae *env.AppEnv, rc *response.RequestContext, params ascodeops.ImportAscodeParams) {
	body, err := io.ReadAll(params.HTTPRequest.Body)
	if err != nil {
		rc.RespondWithError(err)
		return
	}

	dryRun := params.DryRun != nil && *params.DryRun

	format := "yaml"
	if strings.Contains(params.HTTPRequest.Header.Get("Content-Type"), "json") {
		format = "json"
	}

	data := map[string][]interface{}{}
	switch format {
	case "json":
		if err := json.Unmarshal(body, &data); err != nil {
			rc.RespondWithError(err)
			return
		}
	default: // yaml
		if err := yaml.Unmarshal(body, &data); err != nil {
			rc.RespondWithError(err)
			return
		}
	}

	address := ae.GetConfig().Edge.Api.Address
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	transport := httptransport.New(address, "/edge/management/v1", []string{"https", "http"})
	transport.Transport = tr
	token := rc.SessionToken
	if rc.IsJwtToken {
		transport.DefaultAuthentication = httptransport.BearerToken(token)
	} else {
		transport.DefaultAuthentication = httptransport.APIKeyAuth(env.ZitiSession, "header", token)
	}
	client := mgmtclient.New(transport, strfmt.Default)

	imp := &importer.Importer{
		Out:    io.Discard,
		Err:    io.Discard,
		Client: client,
		Data:   data,
	}

	if !dryRun {
		if err := imp.Execute([]string{"all"}); err != nil {
			rc.RespondWithError(err)
			return
		}
	}

	rc.ResponseWriter.WriteHeader(http.StatusNoContent)
}
