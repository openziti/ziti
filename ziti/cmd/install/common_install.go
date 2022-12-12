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

package install

import (
	"fmt"
	"github.com/blang/semver"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/util"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/resty.v1"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// installRequirements installs any requirements needed to run the ziti CLI
func (o *InstallOptions) installRequirements(extraDependencies ...string) error {
	var deps []string

	for _, dep := range extraDependencies {
		deps = o.addRequiredBinary(dep, deps)
	}

	return o.installMissingDependencies(deps)
}

func (o *InstallOptions) addRequiredBinary(binName string, deps []string) []string {
	d := binaryShouldBeInstalled(binName)
	if d != "" && util.StringArrayIndex(deps, d) < 0 {
		deps = append(deps, d)
	}
	return deps
}

// appends the binary to the deps array if it cannot be found on the $PATH
func binaryShouldBeInstalled(d string) string {
	_, err := exec.LookPath(d)
	if err != nil {
		// look for windows exec
		if runtime.GOOS == "windows" {
			d = d + ".exe"
			_, err = exec.LookPath(d)
			if err == nil {
				return ""
			}
		}
		binDir, err := util.BinaryLocation()
		if err == nil {
			exists, err := util.FileExists(filepath.Join(binDir, d))
			if err == nil && exists {
				return ""
			}
		}
		log.Infof("%s not found\n", d)
		return d
	}
	return ""
}

func isBinaryInstalled(d string) bool {
	binDir, err := util.BinaryLocation()
	if err == nil {
		exists, err := util.FileExists(filepath.Join(binDir, d))
		if err == nil && exists {
			return true
		}
	}
	// log.Warnf("%s not found\n", d)
	return false
}

func (o *InstallOptions) deleteInstalledBinary(d string) bool {
	binDir, err := util.BinaryLocation()
	if err == nil {
		exists, err := util.FileExists(filepath.Join(binDir, d))
		if err == nil && exists {
			err = os.Remove(filepath.Join(binDir, d))
			if err != nil {
				log.Warnf("Error attempting to delete %s: %s \n", d, err)
				return false
			}
			return true
		}
	}
	return false
}

func (o *InstallOptions) installMissingDependencies(deps []string) error {

	if len(deps) == 0 {
		return nil
	}

	if o.BatchMode {
		return fmt.Errorf("run without batch mode or manually install missing dependencies %v", deps)
	}
	install := []string{}
	prompt := &survey.MultiSelect{
		Message: "Installing required dependencies, deselect to avoid auto installing:",
		Options: deps,
		Default: deps,
	}
	survey.AskOne(prompt, &install, nil)

	return o.doInstallMissingDependencies(install)
}

func (o *InstallOptions) doInstallMissingDependencies(install []string) error {

	for _, i := range install {
		var err error
		switch i {
		case "aws":
			err = o.installAWSCli()
		default:
			return fmt.Errorf("unknown dependency to install %s", i)
		}
		if err != nil {
			return fmt.Errorf("error installing %s: %v", i, err)
		}
	}
	return nil
}

func (o *InstallOptions) installAWSCli() error {
	return fmt.Errorf("Ziti's ability to auto-install the AWS CLI is curently un-implemented; For now, you must manually install the AWS CLI")
}

func (o *InstallOptions) shouldInstallBinary(binDir string, name string) (fileName string, download bool, err error) {
	fileName = name
	download = false
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}
	pgmPath, err := exec.LookPath(fileName)
	if err == nil {
		log.Warnf("%s is already available on your PATH at %s\n", util.ColorInfo(fileName), util.ColorInfo(pgmPath))
		return
	}

	// lets see if its been installed but just is not on the PATH
	exists, err := util.FileExists(filepath.Join(binDir, fileName))
	if err != nil {
		return
	}
	if exists {
		log.Warnf("Please add %s to your PATH\n", util.ColorInfo(binDir))
		return
	}
	download = true
	return
}

