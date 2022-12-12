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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/blang/semver"
	openApiRuntime "github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge/rest_management_api_client"
	"github.com/openziti/edge/rest_model"
	fabric_rest_client "github.com/openziti/fabric/rest_client"
	"github.com/openziti/ziti/common/version"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"gopkg.in/resty.v1"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Download a file from the given URL
func DownloadFile(filepath string, url string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// make it executable
	err = os.Chmod(filepath, 0755)

	if err != nil {
		return err
	}
	return nil
}

// Use a 2-second timeout with a retry count of 5
func newClient() *resty.Client {
	return resty.
		New().
		SetTimeout(2 * time.Second).
		SetRetryCount(5).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
}

func getRequest(verbose bool) *resty.Request {
	return newClient().
		SetDebug(verbose).
		R()
}

// DownloadArtifactoryFile will download a file from the given Artifactory URL
func DownloadArtifactoryFile(url string, filepath string) (err error) {
	fullUrl := "https://netfoundry.jfrog.io/netfoundry/" + url
	resp, err := getRequest(false).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetOutput(filepath).
		Get(fullUrl)

	if err != nil {
		return fmt.Errorf("unable to download '%s', %s", url, err)
	}

	if resp.IsError() {
		return fmt.Errorf("unable to download file, error HTTP status code [%d] returned for url [%s]", resp.StatusCode(), fullUrl)
	}

	return nil
}

// Used to parse the 'get-object-tagging' response
type Data struct {
	TagSet []struct {
		Value string `json:"Value"`
		Key   string `json:"Key"`
	}
}

// Used to parse the '/api/versions' response from Artifactory
type ArtifactoryVersionsData struct {
	Version   string `json:"version"`
	Artifacts []struct {
	}
}

func GetLatestVersionFromArtifactory(verbose bool, staging bool, branch string, appName string) (semver.Version, error) {
	resp, err := getRequest(verbose).
		SetQueryParams(map[string]string{
			//   "key": "value",
		}).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryVersionsData{}).
		Get("https://netfoundry.jfrog.io/netfoundry/api/versions/" + GetArtifactoryPath(staging, appName, branch))

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s'; %s", appName, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; Not Found", appName, branch)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; %s", appName, branch, resp.Status())
	}

	result := *resp.Result().(*ArtifactoryVersionsData)

	return semver.Make(strings.TrimPrefix(result.Version, "v"))
}

// Used to parse the '/releases/latest' response from GitHub
type GitHubReleasesData struct {
	Version string `json:"tag_name"`
	SemVer  semver.Version
	Assets  []struct {
		BrowserDownloadURL string `json:"browser_download_url"`
	}
}

func (self *GitHubReleasesData) GetDownloadUrl(appName string) (string, error) {
	arches := []string{runtime.GOARCH}
	if strings.ToLower(runtime.GOARCH) == "amd64" {
		arches = append(arches, "x86_64")
	}

	for _, asset := range self.Assets {
		ok := false
		for _, arch := range arches {
			if strings.Contains(strings.ToLower(asset.BrowserDownloadURL), arch) {
				ok = true
			}
		}

		ok = ok && strings.Contains(strings.ToLower(asset.BrowserDownloadURL), runtime.GOOS)
		if ok {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", errors.Errorf("no download URL found for os/arch %v/%v for '%v'", runtime.GOOS, runtime.GOARCH, appName)
}

func GetHighestVersionGitHubReleaseInfo(verbose bool, appName string) (*GitHubReleasesData, error) {
	resp, err := getRequest(verbose).
		SetQueryParams(map[string]string{}).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetResult([]*GitHubReleasesData{}).
		Get("https://api.github.com/repos/openziti/" + appName + "/releases")

	if err != nil {
		return nil, errors.Wrapf(err, "unable to get latest version for '%s'", appName)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, errors.Errorf("unable to get latest version for '%s'; Not Found (invalid URL)", appName)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.Errorf("unable to get latest version for '%s'; return status=%s", appName, resp.Status())
	}

	result := *resp.Result().(*[]*GitHubReleasesData)
	return getHighestVersionRelease(appName, result)
}

func getHighestVersionRelease(appName string, releases []*GitHubReleasesData) (*GitHubReleasesData, error) {
	for _, release := range releases {
		v, err := semver.ParseTolerant(release.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse version %v for '%v'", release.Version, appName)
		}
		release.SemVer = v
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].SemVer.GT(releases[j].SemVer) // sort in reverse order
	})
	if len(releases) == 0 {
		return nil, errors.Errorf("no releases found for '%v'", appName)
	}
	return releases[0], nil
}

