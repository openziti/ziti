package oidc_auth

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/openziti/edge-api/rest_model"
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
)

const ()

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
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
		return
	}
	id := r.FormValue("id")
	code := r.FormValue("code")

	ctx := NewHttpChangeCtx(r)
	authRequest, err := l.store.VerifyTotp(ctx, code, id)

	if err != nil {
		renderTotp(w, id, err)
		return
	}

	if !authRequest.HasAmr(AuthMethodSecondaryTotp) {
		renderTotp(w, id, errors.New("invalid TOTP code"))
	}

	http.Redirect(w, r, l.callback(r.Context(), id), http.StatusFound)
}

type updbCreds struct {
	rest_model.Authenticate
	AuthRequestId string `json:"id"`
}

func (l *login) parseUpdbForm(r *http.Request) (*updbCreds, error) {
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

	contentType := r.Header.Get("content-type")

	creds := &updbCreds{}

	if contentType == "application/x-www-form-urlencoded" {
		var err error
		creds, err = l.parseUpdbForm(r)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

	} else if strings.Contains(contentType, "json") {
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
	}

	if creds.AuthRequestId == "" {
		creds.AuthRequestId = r.URL.Query().Get("id")
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
		renderTotp(w, creds.AuthRequestId, err)
		return
	}

	authRequest.AuthTime = time.Now()
	authRequest.SdkInfo = creds.SdkInfo
	authRequest.EnvInfo = creds.EnvInfo

	callbackUrl := l.callback(r.Context(), creds.AuthRequestId)
	http.Redirect(w, r, callbackUrl, http.StatusFound)
}
