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

package oidc_auth

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag/jsonutils"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	queryAuthRequestID    = "authRequestID"
	queryAuthRequestIdAlt = "id"

	//page specific resource keys
	pageLogin = "login"
	pageTotp  = "totp"

	//method specific login URLs
	passwordLoginUrl = "/oidc/login/username?authRequestID="
	certLoginUrl     = "/oidc/login/cert?authRequestID="
	extJwtLoginUrl   = "/oidc/login/ext-jwt?authRequestID="

	AuthRequestIdHeader = "auth-request-id"
	AcceptHeader        = "accept"
	AuthorizationHeader = "authorization"
	TotpRequiredHeader  = "totp-required"
	ContentTypeHeader   = "content-type"

	FormContentType = "application/x-www-form-urlencoded"
	JsonContentType = "application/json"
	HtmlContentType = "text/html"
)

// embedded file/HTML resources
var (
	//go:embed resources
	resources embed.FS
	pages     = map[string]string{
		pageLogin: "resources/login.html",
		pageTotp:  "resources/totp.html",
	}
	loginTemplate *template.Template
	totpTemplate  *template.Template
)

// init loads page templates and makes them ready for use
func init() {
	var err error
	t1, err := loadTemplate(pageLogin)
	loginTemplate = t1

	if err != nil {
		panic(err)
	}

	t2, err := loadTemplate(pageTotp)
	totpTemplate = t2

	if err != nil {
		panic(err)
	}
}

// loadTemplate will load embedded resource files by name
func loadTemplate(name string) (*template.Template, error) {
	pageBytes, err := resources.ReadFile(pages[name])

	if err != nil {
		return nil, fmt.Errorf("could not read %s resource file", name)
	}

	pageTemplate, err := template.New(name).Parse(string(pageBytes))

	if err != nil {
		return nil, fmt.Errorf("could not parse %s template", name)

	}
	return pageTemplate, err
}

// login represents a set of Storage scoped components used to fulfill HTTP requests from clients
type login struct {
	store    Storage
	router   *mux.Router
	callback func(context.Context, string) string
}

// newLogin create a login
func newLogin(store Storage, callback func(context.Context, string) string, issuerInterceptor *op.IssuerInterceptor) *login {
	l := &login{
		store:    store,
		callback: callback,
	}
	l.createRouter(issuerInterceptor)
	return l
}

func (l *login) createRouter(issuerInterceptor *op.IssuerInterceptor) {
	l.router = mux.NewRouter()
	l.router.Path("/auth-queries").Methods("GET").HandlerFunc(l.listAuthQueries)
	l.router.Path("/password").Methods("GET").HandlerFunc(l.loginHandler)
	l.router.Path("/password").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/username").Methods("GET").HandlerFunc(l.loginHandler)
	l.router.Path("/username").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/cert").Methods("GET").HandlerFunc(issuerInterceptor.HandlerFunc(l.genericHandler))
	l.router.Path("/cert").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/ext-jwt").Methods("GET").HandlerFunc(issuerInterceptor.HandlerFunc(l.genericHandler))
	l.router.Path("/ext-jwt").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/totp").Methods("POST").HandlerFunc(l.checkTotp)
	l.router.Path("/totp/enroll").Methods("POST").HandlerFunc(l.startEnrollTotp)
	l.router.Path("/totp/enroll").Methods("DELETE").HandlerFunc(l.deleteEnrollTotp)
	l.router.Path("/totp/enroll/verify").Methods("POST").HandlerFunc(l.verifyTotp)
}

func (l *login) genericHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
		return
	}

	id := r.FormValue(queryAuthRequestID)
	w.Header().Set(AuthRequestIdHeader, id)
	_, _ = w.Write([]byte("please POST to this URL"))
}

func (l *login) loginHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
		return
	}

	id := r.FormValue(queryAuthRequestID)
	w.Header().Set(AuthRequestIdHeader, id)
	renderLogin(w, id, nil)
}

func renderLogin(w http.ResponseWriter, id string, err error) {
	renderPage(w, loginTemplate, id, err, nil)
}

func renderTotp(w http.ResponseWriter, id string, err error, additionalData any) {
	renderPage(w, totpTemplate, id, err, additionalData)
}

