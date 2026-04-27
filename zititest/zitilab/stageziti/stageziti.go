package stageziti

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/getziti"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// zitiReleaseVersionRe matches semver-style release tags (e.g. v2.0.0,
// v2.0.0-pre11). Anything else — bare commit hashes, branch names, "main",
// "HEAD" — is treated as a git ref and built from source.
var zitiReleaseVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+([-+][0-9A-Za-z.\-]+)?$`)

func StageZitiOnce(run model.Run, component *model.Component, version string, source string) error {
	op := "install.ziti-"
	if version == "" {
		op += "local"
	} else {
		op += version
	}

	return run.DoOnce(op, func() error {
		return StageZiti(run, component, version, source)
	})
}

func StageZrokOnce(run model.Run, component *model.Component, version string, source string) error {
	op := "install.zrok-"
	if version == "" {
		op += "local"
	} else {
		op += version
	}

	return run.DoOnce(op, func() error {
		return StageZrok(run, component, version, source)
	})
}

func StageCaddyOnce(run model.Run, component *model.Component, version string, source string) error {
	op := "install.caddy-"
	if version == "" {
		op += "local"
	} else {
		op += version
	}

	return run.DoOnce(op, func() error {
		return StageCaddy(run, component, version, source)
	})
}

func StageZitiEdgeTunnelOnce(run model.Run, component *model.Component, version string, source string) error {
	op := "install.ziti-edge-tunnel-"
	if version == "" {
		op += "local"
	} else {
		op += version
	}

	return run.DoOnce(op, func() error {
		return StageZitiEdgeTunnel(run, component, version, source)
	})
}

func StageZiti(run model.Run, component *model.Component, version string, source string) error {
	return StageExecutable(run, "ziti", component, version, source, func() error {
		// Release-tagged versions download a prebuilt binary from GitHub
		// (fast). Anything else (commit hash, branch name) gets built from
		// source via git clone + go build. Override the source repo with
		// ZITI_TRAFFIC_TEST_REPO_URL (a local path is fine).
		if version != "" && !zitiReleaseVersionRe.MatchString(version) {
			target := filepath.Join(run.GetBinDir(), "ziti-"+version)
			return buildZitiFromGit(version, target)
		}
		return getziti.InstallZiti(version, "linux", "amd64", run.GetBinDir(), false)
	})
}

func StageZrok(run model.Run, component *model.Component, version string, source string) error {
	return StageExecutable(run, "zrok", component, version, source, func() error {
		return getziti.InstallZrok(version, "linux", "amd64", run.GetBinDir(), false)
	})
}

func StageCaddy(run model.Run, component *model.Component, version string, source string) error {
	return StageExecutable(run, "caddy", component, version, source, func() error {
		return getziti.InstallCaddy(version, "linux", "amd64", run.GetBinDir(), false)
	})
}

// StageZitiTrafficTestOnce stages ziti-traffic-test onto the fablab nodes.
// version="" preserves the legacy behavior (use a pre-built binary from
// `source`, the *_PATH env var, or PATH); a non-empty version is treated as a
// git ref (tag, branch, or commit SHA) and built from source by cloning the
// ziti repo into a temp dir.
//
// Caching: the resulting binary is named ziti-traffic-test-<version>. If a file
// of that name already exists in the kit's bin dir from a prior run, the build
// is skipped. For moving refs (branches, re-pointed tags) delete the cached
// file to force a rebuild.
func StageZitiTrafficTestOnce(run model.Run, component *model.Component, version string, source string) error {
	op := "install.ziti-traffic-test-"
	if version == "" {
		op += "local"
	} else {
		op += version
	}
	return run.DoOnce(op, func() error {
		return StageZitiTrafficTest(run, component, version, source)
	})
}

// StageZitiTrafficTest performs the staging unconditionally. Prefer
// StageZitiTrafficTestOnce when called from per-component StageFiles to avoid
// redundant builds across components on the same run.
func StageZitiTrafficTest(run model.Run, component *model.Component, version string, source string) error {
	if version == "" {
		// Legacy local-only path: error if no pre-built binary is available.
		return StageExecutable(run, "ziti-traffic-test", component, "", source, func() error {
			return fmt.Errorf("unable to fetch ziti-traffic-test, as it is a local-only application")
		})
	}
	target := filepath.Join(run.GetBinDir(), "ziti-traffic-test-"+version)
	return StageExecutable(run, "ziti-traffic-test", component, version, source, func() error {
		return buildZitiTrafficTestFromGit(version, target)
	})
}

func StageLocalOnce(run model.Run, executable string, component *model.Component, source string) error {
	op := fmt.Sprintf("install.%s-local", executable)
	return run.DoOnce(op, func() error {
		return StageExecutable(run, executable, component, "", source, func() error {
			return fmt.Errorf("unable to fetch %s, as it a local-only application", executable)
		})
	})
}

func StageExecutable(run model.Run, executable string, component *model.Component, version string, source string, fallbackF func() error) error {
	fileName := executable
	if version != "" {
		fileName += "-" + version
	}

	target := filepath.Join(run.GetBinDir(), fileName)
	if version == "" || version == "latest" {
		_ = os.Remove(target)
	}

	envVar := strings.ToUpper(executable) + "_PATH"

	if version == "" {
		if source != "" {
			logrus.Infof("[%s] => [%s]", source, target)
			return util.CopyFile(source, target)
		}
		if envSource, found := component.GetStringVariable(envVar); found {
			logrus.Infof("[%s] => [%s]", envSource, target)
			return util.CopyFile(envSource, target)
		}
		if zitiPath, err := exec.LookPath(executable); err == nil {
			logrus.Infof("[%s] => [%s]", zitiPath, target)
			return util.CopyFile(zitiPath, target)
		}
		return fmt.Errorf("%s binary not found in path, no path provided and no %s env variable set", executable, envVar)
	}

	found, err := run.FileExists(filepath.Join(model.BuildKitDir, model.BuildBinDir, fileName))
	if err != nil {
		return err
	}

	if found {
		logrus.Infof("%s already present, not downloading again", target)
		return nil
	}

	logrus.Infof("%s not present, attempting to fetch", target)

	return fallbackF()
}

func StageZitiEdgeTunnel(run model.Run, component *model.Component, version string, source string) error {
	fileName := "ziti-edge-tunnel"
	if version != "" {
		fileName += "-" + version
	}

	target := filepath.Join(run.GetBinDir(), fileName)
	if version == "" || version == "latest" {
		_ = os.Remove(target)
	}

	if version == "" {
		if source != "" {
			logrus.Infof("[%s] => [%s]", source, target)
			return util.CopyFile(source, target)
		}
		if envSource, found := component.GetStringVariable("ziti-edge-tunnel.path"); found {
			logrus.Infof("[%s] => [%s]", envSource, target)
			return util.CopyFile(envSource, target)
		}
		if zitiPath, err := exec.LookPath("ziti-edge-tunnel"); err == nil {
			logrus.Infof("[%s] => [%s]", zitiPath, target)
			return util.CopyFile(zitiPath, target)
		}
		return errors.New("ziti-edge-tunnel binary not found in path, no path provided and no ziti-edge-tunnel.path env variable set")
	}

	found, err := run.FileExists(filepath.Join(model.BuildKitDir, model.BuildBinDir, fileName))
	if err != nil {
		return err
	}

	if found {
		logrus.Infof("%s already present, not downloading again", target)
		return nil
	}
	logrus.Infof("%s not present, attempting to fetch", target)

	return getziti.InstallZitiEdgeTunnel(version, "linux", "amd64", run.GetBinDir(), false)
}