func (o *InstallOptions) downloadFile(clientURL string, fullPath string) error {
	log.Infof("Downloading %s to %s...\n", util.ColorInfo(clientURL), util.ColorInfo(fullPath))
	err := util.DownloadFile(fullPath, clientURL)
	if err != nil {
		return fmt.Errorf("Unable to download file %s from %s due to: %v", fullPath, clientURL, err)
	}
	log.Infof("Downloaded %s\n", util.ColorInfo(fullPath))
	return nil
}

func (o *InstallOptions) getLatestZitiVersion(branch string) (semver.Version, error) {
	return util.GetLatestVersionFromArtifactory(o.Verbose, o.Staging, branch, c.ZITI)
}

func (o *InstallOptions) getLatestZitiAppVersion(branch string, zitiApp string) (semver.Version, error) {
	return util.GetLatestVersionFromArtifactory(o.Verbose, o.Staging, branch, zitiApp)
}

func (o *InstallOptions) getLatestZitiAppVersionForBranch(branch string, zitiApp string) (semver.Version, error) {
	return util.GetLatestVersionFromArtifactory(o.Verbose, o.Staging, branch, zitiApp)
}

func (o *InstallOptions) getLatestTerraformProviderVersion(branch string, provider string) (semver.Version, error) {
	return util.GetLatestTerraformProviderVersionFromArtifactory(branch, provider)
}

func (o *InstallOptions) getLatestGitHubReleaseVersion(zitiApp string) (semver.Version, error) {
	var result semver.Version
	release, err := util.GetHighestVersionGitHubReleaseInfo(o.Verbose, zitiApp)
	if release != nil {
		result = release.SemVer
	}
	return result, err
}

func (o *InstallOptions) getHighestVersionGitHubReleaseInfo(zitiApp string) (*util.GitHubReleasesData, error) {
	return util.GetHighestVersionGitHubReleaseInfo(o.Verbose, zitiApp)
}

func (o *InstallOptions) getCurrentZitiSnapshotList() ([]string, error) {
	children, err := util.GetCurrentSnapshotListFromArtifactory(o.Verbose)

	list := make([]string, 0)
	list = append(list, "main")

	for _, v := range children {
		str := strings.Replace(v.URI, "/", "", -1)
		list = append(list, str)
	}

	return list, err
}

func (o *InstallOptions) installZitiApp(branch string, zitiApp string, upgrade bool, version string) error {
	binDir, err := util.BinaryLocation()
	if err != nil {
		return err
	}
	binary := zitiApp
	fileName := binary
	if !upgrade {
		f, flag, err := o.shouldInstallBinary(binDir, binary)
		if err != nil || !flag {
			return err
		}
		fileName = f
	}
	var latestVersion semver.Version

	if version != "" {

		if strings.Contains(version, "*") {
			latestVersion, err = util.GetLatestSemanticVersionFromArtifactory(o.Verbose, o.Staging, branch, binary, version)
			if err != nil {
				return err
			}
			version = latestVersion.String()
		} else {
			latestVersion, err = semver.Make(version)
			if err != nil {
				return err
			}
		}
	}

	fullPath := filepath.Join(binDir, fileName)
	ext := ".tar.gz"
	tarFile := fullPath + ext

	repoUrl := util.GetArtifactoryPath(o.Staging, binary, branch) + "/" + version + "/" + zitiApp + ext

	log.Infof("Attempting to download %s to %s", repoUrl, tarFile)

	err = util.DownloadArtifactoryFile(repoUrl, tarFile)
	if err != nil {
		return err
	}

	log.Infof("Attempting to extract %s to %s", tarFile, fileName)
	err = util.UnTargz(tarFile, binDir, []string{binary, fileName})
	if err != nil {
		return err
	}
	err = os.Remove(tarFile)
	if err != nil {
		return err
	}
	log.Infof("Successfully installed '%s' version '%s' from branch '%s'\n", zitiApp, latestVersion, branch)
	return os.Chmod(fullPath, 0755)
}