func renderPage(w http.ResponseWriter, pageTemplate *template.Template, id string, err error, additionalData any) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	var errMsg string
	errDisplay := "none"
	if err != nil {
		errMsg = err.Error()
		errDisplay = "block"
	}
	data := &struct {
		ID             string
		Error          string
		ErrorDisplay   string
		AdditionalData any
	}{
		ID:             id,
		Error:          errMsg,
		ErrorDisplay:   errDisplay,
		AdditionalData: additionalData,
	}

	templateErr := pageTemplate.Execute(w, data)
	if templateErr != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (l *login) checkTotp(w http.ResponseWriter, r *http.Request) {
	responseType, err := negotiateResponseContentType(r)

	if err != nil {
		renderJsonApiError(w, err)
		return
	}

	bodyContentType, err := negotiateBodyContentType(r)

	if err != nil {
		if responseType == JsonContentType {
			renderJsonApiError(w, err)
			return
		}
		http.Error(w, fmt.Sprintf("cannot process body content type: %s", err), http.StatusBadRequest)
		return
	}

	id := ""
	code := ""
	if bodyContentType == FormContentType {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusBadRequest)
			return
		}
		id = r.FormValue("id")
		code = r.FormValue("code")
	} else if bodyContentType == JsonContentType {
		payload := &TotpRequestBody{}
		body, err := io.ReadAll(r.Body)

		if err != nil {
			renderJsonError(w, err)
			return
		}
		err = json.Unmarshal(body, payload)

		if err != nil {
			renderJsonError(w, err)
			return
		}

		id = payload.GetAuthRequestId()
		code = payload.Code
	}

	ctx := NewHttpChangeCtx(r)
	authRequest, verifyErr := l.store.VerifyTotp(ctx, code, id)

	if verifyErr != nil {
		if responseType == JsonContentType {
			renderJsonApiError(w, &errorz.ApiError{
				Code:    "INVALID TOTP CODE",
				Message: "an invalid TOTP code was supplied",
				Status:  http.StatusBadRequest,
			})
			return
		} else {
			renderTotp(w, id, verifyErr, nil)
			return
		}
	}

	if !authRequest.HasAmr(AuthMethodSecondaryTotp) {
		renderTotp(w, id, errors.New("TOTP supplied but not enabled or required on identity"), nil)
	}

	callbackUrl := l.callback(r.Context(), id)
	http.Redirect(w, r, callbackUrl, http.StatusFound)
}

func (l *login) authenticate(w http.ResponseWriter, r *http.Request) {
	responseType, err := negotiateResponseContentType(r)

	if err != nil {
		renderJsonError(w, err)
	}

	pathSplits := strings.Split(r.URL.Path, "/")

	if len(pathSplits) == 0 {
		http.Error(w, "invalid login path, could not find auth method", http.StatusBadRequest)
		return
	}

	method := pathSplits[len(pathSplits)-1]

	//patch username from standard OIDC auth URIs
	if method == "username" {
		method = AuthMethodPassword
	}

	credentials := &OidcUpdbCreds{}
	apiErr := parsePayload(r, credentials)

	if apiErr != nil {
		renderJsonError(w, apiErr)
		return
	}

	authCtx := model.NewAuthContextHttp(r, method, credentials, NewHttpChangeCtx(r))

	authRequest, apiErr := l.store.Authenticate(authCtx, credentials.AuthRequestId, credentials.ConfigTypes)

	if apiErr != nil {
		invalid := apierror.NewInvalidAuth()
		if method == AuthMethodPassword {
			renderLogin(w, credentials.AuthRequestId, invalid)
			w.WriteHeader(invalid.Status)
			return
		}

		http.Error(w, invalid.Message, invalid.Status)
		return
	}

	authRequest.SdkInfo = credentials.SdkInfo
	authRequest.EnvInfo = credentials.EnvInfo
	authRequest.AuthTime = time.Now()

	var authQueries []*rest_model.AuthQueryDetail

	if !authRequest.HasSecondaryAuth() {
		authQueries = authRequest.GetAuthQueries()
	}

	if authRequest.NeedsTotp() {
		w.Header().Set(TotpRequiredHeader, "true")
	}

	if len(authQueries) > 0 {

		if responseType == HtmlContentType {
			renderTotp(w, credentials.AuthRequestId, err, authQueries)
		} else if responseType == JsonContentType {
			respBody := JsonMap(map[string]interface{}{
				"authQueries": authQueries,
			})
			renderJson(w, http.StatusOK, &respBody)
		}

		return
	}

	apiErr = l.store.UpdateSdkEnvInfo(authRequest)

	if apiErr != nil {
		pfxlog.Logger().WithError(apiErr).Errorf("cannot update sdk env info, continuing with authentication")
	}

	callbackUrl := l.callback(r.Context(), credentials.AuthRequestId)
	http.Redirect(w, r, callbackUrl, http.StatusFound)
}