func GetLatestGitHubReleaseAsset(verbose bool, appName string) (*GitHubReleasesData, error) {
	resp, err := getRequest(verbose).
		SetQueryParams(map[string]string{}).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetResult(&GitHubReleasesData{}).
		Get("https://api.github.com/repos/openziti/" + appName + "/releases/latest")

	if err != nil {
		return nil, fmt.Errorf("unable to get latest version for '%s'; %s", appName, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("unable to get latest version for '%s'; Not Found", appName)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unable to get latest version for '%s'; %s", appName, resp.Status())
	}

	result := resp.Result().(*GitHubReleasesData)
	return result, nil
}

// DownloadGitHubReleaseAsset will download a file from the given GitHUb release area
func DownloadGitHubReleaseAsset(fullUrl string, filepath string) (err error) {
	resp, err := getRequest(false).
		SetOutput(filepath).
		Get(fullUrl)

	if err != nil {
		return fmt.Errorf("unable to download '%s', %s", fullUrl, err)
	}

	if resp.IsError() {
		return fmt.Errorf("unable to download file, error HTTP status code [%d] returned for url [%s]", resp.StatusCode(), fullUrl)
	}

	return nil
}

// Used to parse the '/api/search/aql' response from Artifactory
type AQLResult struct {
	Repo       string
	Path       string
	Name       string
	Type       string
	Size       int
	Created    string
	CreatedBy  string
	Modified   string
	ModifiedBy string
	Updated    string
	Properties []struct {
		Key   string
		Value string
	}
}
type ArtifactoryAQLData struct {
	Results []AQLResult
}

type AQLVars struct {
	SemverMajor    string
	SemverMaxMinor string
	SemverMinMinor string
	App            string
	Arch           string
	OS             string
}

func GetLatestSemanticVersionFromArtifactory(verbose bool, staging bool, branch string, appName string, versionWildcard string) (semver.Version, error) {
	sv := strings.Split(versionWildcard, ".")
	minor, err := strconv.Atoi(sv[1])
	if err != nil {
		panic(err)
	}
	maxMinor := minor + 1
	aqlVars := AQLVars{sv[0], strconv.Itoa(maxMinor), sv[1], appName, runtime.GOARCH, runtime.GOOS}
	tpl, err := template.New("aql").Parse("items.find( { \"@build.number\":{\"$lt\":\"{{ .SemverMajor}}.{{ .SemverMaxMinor}}.*\"}, \"@build.number\":{\"$gt\":\"{{ .SemverMajor}}.{{ .SemverMinMinor}}.*\"}, \"repo\":{\"$match\":\"ziti-release\"}, \"path\":{\"$match\":\"{{ .App}}/{{ .Arch}}/{{ .OS}}/*\"} } ).include(\"@build.number\") ")
	if err != nil {
		panic(err)
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, aqlVars)
	if err != nil {
		panic(err)
	}
	resp, err := getRequest(verbose).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetHeader("Content-Type", "text/plain").
		SetBody(body.String()).
		SetResult(&ArtifactoryAQLData{}).
		Post("https://netfoundry.jfrog.io/netfoundry/api/search/aql")

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s'; %s", appName, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; Not Found", appName, branch)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; %s", appName, branch, resp.Status())
	}

	aqlData := (*resp.Result().(*ArtifactoryAQLData))

	latestSemVer, _ := semver.Make("0.0.0")

	for _, result := range aqlData.Results {
		sv, err := semver.Make(result.Properties[0].Value)
		if err != nil {
			panic(err)
		}
		if sv.GT(latestSemVer) {
			latestSemVer = sv
		}
	}

	return latestSemVer, nil
}

