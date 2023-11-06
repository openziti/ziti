package oidc_auth

import (
	"context"
	"embed"
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"github.com/zitadel/oidc/v2/pkg/op"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const (
	queryAuthRequestID = "authRequestID"

	//page specific resource keys
	pageLogin = "login"
	pageTotp  = "totp"

	//method specific login URLs
	passwordLoginUrl = "/oidc/login/username?authRequestID="
	certLoginUrl     = "/oidc/login/cert?authRequestID="
	extJwtLoginUrl   = "/oidc/login/ext-jwt?authRequestID="

	AuthRequestIdHeader = "auth-request-id"
	AcceptHeader        = "accept"
	TotpRequiredHeader  = "totp-required"
	ContentTypeHeader   = "content-type"

	FormContentType = "application/x-www-form-urlencoded"
	JsonContentType = "application/json"
	HtmlContentType = "text/html"
)

type totpCode struct {
	rest_model.MfaCode
	AuthRequestId string `json:"id"`
}

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
	l.router.Path("/username").Methods("GET").HandlerFunc(l.loginHandler)
	l.router.Path("/username").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/cert").Methods("GET").HandlerFunc(issuerInterceptor.HandlerFunc(l.genericHandler))
	l.router.Path("/cert").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/ext-jwt").Methods("GET").HandlerFunc(issuerInterceptor.HandlerFunc(l.genericHandler))
	l.router.Path("/ext-jwt").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))

	l.router.Path("/totp").Methods("POST").HandlerFunc(l.checkTotp)
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
	renderPage(w, loginTemplate, id, err)
}

func renderTotp(w http.ResponseWriter, id string, err error) {
	renderPage(w, totpTemplate, id, err)
}

func renderPage(w http.ResponseWriter, pageTemplate *template.Template, id string, err error) {
	var errMsg string
	errDisplay := "none"
	if err != nil {
		errMsg = err.Error()
		errDisplay = "block"
	}
	data := &struct {
		ID           string
		Error        string
		ErrorDisplay string
	}{
		ID:           id,
		Error:        errMsg,
		ErrorDisplay: errDisplay,
	}

	err = pageTemplate.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (l *login) checkTotp(w http.ResponseWriter, r *http.Request) {
	responseType, err := negotiateContentType(r)

	if err != nil {
		safeRenderJson(w, http.StatusNotAcceptable, &rest_model.APIError{
			Code:    "NOT_ACCEPTABLE",
			Message: err.Error(),
		})

		return
	}

	contentType := r.Header.Get(ContentTypeHeader)
	id := ""
	code := ""
	if contentType == FormContentType {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
			return
		}
		id = r.FormValue("id")
		code = r.FormValue("code")
	} else if contentType == JsonContentType {
		payload := &totpCode{}
		body, err := io.ReadAll(r.Body)

		if err != nil {
			safeRenderJson(w, http.StatusInternalServerError, &rest_model.APIError{
				Code:    "INTERNAL_ERROR",
				Message: err.Error(),
			})
			return
		}
		err = json.Unmarshal(body, payload)

		if err != nil {
			safeRenderJson(w, http.StatusInternalServerError, &rest_model.APIError{
				Code:    "INTERNAL_ERROR",
				Message: err.Error(),
			})
			return
		}

		id = payload.AuthRequestId
		code = stringz.OrEmpty(payload.Code)
	}

	ctx := NewHttpChangeCtx(r)
	authRequest, err := l.store.VerifyTotp(ctx, code, id)

	if err != nil {
		renderTotp(w, id, err)
		return
	}

	if !authRequest.HasAmr(AuthMethodSecondaryTotp) {
		renderTotp(w, id, errors.New("invalid TOTP code"))
	}

	if responseType == HtmlContentType {
		http.Redirect(w, r, l.callback(r.Context(), id), http.StatusFound)
	}

	safeRenderJson(w, http.StatusOK, &rest_model.Empty{})
}

type updbCreds struct {
	rest_model.Authenticate
	AuthRequestId string `json:"id"`
}

func (l *login) parseFormData(r *http.Request) (*updbCreds, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, fmt.Errorf("cannot parse form:%s", err)
	}

	result := &updbCreds{
		Authenticate: rest_model.Authenticate{
			EnvInfo: &rest_model.EnvInfo{
				Arch:      r.FormValue("envArch"),
				Os:        r.FormValue("envOs"),
				OsRelease: r.FormValue("envOsRelease"),
				OsVersion: r.FormValue("envOsVersion"),
			},
			SdkInfo: &rest_model.SdkInfo{
				AppID:      r.FormValue("sdkAppId"),
				AppVersion: r.FormValue("sdkAppVersion"),
				Branch:     r.FormValue("sdkBranch"),
				Revision:   r.FormValue("sdkRevision"),
				Type:       r.FormValue("sdkType"),
				Version:    r.FormValue("sdkVersion"),
			},
			Username:    rest_model.Username(r.FormValue("username")),
			Password:    rest_model.Password(r.FormValue("password")),
			ConfigTypes: r.Form["configTypes"],
		},
		AuthRequestId: r.FormValue("id"),
	}

	return result, nil
}

