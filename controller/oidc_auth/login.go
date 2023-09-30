package oidc_auth

import (
	"context"
	"embed"
	"fmt"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"github.com/zitadel/oidc/v2/pkg/op"
	"html/template"
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
	l.router.Path("/cert").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))
	l.router.Path("/ext-jwt").Methods("POST").HandlerFunc(issuerInterceptor.HandlerFunc(l.authenticate))
	l.router.Path("/totp").Methods("POST").HandlerFunc(l.checkTotp)
}

func (l *login) loginHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
		return
	}

	renderLogin(w, r.FormValue(queryAuthRequestID), nil)
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

func (l *login) authenticate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot parse form:%s", err), http.StatusInternalServerError)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	configTypes := r.Form["configTypes"]

	id := r.FormValue("id")

	if id == "" {
		id = r.URL.Query().Get("id")
	}

	pathSplits := strings.Split(r.URL.Path, "/")

	if len(pathSplits) == 0 {
		http.Error(w, "invalid login path, could not find auth method", http.StatusBadRequest)
		return
	}

	method := pathSplits[len(pathSplits)-1]

	if method == "username" {
		method = AuthMethodPassword
	}

	authCtx := model.NewAuthContextHttp(r, method, map[string]any{
		"username":    username,
		"password":    password,
		"configTypes": configTypes,
	}, NewHttpChangeCtx(r))

	authRequest, err := l.store.Authenticate(authCtx, id, configTypes)

	if err != nil {
		invalid := apierror.NewInvalidAuth()
		if method == AuthMethodPassword {
			renderLogin(w, id, invalid)
			return
		}

		http.Error(w, invalid.Message, invalid.Status)
		return
	}

	if authRequest.SecondaryTotpRequired && !authRequest.HasAmr(AuthMethodSecondaryTotp) {
		renderTotp(w, id, err)
		return
	}

	authRequest.AuthTime = time.Now()

	http.Redirect(w, r, l.callback(r.Context(), id), http.StatusFound)
}
