package stageziti

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/acquire"
	"github.com/openziti/ziti/v2/common/getziti"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// hostGoos and hostGoarch are the platform of the hosts fablab provisions, which is what every
// binary staged into the kit has to be built for, regardless of what the orchestrator runs on.
const (
	hostGoos   = "linux"
	hostGoarch = "amd64"
)

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
		return getziti.InstallZiti(version, hostGoos, hostGoarch, run.GetBinDir(), false)
	})
}

// StageZitiForComponentOnce stages the ziti binary for a component once per run. When sourceRef is set
// it builds that git ref (branch, tag, or SHA) on openziti/ziti and stamps it as version; otherwise it
// stages localPath or the released version, exactly as StageZitiOnce does. This lets any component
// (controller, router, ziti-tunnel) run an unreleased build while reporting a chosen version.
func StageZitiForComponentOnce(run model.Run, component *model.Component, version, sourceRef, localPath string) error {
	if sourceRef != "" {
		return StageZitiFromRefOnce(run, component, sourceRef, version, localPath)
	}
	return StageZitiOnce(run, component, version, localPath)
}

// StageZitiFromRefOnce is StageZitiFromRef guarded by run.DoOnce, keyed on the ref and stamped version
// so distinct (ref, version) builds each run once while a repeated one is reused.
func StageZitiFromRefOnce(run model.Run, component *model.Component, sourceRef, stampVersion, localPath string) error {
	op := fmt.Sprintf("install.ziti-ref-%s-as-%s", sourceRef, stampVersion)
	return run.DoOnce(op, func() error {
		return StageZitiFromRef(run, component, sourceRef, stampVersion, localPath)
	})
}

// StageZitiFromRef builds the ziti binary from sourceRef (a git ref on openziti/ziti), stamps it to
// report stampVersion, and stages it under the ziti-<stampVersion> name so an in-place version swap can
// pick it up. An empty stampVersion stages it as plain ziti and leaves the build unstamped, which is
// how every other staging path names an unversioned binary and what the component start paths resolve.
// It always delegates to acquire, which resolves the ref to a commit and caches by that immutable id,
// so an unchanged ref reuses the cached build while a changed ref (or a branch moved to a new commit)
// rebuilds. It deliberately does not skip on an existing ziti-<stampVersion>: that would key
// reuse on the version name and silently serve a stale binary when the ref changed under the same
// stampVersion. If localPath is set it is staged directly instead of building, so a prebuilt binary
// still wins. GITHUB_TOKEN, when set, raises the ref-resolution rate limit.
//
// The build targets hostGoos/hostGoarch even when the orchestrator runs on something else, so a run
// driven from a mac or an arm box still stages a binary the hosts can execute.
func StageZitiFromRef(run model.Run, component *model.Component, sourceRef, stampVersion, localPath string) error {
	fileName := "ziti"
	if stampVersion != "" {
		fileName += "-" + stampVersion
	}
	target := filepath.Join(run.GetBinDir(), fileName)

	if localPath != "" {
		logrus.Infof("[%s] => [%s]", localPath, target)
		return util.CopyFile(localPath, target)
	}

	cacheDir, err := acquire.DefaultCacheDir()
	if err != nil {
		return err
	}
	cfg := acquire.Versions{Source: acquire.Source{Org: "openziti", Repo: "ziti"}}
	src := acquire.NewGitHubReleaseSource(cfg.Source.Org, cfg.Source.Repo, os.Getenv("GITHUB_TOKEN"))

	logrus.Infof("building ziti from ref %s (stamped %s) -> %s", sourceRef, stampVersion, target)
	built, id, err := acquire.Ziti(context.Background(), sourceRef, cfg, src, cacheDir,
		acquire.WithVersion(stampVersion), acquire.WithPlatform(hostGoos, hostGoarch))
	if err != nil {
		return fmt.Errorf("building ziti from ref %q: %w", sourceRef, err)
	}
	logrus.Infof("built ziti from ref %s (commit %s) stamped %s", sourceRef, id.Tag, stampVersion)
	return util.CopyFile(built, target)
}

func StageZrok(run model.Run, component *model.Component, version string, source string) error {
	return StageExecutable(run, "zrok", component, version, source, func() error {
		return getziti.InstallZrok(version, hostGoos, hostGoarch, run.GetBinDir(), false)
	})
}

func StageCaddy(run model.Run, component *model.Component, version string, source string) error {
	return StageExecutable(run, "caddy", component, version, source, func() error {
		return getziti.InstallCaddy(version, hostGoos, hostGoarch, run.GetBinDir(), false)
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

	return getziti.InstallZitiEdgeTunnel(version, hostGoos, hostGoarch, run.GetBinDir(), false)
}
