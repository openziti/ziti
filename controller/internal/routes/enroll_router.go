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

package routes

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/fullsailor/pkcs7"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_server/operations/enroll"
	client_well_known "github.com/openziti/edge-api/rest_client_api_server/operations/well_known"
	management_well_known "github.com/openziti/edge-api/rest_management_api_server/operations/well_known"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	cert2 "github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"io"
	"net/http"
	"strings"
)

func init() {
	r := NewEnrollRouter()
	env.AddRouter(r)
}

type EnrollRouter struct {
}

func NewEnrollRouter() *EnrollRouter {
	return &EnrollRouter{}
}

func (ro *EnrollRouter) AddMiddleware(ae *env.AppEnv) {
	ae.ClientApi.AddMiddlewareFor("POST", "/enroll", func(next http.Handler) http.Handler {
		// This endpoint is hijacked as middleware to stop automatic processing of the body.
		// This is done due to OpenAPI 2.0 not being able to specify multiple input and output types with/without
		// schemas. Specifically when dealing with input of a plain text CSR vs JSON (i.e. for ott vs updb)
		// This endpoint should not be used. The enroll method specific endpoint should be used instead. It is impossible
		// to define an OpenAPI 2.0 compliant spec for this endpoint. The best we can do is provide one that is
		// flexible enough to get data in/out for clients and handle the assumptions in code.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			fullCt := strings.TrimSpace(r.Header.Get("content-type"))

			method := r.URL.Query().Get("method")

			if method == "" {
				method = rest_model.EnrollmentCreateMethodOtt
			}

			//missing content-type, let the normal processing handle the error production
			//ca is skipped because it may have no body and thus no content type
			if fullCt == "" && method != "ca" {
				next.ServeHTTP(w, r)
				return
			}

			mediaType := strings.Split(fullCt, ";")[0]

			switch mediaType {
			//special handling for legacy clients that are not spec compliant
			case "application/pkcs7", "application/x-pem-file", "text/plain", "":
				// empty string = no body (e.g. 3rd party ca)
				ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
					rc.ResponseWriter.Header().Set("content-type", "application/json")
					rc.SetProducer(runtime.JSONProducer())
					ro.legacyGenericEnrollPemHandler(ae, rc)
				}, r, "", "", permissions.Always()).WriteResponse(w, nil)
			default:
				//use spec compliant processing for application/json and or unsupported types (for error generation)
				next.ServeHTTP(w, r)
			}
		})
	})
}