func GetLatestTerraformProviderVersionFromArtifactory(branch string, provider string) (semver.Version, error) {
	repoUrl := "https://netfoundry.jfrog.io/netfoundry/api/versions/" + GetTerraformProviderArtifactoryPath(provider, branch)
	resp, err := getRequest(false).
		SetQueryParams(map[string]string{
			//   "key": "value",
		}).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryVersionsData{}).
		Get(repoUrl)

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on repo url %s; %s", provider, repoUrl, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s' on repo url %s; Not Found", provider, branch, repoUrl)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s' on repo url %s; %s", provider, branch, repoUrl, resp.Status())
	}

	result := (*resp.Result().(*ArtifactoryVersionsData))

	return semver.Make(strings.TrimPrefix(result.Version, "v"))
}

// Used to parse the '/api/storage' response from Artifactory
type ArtifactoryStorageChildrenData struct {
	URI    string `json:"uri"`
	Folder bool   `json:"folder"`
}
type ArtifactoryStorageData struct {
	Repo         string                           `json:"repo"`
	Path         string                           `json:"path"`
	Created      string                           `json:"created"`
	LastModified string                           `json:"lastModified"`
	LastUpdated  string                           `json:"lastUpdated"`
	Children     []ArtifactoryStorageChildrenData `json:"children"`
	URI          string                           `json:"uri"`
}

func GetCurrentSnapshotListFromArtifactory(verbose bool) ([]ArtifactoryStorageChildrenData, error) {
	resp, err := getRequest(verbose).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryStorageData{}).
		Get("https://netfoundry.jfrog.io/netfoundry/api/storage/ziti-snapshot/")

	if err != nil {
		return nil, fmt.Errorf("unable to get list of branches; %s", err)
	}

	result := (*resp.Result().(*ArtifactoryStorageData))

	return result.Children, nil
}

func GetArtifactoryPath(staging bool, appName string, branch string) string {
	if branch == "" {
		branch = version.GetBranch()
	}

	arch := runtime.GOARCH
	os := runtime.GOOS

	var path string
	if staging {
		path = "ziti-staging/"
	} else if branch == "main" {
		path = "ziti-release/"
	} else {
		path = "ziti-snapshot/" + branch + "/"
	}
	// Special-case the source-repo when dealing with ziti-prox-c
	if branch == "main" && appName == c.ZITI_PROX_C {
		path = "ziti-staging/"
	}

	path += appName + "/" + arch + "/" + os

	return path
}

func GetTerraformProviderArtifactoryPath(provider string, branch string) string {
	if branch == "" {
		branch = "master"
	}
	var path string
	if branch == "master" {
		path = c.TERRAFORM_PROVIDER_PREFIX + provider + "-release/"
	} else {
		path = c.TERRAFORM_PROVIDER_PREFIX + provider + "-snapshot/" + branch + "/"
	}
	path += c.TERRAFORM_PROVIDER_PREFIX + provider + "/" + version.GetArchitecture() + "/" + version.GetOS()

	return path
}