func (o *InstallOptions) installTerraformProvider(branch string, provider string, upgrade bool, version string) error {
	resty.SetDebug(o.Verbose)
	binDir, err := util.TerraformProviderBinaryLocation()
	if err != nil {
		return err
	}
	latestVersion, err := util.GetLatestTerraformProviderVersionFromArtifactory(branch, provider)
	if err != nil {
		return err
	}
	if version != "" {
		latestVersion, err = semver.Make(version)
		if err != nil {
			return err
		}
	}

	fullPath := filepath.Join(binDir, c.TERRAFORM_PROVIDER_PREFIX+provider)
	ext := ".tar.gz"
	tarFile := fullPath + ext

	repoUrl := util.GetTerraformProviderArtifactoryPath(provider, branch) + "/" + version + "/" + c.TERRAFORM_PROVIDER_PREFIX + provider + "_v" + version + ext

	log.Infof("Attempting to download %s to %s", repoUrl, tarFile)

	err = util.DownloadArtifactoryFile(repoUrl, tarFile)
	if err != nil {
		return err
	}
	fileToExtract := c.TERRAFORM_PROVIDER_PREFIX + provider + "_v" + version
	if runtime.GOOS == "windows" {
		fileToExtract += ".exe"
	}
	log.Infof("Attempting to extract file: '%s'\n", fileToExtract)
	err = util.UnTargz(tarFile, binDir, []string{fileToExtract})
	if err != nil {
		return err
	}
	err = os.Remove(tarFile)
	if err != nil {
		return err
	}
	log.Infof("Successfully installed Terraform Provider '%s' version '%s' from branch '%s'\n", provider, latestVersion, branch)
	fileToChmod := fullPath + "_v" + version
	if runtime.GOOS == "windows" {
		fileToChmod += ".exe"
	}
	return os.Chmod(fileToChmod, 0755)
}

func (o *InstallOptions) findVersionAndInstallGitHubRelease(zitiApp string, zitiAppGitHub string, upgrade bool, version string) error {
	var latestVersion semver.Version
	var err error
	if version != "" {
		if strings.Contains(version, "*") {
			latestRelease, err := util.GetHighestVersionGitHubReleaseInfo(o.Verbose, zitiAppGitHub)
			if err != nil {
				return err
			}
			latestVersion = latestRelease.SemVer
			version = latestVersion.String()
		} else {
			latestVersion, err = semver.Make(version)
			if err != nil {
				return err
			}
		}
	}

	release, err := util.GetLatestGitHubReleaseAsset(o.Staging, zitiAppGitHub)
	if err != nil {
		return err
	}
	return o.installGitHubRelease(zitiApp, upgrade, release)
}

func (o *InstallOptions) installGitHubRelease(zitiApp string, upgrade bool, release *util.GitHubReleasesData) error {
	binDir, err := util.BinaryLocation()
	if err != nil {
		return err
	}
	binary := zitiApp
	fileName := binary

	if !upgrade {
		f, flag, err := o.shouldInstallBinary(binDir, binary)
		if err != nil || !flag {
			return err
		}
		fileName = f
	}

	fullPath := filepath.Join(binDir, fileName)
	ext := ".zip"
	zipFile := fullPath + ext

	releaseUrl, err := release.GetDownloadUrl(zitiApp)
	if err != nil {
		return err
	}

	err = util.DownloadGitHubReleaseAsset(releaseUrl, zipFile)
	if err != nil {
		return err
	}

	err = util.Unzip(zipFile, binDir)
	if err != nil {
		return err
	}
	err = os.Remove(zipFile)
	if err != nil {
		return err
	}
	log.Infof("Successfully installed '%s' version '%s'\n", zitiApp, release.SemVer)
	return os.Chmod(fullPath, 0755)
}