func (l *login) listAuthQueries(w http.ResponseWriter, r *http.Request) {
	authRequestId := r.URL.Query().Get("id")

	authRequest, err := l.store.GetAuthRequest(authRequestId)

	if err != nil {
		invalid := apierror.NewInvalidAuth()
		http.Error(w, invalid.Message, invalid.Status)
		return
	}

	var authQueries []*rest_model.AuthQueryDetail

	if !authRequest.HasSecondaryAuth() {
		authQueries = authRequest.GetAuthQueries()
	}

	if authRequest.NeedsTotp() {
		w.Header().Set(TotpRequiredHeader, "true")
	}

	respBody := JsonMap(map[string]interface{}{
		"authQueries": authQueries,
	})
	renderJson(w, http.StatusOK, &respBody)
}

type JsonMap map[string]any

func (m *JsonMap) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return jsonutils.WriteJSON(m)
}

func (l *login) deleteEnrollTotp(w http.ResponseWriter, r *http.Request) {
	changeCtx := NewHttpChangeCtx(r)

	_, err := negotiateResponseContentType(r)

	if err != nil {
		renderJsonError(w, err)
		return
	}

	payload := &TotpRequestBody{}
	apiErr := parsePayload(r, payload)

	if apiErr != nil {
		renderJsonError(w, apiErr)
		return
	}

	apiErr = l.store.DeleteTotpEnrollment(changeCtx, payload.AuthRequestId, payload.Code)

	if apiErr != nil {
		renderJsonError(w, apiErr)
		return
	}

	renderJson(w, http.StatusOK, &rest_model.Empty{})
}

func (l *login) startEnrollTotp(w http.ResponseWriter, r *http.Request) {
	changeCtx := NewHttpChangeCtx(r)

	_, err := negotiateResponseContentType(r)

	if err != nil {
		renderJsonError(w, err)
		return
	}

	payload := &AuthRequestBody{}
	apiErr := parsePayload(r, payload)

	if apiErr != nil {
		renderJsonError(w, apiErr)
		return
	}

	mfaDetail, totpStartErr := l.store.StartTotpEnrollment(changeCtx, payload.AuthRequestId)

	if totpStartErr != nil {
		renderJsonError(w, totpStartErr)
		return
	}

	renderJson(w, http.StatusCreated, &rest_model.DetailMfa{
		BaseEntity: rest_model.BaseEntity{
			CreatedAt: ToPtr(strfmt.DateTime(mfaDetail.CreatedAt)),
			ID:        ToPtr(mfaDetail.Id),
			UpdatedAt: ToPtr(strfmt.DateTime(mfaDetail.UpdatedAt)),
		},
		IsVerified:      ToPtr(false),
		ProvisioningURL: l.store.Managers().Mfa.GetProvisioningUrl(mfaDetail),
		RecoveryCodes:   mfaDetail.RecoveryCodes,
	})
}

func ToPtr[T any](v T) *T {
	return &v
}

func (l *login) verifyTotp(w http.ResponseWriter, r *http.Request) {
	changeCtx := NewHttpChangeCtx(r)

	_, err := negotiateResponseContentType(r)

	if err != nil {
		renderJsonError(w, err)
		return
	}

	payload := &TotpRequestBody{}
	apiErr := parsePayload(r, payload)

	if apiErr != nil {
		renderJsonError(w, err)
		return
	}

	apiErr = l.store.CompleteTotpEnrollment(changeCtx, payload.AuthRequestId, payload.Code)

	if apiErr != nil {
		renderJsonError(w, apiErr)
		return
	}

	renderJson(w, http.StatusOK, &rest_model.Empty{})
}