func (ro *EnrollRouter) Register(ae *env.AppEnv) {
	//Enroll
	ae.ClientApi.EnrollEnrollHandler = enroll.EnrollHandlerFunc(func(params enroll.EnrollParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.enrollHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollEnrollCaHandler = enroll.EnrollCaHandlerFunc(func(params enroll.EnrollCaParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.caHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollEnrollOttCaHandler = enroll.EnrollOttCaHandlerFunc(func(params enroll.EnrollOttCaParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.ottCaHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollEnrollOttHandler = enroll.EnrollOttHandlerFunc(func(params enroll.EnrollOttParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.ottHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollEnrollErOttHandler = enroll.EnrollErOttHandlerFunc(func(params enroll.EnrollErOttParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.erOttHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollEnrollUpdbHandler = enroll.EnrollUpdbHandlerFunc(func(params enroll.EnrollUpdbParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.updbHandler(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	// Extend Enrollment
	ae.ClientApi.EnrollExtendRouterEnrollmentHandler = enroll.ExtendRouterEnrollmentHandlerFunc(func(params enroll.ExtendRouterEnrollmentParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.extendRouterEnrollment(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	// Utility, well-known
	ae.ClientApi.WellKnownListWellKnownCasHandler = client_well_known.ListWellKnownCasHandlerFunc(func(params client_well_known.ListWellKnownCasParams) middleware.Responder {
		return ae.IsAllowed(ro.getCaCerts, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.WellKnownListWellKnownCasHandler = management_well_known.ListWellKnownCasHandlerFunc(func(params management_well_known.ListWellKnownCasParams) middleware.Responder {
		return ae.IsAllowed(ro.getCaCerts, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (ro *EnrollRouter) getCaCerts(ae *env.AppEnv, rc *response.RequestContext) {
	rc.ResponseWriter.Header().Set("content-type", "application/pkcs7-mime")
	rc.ResponseWriter.Header().Set("Content-Transfer-Encoding", "base64")
	rc.ResponseWriter.WriteHeader(http.StatusOK)

	// Decode each PEM block in the input and append the ASN.1
	// DER bytes for each certificate therein to the data slice.

	input := ae.GetConfig().Edge.CaPems()
	var data []byte

	for len(input) > 0 {
		var block *pem.Block
		block, input = pem.Decode(input)
		data = append(data, block.Bytes...)
	}

	// Build a PKCS#7 degenerate "certs only" structure from that ASN.1 certificates data.
	var err error
	data, err = pkcs7.DegenerateCertificate(data)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected issue creating pkcs7 degenerate: %s", err)
		rc.RespondWithError(errorz.NewUnhandled(err))
		return
	}

	//encode as b64 and write to a string so the string can be written out in 64 byte lines
	//there has to be a standard library for this - it feels strange to have to reinvent this
	//write the bytes out in 64 byte lines...
	bytes := []byte(base64.StdEncoding.EncodeToString(data))
	step := 64
	tot := len(bytes)
	for i := 0; i < tot; i += step {
		if i+step < tot {
			_, _ = rc.ResponseWriter.Write(bytes[i : i+step])
			_, _ = rc.ResponseWriter.Write([]byte("\n"))
		} else {
			_, _ = rc.ResponseWriter.Write(bytes[i:tot])
		}
	}
}

func (ro *EnrollRouter) ottHandler(ae *env.AppEnv, rc *response.RequestContext, params enroll.EnrollOttParams) {
	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	enrollContext := &model.EnrollmentContextHttp{
		Headers: headers,
		Data: &model.EnrollmentData{
			ClientCsrPem: []byte(params.OttEnrollmentRequest.ClientCsr),
		},
		Certs:         rc.Request.TLS.PeerCertificates,
		Token:         params.OttEnrollmentRequest.Token,
		Method:        db.MethodEnrollOtt,
		ChangeContext: changeCtx,
	}

	enrollContext.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) processEnrollContext(ae *env.AppEnv, rc *response.RequestContext, enrollContext *model.EnrollmentContextHttp) {
	result, err := ae.Managers.Enrollment.Enroll(enrollContext)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if result == nil {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	//prefer json producer for non ott methods (backwards compat for ott)
	explicitJsonAccept := false
	if accept := rc.Request.Header.Values("accept"); len(accept) == 0 {
		explicitJsonAccept = false //no headers specified
	} else {
		for _, val := range accept {
			if strings.Split(val, ";")[0] == "application/json" {
				explicitJsonAccept = true
			}
		}
	}

	// for non ott enrollment, always return JSON
	//prefer JSON if explicitly acceptableN
	if enrollContext.GetMethod() != db.MethodEnrollOtt || explicitJsonAccept {
		rc.SetProducer(runtime.JSONProducer())
	}

	rc.RespondWithOk(result.Content, &rest_model.Meta{})
}

// legacyGenericEnrollPemHandler handles legacy generic enrollment. It should not be used and is considered deprecated.
//
// This endpoint is hijacked as middleware to stop automatic processing of the body.
// This is done due to OpenAPI 2.0 not being able to specify multiple input and output types with/without
// schemas. Specifically when dealing with input of a plain text CSR vs JSON (i.e. for ott vs updb)
// This endpoint should not be used. The enroll method specific endpoint should be used instead. It is impossible
// to define an OpenAPI 2.0 compliant spec for this endpoint. The best we can do is provide one that is
// flexible enough to get data in/out for clients and handle the assumptions in code.
func (ro *EnrollRouter) legacyGenericEnrollPemHandler(ae *env.AppEnv, rc *response.RequestContext) {

	if !willAcceptJson(rc.Request) {
		rc.RespondWithError(&errorz.ApiError{
			Code:    "UNSUPPORTED_MEDIA",
			Message: "No suitable accept media types were detected, supports: application/json",
			Status:  http.StatusUnsupportedMediaType,
		})
		return
	}

	enrollContext := &model.EnrollmentContextHttp{}
	err := enrollContext.FillFromHttpRequest(rc.Request, rc.NewChangeContext())

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	body, err := io.ReadAll(rc.Request.Body)

	if err != nil {
		rc.RespondWithError(fmt.Errorf("could not read body: %w", err))
		return
	}

	enrollContext.Data = &model.EnrollmentData{
		ClientCsrPem: body,
	}

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) extendRouterEnrollment(ae *env.AppEnv, rc *response.RequestContext, params enroll.ExtendRouterEnrollmentParams) {
	peerCerts := rc.Request.TLS.PeerCertificates

	if len(peerCerts) == 0 {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	var cert *x509.Certificate
	for _, peerCert := range peerCerts {
		if !peerCert.IsCA {
			cert = peerCert
		}
	}

	if cert == nil {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	fingerprint := ae.GetFingerprintGenerator().FromCert(cert)

	if fingerprint == "" {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	if params.RouterExtendEnrollmentRequest.CertCsr == nil {
		rc.RespondWithError(errorz.NewFieldApiError(&errorz.FieldError{
			Reason:     "client CSR is required",
			FieldName:  "certCsr",
			FieldValue: params.RouterExtendEnrollmentRequest.CertCsr,
		}))
		return
	}

	if params.RouterExtendEnrollmentRequest.ServerCertCsr == nil {
		rc.RespondWithError(errorz.NewFieldApiError(&errorz.FieldError{
			Reason:     "server CSR is required",
			FieldName:  "serverCertCsr",
			FieldValue: params.RouterExtendEnrollmentRequest.ServerCertCsr,
		}))
		return
	}

	certCsr := []byte(*params.RouterExtendEnrollmentRequest.CertCsr)
	serverCertCsr := []byte(*params.RouterExtendEnrollmentRequest.ServerCertCsr)

	if edgeRouter, _ := ae.Managers.EdgeRouter.ReadOneByFingerprint(fingerprint); edgeRouter != nil {
		certs, err := ae.Managers.EdgeRouter.ExtendEnrollment(edgeRouter, certCsr, serverCertCsr, rc.NewChangeContext())

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		clientChainPem, err := ae.Managers.Enrollment.GetCertChainPem(certs.RawClientCert)
		if err != nil {
			rc.RespondWithError(err)
			return
		}

		serverPem, err := cert2.RawToPem(certs.RawServerCert)

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		rc.RespondWithOk(&rest_model.EnrollmentCerts{
			Cert:       clientChainPem,
			ServerCert: string(serverPem),
		}, &rest_model.Meta{})

		return
	}

	if router, _ := ae.Managers.TransitRouter.ReadOneByFingerprint(fingerprint); router != nil {
		certs, err := ae.Managers.TransitRouter.ExtendEnrollment(router, certCsr, serverCertCsr, rc.NewChangeContext())

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		clientChainPem, err := ae.Managers.Enrollment.GetCertChainPem(certs.RawClientCert)

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		serverPem, err := cert2.RawToPem(certs.RawServerCert)

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		rc.RespondWithOk(&rest_model.EnrollmentCerts{
			Cert:       clientChainPem,
			ServerCert: string(serverPem),
		}, &rest_model.Meta{})

		return
	}

	//default unauthorized
	rc.RespondWithApiError(errorz.NewUnauthorized())
}

func (ro *EnrollRouter) ottCaHandler(ae *env.AppEnv, rc *response.RequestContext, params enroll.EnrollOttCaParams) {
	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	enrollContext := &model.EnrollmentContextHttp{
		Headers: headers,
		Data: &model.EnrollmentData{
			ClientCsrPem: []byte(params.OttEnrollmentRequest.ClientCsr),
		},
		Certs:         rc.Request.TLS.PeerCertificates,
		Token:         params.OttEnrollmentRequest.Token,
		Method:        db.MethodEnrollOttCa,
		ChangeContext: changeCtx,
	}

	enrollContext.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) updbHandler(ae *env.AppEnv, rc *response.RequestContext, params enroll.EnrollUpdbParams) {
	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	enrollContext := &model.EnrollmentContextHttp{
		Headers: headers,
		Data: &model.EnrollmentData{
			Username: string(params.UpdbCredentials.Username),
			Password: string(params.UpdbCredentials.Password),
		},
		Certs:         rc.Request.TLS.PeerCertificates,
		Token:         string(params.Token),
		Method:        db.MethodEnrollUpdb,
		ChangeContext: changeCtx,
	}

	enrollContext.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) erOttHandler(ae *env.AppEnv, rc *response.RequestContext, params enroll.EnrollErOttParams) {
	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	enrollContext := &model.EnrollmentContextHttp{
		Headers: headers,
		Data: &model.EnrollmentData{
			ClientCsrPem: []byte(params.ErOttEnrollmentRequest.ClientCsr),
		},
		Certs:         rc.Request.TLS.PeerCertificates,
		Token:         params.ErOttEnrollmentRequest.Token,
		Method:        model.MethodEnrollEdgeRouterOtt,
		ChangeContext: changeCtx,
	}

	enrollContext.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) caHandler(ae *env.AppEnv, rc *response.RequestContext, _ enroll.EnrollCaParams) {
	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	enrollContext := &model.EnrollmentContextHttp{
		Headers:       headers,
		Certs:         rc.Request.TLS.PeerCertificates,
		Method:        "ca",
		ChangeContext: changeCtx,
	}

	enrollContext.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	ro.processEnrollContext(ae, rc, enrollContext)
}

func (ro *EnrollRouter) enrollHandler(ae *env.AppEnv, rc *response.RequestContext, params enroll.EnrollParams) {
	method := stringz.OrEmpty(params.Method)
	token := ""
	if params.Token != nil {
		token = params.Token.String()
	}

	changeCtx := rc.NewChangeContext()

	headers := map[string]interface{}{}
	for h, v := range rc.Request.Header {
		headers[h] = v
	}

	if params.Body == nil {
		params.Body = &rest_model.GenericEnroll{}
	}

	enrollCtx := &model.EnrollmentContextHttp{
		Headers:    headers,
		Parameters: nil,
		Data: &model.EnrollmentData{
			RequestedName: params.Body.Name,
			ServerCsrPem:  []byte(params.Body.ServerCertCsr),
			ClientCsrPem:  []byte(params.Body.ClientCsr),
			Username:      string(params.Body.Username),
			Password:      string(params.Body.Password),
		},
		Certs:         rc.Request.TLS.PeerCertificates,
		Token:         token,
		Method:        method,
		ChangeContext: changeCtx,
	}

	if method == model.MethodEnrollEdgeRouterOtt || method == model.MethodEnrollTransitRouterOtt {
		// enrolling routers use `certCsr` not `clientCsr`
		enrollCtx.Data.ClientCsrPem = []byte(params.Body.CertCsr)
	}

	ro.processEnrollContext(ae, rc, enrollCtx)
}

func willAcceptJson(r *http.Request) bool {
	acceptHeader := r.Header.Get("Accept")

	// no header is equivalent to */*
	if acceptHeader == "" {
		return true
	}

	// Split by comma to handle multiple media types
	mediaTypes := strings.Split(acceptHeader, ",")
	for _, mediaType := range mediaTypes {
		// Extract the MIME type before a possible semicolon
		media := strings.TrimSpace(strings.Split(mediaType, ";")[0])
		if media == "application/json" || media == "*/*" {
			return true
		}
	}
	return false
}