// untargz a tarball to a target, from
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip
func UnTargz(tarball, target string, onlyFiles []string) error {
	zreader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer zreader.Close()

	reader, err := gzip.NewReader(zreader)
	defer func() {
		_ = reader.Close()
	}()

	if err != nil {
		panic(err)
	}

	tarReader := tar.NewReader(reader)

	for {
		inkey := false
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		for _, value := range onlyFiles {
			if value == "*" || value == path.Base(header.Name) {
				inkey = true
				break
			}
		}

		if !inkey {
			continue
		}

		path := filepath.Join(target, path.Base(header.Name))
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

// EdgeControllerLogin will authenticate to the given Edge Controller
func EdgeControllerLogin(url string, cert string, authentication string, out io.Writer, logJSON bool, timeout int, verbose bool) (*gabs.Container, error) {
	client := newClient()

	if cert != "" {
		client.SetRootCertificate(cert)
	}

	resp, err := client.
		SetTimeout(time.Duration(time.Duration(timeout)*time.Second)).
		SetDebug(verbose).
		R().
		SetQueryParam("method", "password").
		SetHeader("Content-Type", "application/json").
		SetBody(authentication).
		Post(url + "/authenticate")

	if err != nil {
		return nil, fmt.Errorf("unable to authenticate to %v. Error: %v", url, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unable to authenticate to %v. Status code: %v, Server returned: %v", url, resp.Status(), prettyPrintResponse(resp))
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())
	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", url, resp.String())
	}

	return jsonParsed, nil
}

func prettyPrintResponse(resp *resty.Response) string {
	out := resp.String()
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(out), "", "    "); err == nil {
		return prettyJSON.String()
	}
	return out
}

func outputJson(out io.Writer, data []byte) {
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
			queryUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logJSON {
		outputJson(out, resp.Body())
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
			queryUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logJSON {
		outputJson(out, resp.Body())
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
			return fmt.Sprintf("could not read API error, payload.error was nil: %v", a.Error())
		}
		return formatApiError(payload.Error)
	}

	return fmt.Sprintf("could not read API error, payload was nil: %v", a.Error())
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
		outputJson(out, []byte(body))
		fmt.Println()
	}

	resp, err := req.SetBody(body).Post(url)

	if err != nil {
		return nil, fmt.Errorf("unable to create %v instance in Ziti Edge Controller at %v. Error: %v", entityType, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("error creating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, baseUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logResponseJson {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", baseUrl, resp.String())
	}

	return jsonParsed, nil
}

// ControllerDelete will delete entities of the given type in the given Controller
func ControllerDelete(api API, entityType string, id string, body string, out io.Writer, logRequestJson bool, logResponseJson bool, timeout int, verbose bool) error {
	restClientIdentity, err := LoadSelectedRWIdentityForApi(api)
	if err != nil {
		return err
	}

	baseUrl, err := restClientIdentity.GetBaseUrlForApi(api)
	if err != nil {
		return err
	}

	req, err := NewRequest(restClientIdentity, timeout, verbose)
	if err != nil {
		return err
	}

	entityPath := entityType + "/" + id
	fullUrl := baseUrl + "/" + entityPath

	if logRequestJson {
		fmt.Printf("%v to %v\n", "POST", fullUrl)
		outputJson(out, []byte(body))
		fmt.Println()
	}

	if body != "" {
		req = req.SetBody(body)
	}

	resp, err := req.Delete(fullUrl)

	if err != nil {
		return fmt.Errorf("unable to delete %v instance in Ziti Edge Controller at %v. Error: %v", entityPath, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error deleting %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityPath, baseUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logResponseJson {
		outputJson(out, resp.Body())
	}

	return nil
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
		outputJson(out, []byte(body))
		fmt.Println()
	}

	resp, err := req.SetBody(body).Execute(method, url)

	if err != nil {
		return nil, fmt.Errorf("unable to update %v instance in Ziti Edge Controller at %v. Error: %v", entityType, baseUrl, err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusAccepted {
		return nil, fmt.Errorf("error updating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, baseUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logResponseJSON {
		outputJson(out, resp.Body())
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
			entityType, id, baseUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logJSON {
		outputJson(out, resp.Body())
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
			request.Method, entityType, baseUrl, resp.Status(), prettyPrintResponse(resp))
	}

	if logJSON {
		outputJson(out, resp.Body())
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
	client := newClient()

	client.SetHostURL(host)

	if cert != "" {
		client.SetRootCertificate(cert)
	}

	resp, err := client.R().Get("/version")

	if err != nil || resp.StatusCode() != http.StatusOK {
		return host
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