func (l *login) authenticate(w http.ResponseWriter, r *http.Request) {
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

	var authCtx model.AuthContext

	contentType := r.Header.Get(ContentTypeHeader)
	creds := &updbCreds{}

	if contentType == FormContentType {
		var err error
		creds, err = l.parseFormData(r)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

	} else if contentType == FormContentType {
		body, err := io.ReadAll(r.Body)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = json.Unmarshal(body, creds)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, fmt.Sprintf("invalid content type: %s,", contentType), http.StatusUnsupportedMediaType)
	}

	if creds.AuthRequestId == "" {
		creds.AuthRequestId = r.URL.Query().Get("id")
	}

	responseType, err := negotiateContentType(r)

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotAcceptable)
	}

	authCtx = model.NewAuthContextHttp(r, method, creds, NewHttpChangeCtx(r))

	authRequest, err := l.store.Authenticate(authCtx, creds.AuthRequestId, creds.ConfigTypes)

	if err != nil {
		invalid := apierror.NewInvalidAuth()
		if method == AuthMethodPassword {
			renderLogin(w, creds.AuthRequestId, invalid)
			return
		}

		http.Error(w, invalid.Message, invalid.Status)
		return
	}

	if authRequest.SecondaryTotpRequired && !authRequest.HasAmr(AuthMethodSecondaryTotp) {
		w.Header().Set(TotpRequiredHeader, "true")
		if responseType == HtmlContentType {
			renderTotp(w, creds.AuthRequestId, err)
		} else if responseType == JsonContentType {
			safeRenderJson(w, http.StatusOK, &rest_model.Empty{})
		}

		return
	}

	authRequest.AuthTime = time.Now()
	authRequest.SdkInfo = creds.SdkInfo
	authRequest.EnvInfo = creds.EnvInfo

	callbackUrl := l.callback(r.Context(), creds.AuthRequestId)
	http.Redirect(w, r, callbackUrl, http.StatusFound)
}

// renderJson will attempt to marshal the data argument. If marshalling fails true and an error is returned
// without altering the http.ResponseWriter. If writing to the http.ResponseWriter fails, false and an error
// is returned. The boolean return signals the http.ResponseWriter is sill "writable".
func renderJson(w http.ResponseWriter, status int, data encoding.BinaryMarshaler) (bool, error) {
	payload, err := data.MarshalBinary()

	if err != nil {
		return true, err
	}

	w.Header().Set(ContentTypeHeader, JsonContentType)
	w.WriteHeader(status)
	_, err = w.Write(payload)

	return false, err
}

// safeRenderJson will attempt to call renderJson to return an error. If it fails, false will be returned.
// If possible, an error is written to the HTTP response. Otherwise, true is returned.
func safeRenderJson(w http.ResponseWriter, status int, data encoding.BinaryMarshaler) bool {
	canStillWrite, err := renderJson(w, http.StatusOK, &rest_model.Empty{})
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal to JSON")
		if canStillWrite {
			_, internalErr := renderJson(w, http.StatusInternalServerError, &rest_model.APIError{
				Code:    errorz.UnhandledCode,
				Message: fmt.Sprintf("could not marhsal to JSON: %s", err),
			})

			if internalErr != nil {
				pfxlog.Logger().WithError(internalErr).Errorf("could not write JSON marshaling error to HTTP response")
			}

			return false
		}
	}

	return true
}

// parseAcceptHeader parses HTTP accept headers and returns an array of supported
// content types sorted by quality factor (0=most desired response type). The return
// strings are the content type only (e.g. "application/json")
func parseAcceptHeader(acceptHeader string) []string {
	parts := strings.Split(acceptHeader, ",")
	contentTypes := make([]string, len(parts))

	for i, part := range parts {
		typeAndFactor := strings.Split(strings.TrimSpace(part), ";")
		contentTypes[i] = typeAndFactor[0]
	}

	return contentTypes
}

// negotiateContentType returns the response content type that should be
// used based on the results of parseAcceptHeader. If the accept header
// cannot be satisfied an error is returned.
func negotiateContentType(r *http.Request) (string, error) {
	acceptHeader := r.Header.Get(AcceptHeader)
	contentTypes := parseAcceptHeader(acceptHeader)

	if len(contentTypes) == 0 || acceptHeader == "" {
		return JsonContentType, nil
	}

	for _, contentType := range contentTypes {
		if contentType == JsonContentType {
			return contentType, nil
		} else if contentType == HtmlContentType {
			return HtmlContentType, nil
		}
	}

	return "", fmt.Errorf("unable to satisfy accept header provided: %s. Supported headers include %s and %s", acceptHeader, JsonContentType, HtmlContentType)
}
