package getziti

import (
	"context"
	"fmt"
	"github.com/blang/semver"
	"github.com/go-resty/resty/v2"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/versions"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GitHubReleasesData is used to parse the '/releases/latest' response from GitHub
type GitHubReleasesData struct {
	Version string `json:"tag_name"`
	SemVer  semver.Version
	Assets  []struct {
		BrowserDownloadURL string `json:"browser_download_url"`
	}
}

func (self *GitHubReleasesData) GetDownloadUrl(appName string, targetOS, targetArch string) (string, error) {
	arches := []string{targetArch}
	if strings.ToLower(targetArch) == "amd64" {
		arches = append(arches, "x86_64")
	}

	for _, asset := range self.Assets {
		ok := false
		for _, arch := range arches {
			if strings.Contains(strings.ToLower(asset.BrowserDownloadURL), arch) {
				ok = true
			}
		}

		ok = ok && strings.Contains(strings.ToLower(asset.BrowserDownloadURL), targetOS)
		if ok {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", errors.Errorf("no download URL found for os/arch %s/%s for '%s'", targetOS, targetArch, appName)
}

func NewClient() *resty.Client {
	// Use a 2-second timeout with a retry count of 5
	return resty.
		New().
		SetTimeout(2 * time.Second).
		SetRetryCount(5).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
}

func getRequest(verbose bool) *resty.Request {
	return NewClient().
		SetDebug(verbose).
		R()
}

func GetLatestGitHubReleaseVersion(zitiApp string, verbose bool) (semver.Version, error) {
	var result semver.Version
	release, err := GetHighestVersionGitHubReleaseInfo(zitiApp, verbose)
	if release != nil {
		result = release.SemVer
	}
	return result, err
}

func GetHighestVersionGitHubReleaseInfo(appName string, verbose bool) (*GitHubReleasesData, error) {
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
	return GetHighestVersionRelease(appName, result)
}

func GetHighestVersionRelease(appName string, releases []*GitHubReleasesData) (*GitHubReleasesData, error) {
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

func GetLatestGitHubReleaseAsset(appName string, appGitHub string, version string, verbose bool) (*GitHubReleasesData, error) {
	if version != "latest" {
		if appName == "ziti-prox-c" {
			version = strings.TrimPrefix(version, "v")
		}

		if appName == "ziti" || appName == "ziti-edge-tunnel" {
			if !strings.HasPrefix(version, "v") {
				version = "v" + version
			}
		}
	}

	if version != "latest " {
		version = "tags/" + version
	}

	ctx, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelF()

	resp, err := getRequest(verbose).
		SetContext(ctx).
		SetQueryParams(map[string]string{}).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetResult(&GitHubReleasesData{}).
		Get("https://api.github.com/repos/openziti/" + appGitHub + "/releases/" + version)

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
	resp, err := resty.
		New().
		SetTimeout(time.Minute).
		SetRetryCount(2).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(15)).
		R().
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

func FindVersionAndInstallGitHubRelease(zitiApp string, zitiAppGitHub string, targetOS, targetArch string, binDir string, version string, verbose bool) error {
	releaseVersion := version
	if version != "" && version != "latest" {
		if _, err := semver.Make(strings.TrimPrefix(version, "v")); err != nil {
			return err
		}
	} else {
		version = "latest"
		v, err := GetLatestGitHubReleaseVersion(zitiApp, verbose)
		if err != nil {
			return err
		}
		releaseVersion = v.String()
	}

	release, err := GetLatestGitHubReleaseAsset(zitiApp, zitiAppGitHub, releaseVersion, verbose)
	if err != nil {
		return err
	}
	return InstallGitHubRelease(zitiApp, release, binDir, targetOS, targetArch, version)
}

func InstallGitHubRelease(zitiApp string, release *GitHubReleasesData, binDir string, targetOS, targetArch, version string) error {
	releaseUrl, err := release.GetDownloadUrl(zitiApp, targetOS, targetArch)
	if err != nil {
		return err
	}

	parsedUrl, err := url.Parse(releaseUrl)
	if err != nil {
		return err
	}

	fileName := filepath.Base(parsedUrl.Path)

	fullPath := filepath.Join(binDir, fileName)
	err = DownloadGitHubReleaseAsset(releaseUrl, fullPath)
	if err != nil {
		return err
	}

	defer func() {
		err = os.Remove(fullPath)
		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to removed downloaded release archive %s", fullPath)
		}
	}()

	if strings.HasSuffix(fileName, ".zip") {
		if zitiApp == c.ZITI_EDGE_TUNNEL {
			count := 0
			zitiFileName := "ziti-edge-tunnel-" + version
			err = Unzip(fullPath, binDir, func(path string) (string, bool) {
				if path == "ziti-edge-tunnel" {
					count++
					return zitiFileName, true
				}
				return "", false
			})

			if err != nil {
				return err
			}

			if count != 1 {
				return errors.Errorf("didn't find ziti-edge-tunnel executable in release archive. count: %v", count)
			}

			pfxlog.Logger().Infof("Successfully installed '%s' version '%s' to %s", zitiApp, release.Version, filepath.Join(binDir, zitiFileName))
			return nil
		} else {
			return errors.Errorf("unsupported application '%s'", zitiApp)
		}
	} else if strings.HasSuffix(fileName, ".tar.gz") {
		if zitiApp == c.ZITI {
			count := 0
			zitiFileName := "ziti-" + version
			expectedPath := "ziti"
			if version != "latest" {
				semVer, err := versions.ParseSemVer(version)
				if err != nil {
					return err
				}

				pathChangedVersion := versions.MustParseSemVer("0.29.0")
				if semVer.CompareTo(pathChangedVersion) < 0 {
					expectedPath = "ziti/ziti"
				}
			}
			err = UnTarGz(fullPath, binDir, func(path string) (string, bool) {
				if path == expectedPath {
					count++
					return zitiFileName, true
				}
				return "", false
			})

			if err != nil {
				return err
			}

			if count != 1 {
				return errors.Errorf("didn't find ziti executable in release archive. count: %v", count)
			}

			pfxlog.Logger().Infof("Successfully installed '%s' version '%s' to %s", zitiApp, release.Version, filepath.Join(binDir, zitiFileName))
			return nil
		} else {
			return errors.Errorf("unsupported application '%s'", zitiApp)
		}
	} else {
		return errors.Errorf("unsupported release file type '%s'", fileName)
	}
}
