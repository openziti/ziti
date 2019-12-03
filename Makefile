
SMOKE_FREE_FILE:= .smoke-free

SHELL := /bin/bash
RELEASE_BRANCH := master
SEMVER_AUTOBUMP_BRANCH := master

EDGE_CMD_LIST := edge-controller gateway
EDGE_TARGETS := $(addprefix ziti-, $(EDGE_CMD_LIST))

FABRIC_CMD_LIST := controller fabric fabric-gw fabric-test router
FABRIC_TARGETS := $(addprefix ziti-, $(FABRIC_CMD_LIST))

TUNNEL_CMD_LIST := tunnel
TUNNEL_TARGETS := $(addprefix ziti-, $(TUNNEL_CMD_LIST))

SDK_CMD_LIST := enroller proxy
SDK_TARGETS := $(addprefix ziti-, $(SDK_CMD_LIST))

GO := go
ROOT_PACKAGE := bitbucket.org/netfoundry/ziti/common
GO_VERSION := $(shell $(GO) version | sed -e 's/^[^0-9.]*\([0-9.]*\).*/\1/')
PACKAGE_DIRS := $(shell $(GO) list ./... | grep -v /vendor/)
PKGS := $(shell go list ./... | grep -v /vendor | grep -v generated)

GO_DEPENDENCIES := cmd/*/*.go fabric/*/*.go info/*/*.go sequence/*/*.go transport/*/*.go

BRANCH_NAME := $(shell echo $$BRANCH_NAME)
BRANCH := $(shell echo $$BRANCH_NAME)
BUILD_NUMBER := $(shell echo $$BUILD_NUMBER)


REV        := $(shell git rev-parse --short HEAD 2> /dev/null  || echo 'unknown')
BUILD_DATE := $(shell date +%a-%m/%d/%Y-%H:%M:%S-%Z)
BUILD_COMMITTER := $(shell git log -1 --pretty=format:'%ae' 2> /dev/null  || echo 'curt.tudor@netfoundry.io')
ZITI_DEPLOY_BRANCH := $(shell if git ls-remote --heads git@bitbucket.org:netfoundry/ziti-deploy-modules.git $$BRANCH_NAME | grep -sw $$BRANCH_NAME 1>/dev/null; then echo "$$BRANCH_NAME" ; else echo "master" ; fi)
VERSION_FROM_SRC := $(shell cat common/version/VERSION)
VERSION := $(shell echo $(VERSION_FROM_SRC)-$$BUILD_NUMBER)

BUILDFLAGS_DARWIN_AMD64 := -ldflags \
  " -X $(ROOT_PACKAGE)/version.Version=$(VERSION)\
	-X $(ROOT_PACKAGE)/version.Revision=$(REV)\
	-X $(ROOT_PACKAGE)/version.Branch=$(BRANCH)\
	-X $(ROOT_PACKAGE)/version.BuildDate=$(BUILD_DATE)\
	-X $(ROOT_PACKAGE)/version.OS=darwin\
	-X $(ROOT_PACKAGE)/version.Arch=amd64\
	-X $(ROOT_PACKAGE)/version.GoVersion=$(GO_VERSION) -s"

BUILDFLAGS_LINUX_AMD64 := -ldflags \
  " -X $(ROOT_PACKAGE)/version.Version=$(VERSION)\
	-X $(ROOT_PACKAGE)/version.Revision=$(REV)\
	-X $(ROOT_PACKAGE)/version.Branch=$(BRANCH)\
	-X $(ROOT_PACKAGE)/version.BuildDate=$(BUILD_DATE)\
	-X $(ROOT_PACKAGE)/version.OS=linux\
	-X $(ROOT_PACKAGE)/version.Arch=amd64\
	-X $(ROOT_PACKAGE)/version.GoVersion=$(GO_VERSION) "

BUILDFLAGS_LINUX_ARM := -ldflags \
  " -X $(ROOT_PACKAGE)/version.Version=$(VERSION)\
	-X $(ROOT_PACKAGE)/version.Revision=$(REV)\
	-X $(ROOT_PACKAGE)/version.Branch=$(BRANCH)\
	-X $(ROOT_PACKAGE)/version.BuildDate=$(BUILD_DATE)\
	-X $(ROOT_PACKAGE)/version.OS=linux\
	-X $(ROOT_PACKAGE)/version.Arch=arm\
	-X $(ROOT_PACKAGE)/version.GoVersion=$(GO_VERSION) "

BUILDFLAGS_WINDOWS_AMD64 := -ldflags \
  " -X $(ROOT_PACKAGE)/version.Version=$(VERSION)\
	-X $(ROOT_PACKAGE)/version.Revision=$(REV)\
	-X $(ROOT_PACKAGE)/version.Branch=$(BRANCH)\
	-X $(ROOT_PACKAGE)/version.BuildDate=$(BUILD_DATE)\
	-X $(ROOT_PACKAGE)/version.OS=windows\
	-X $(ROOT_PACKAGE)/version.Arch=amd64\
	-X $(ROOT_PACKAGE)/version.GoVersion=$(GO_VERSION) "

CGO_ENABLED = 1

VENDOR_DIR=vendor

all: pre-release

check:
	echo "ZITI_DEPLOY_BRANCH is: $(ZITI_DEPLOY_BRANCH)"
	rm -rf release && mkdir release
	cd release; mkdir -p amd64/linux amd64/darwin amd64/windows arm/linux

define do-semver-bump
$(info ============ do-semver-bump entered for branch: $(BRANCH), build-number: $(BUILD_NUMBER) ============)
$(eval PRE_BUMP_STATUS := $(shell ./pre-bump-semver.sh || echo $$? ) )
endef

define skip-semver-bump
$(info ============ skip-semver-bump entered for branch: $(BRANCH), build-number: $(BUILD_NUMBER) ============)
endef

define semver-bump-check
$(info ============ semver-bump-check entered for branch: $(BRANCH), build-number: $(BUILD_NUMBER) ============)
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(SEMVER_AUTOBUMP_BRANCH))),$(call do-semver-bump),$(call skip-semver-bump)) 
$(eval VERSION_FROM_SRC := $(shell cat common/version/VERSION)) 
$(eval VERSION := $(shell echo $(VERSION_FROM_SRC)-$(BUILD_NUMBER)))
endef

semver-bump:
	$(call semver-bump-check)
	$(info ============ PRE_BUMP_STATUS is: '$(PRE_BUMP_STATUS)' ============)
	$(if $(filter $(strip $(PRE_BUMP_STATUS)),200),$(info ============ Short-circuiting build here since semver-bump has occurred ============),$(MAKE) -f Jenkins.Makefile release) 


full: $(PKGS)

fmt:
	@FORMATTED=`$(GO) fmt $(PACKAGE_DIRS)`
	@([[ ! -z "$(FORMATTED)" ]] && printf "Fixed unformatted files:\n$(FORMATTED)") || true

getgox:
	$(GO) get github.com/mitchellh/gox

$(EDGE_TARGETS): ziti%:
$(FABRIC_TARGETS): ziti%:
$(TUNNEL_TARGETS): ziti%:
$(SDK_TARGETS): ziti%:

define publish-target-as-release
$(info ============ publish-target-as-release starting for: $1 $2 $3 $4 ============)
cd release/$2/$3; jfrog rt u $4 ziti-staging/$1/$2/$3/$(VERSION)/$4 --apikey $(JFROG_API_KEY) --url https://netfoundry.jfrog.io/netfoundry --props "version=$(VERSION);name=$1;arch=$2;os=$3;branch=master" --build-name=ziti --build-number=$(VERSION)
endef

define publish-target-as-snapshot
$(info ============ publish-target-as-snapshot starting for: $1 $2 $3 $4 $5 ============)
cd release/$3/$4; jfrog rt u $5 ziti-snapshot/$2/$1/$3/$4/$(VERSION)/$5 --apikey $(JFROG_API_KEY) --url https://netfoundry.jfrog.io/netfoundry --props "version=$(VERSION);name=$1;arch=$3;os=$4;branch=$2" --build-name=ziti --build-number=$(VERSION)
endef

define publish-all-as-release
$(info ============ publish-all-as-release starting  ============)
cd release; tar -zcvf ziti-all.$(VERSION).tar.gz amd64 arm
cd release; jfrog rt u ziti-all.$(VERSION).tar.gz ziti-staging/ziti-all/$(VERSION)/ziti-all.$(VERSION).tar.gz --apikey $(JFROG_API_KEY) --url https://netfoundry.jfrog.io/netfoundry --props "version=$(VERSION);branch=master" --build-name=ziti --build-number=$(VERSION)
endef

define publish-all-as-snapshot
$(info ============ publish-all-as-snapshot starting  ============)
cd release; tar -zcvf ziti-all.$(VERSION).tar.gz amd64 arm
cd release; jfrog rt u ziti-all.$(VERSION).tar.gz ziti-snapshot/$1/ziti-all/$(VERSION)/ziti-all.$(VERSION).tar.gz --apikey $(JFROG_API_KEY) --url https://netfoundry.jfrog.io/netfoundry --props "version=$(VERSION);branch=$1" --build-name=ziti --build-number=$(VERSION)
endef

define make-target
$(info ============ make-target starting for: $1 ============)

mv release/$1_darwin_amd64 release/amd64/darwin/$1
cd release/amd64/darwin; chmod +x $1
cd release/amd64/darwin; tar -zcvf $1.tar.gz $1
cd release/amd64/darwin; rm $1
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,amd64,darwin,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),amd64,darwin,$1.tar.gz))

mv release/$1_linux_amd64 release/amd64/linux/$1
cd release/amd64/linux; chmod +x $1
cd release/amd64/linux; tar -zcvf $1.tar.gz $1
cd release/amd64/linux; rm $1
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,amd64,linux,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),amd64,linux,$1.tar.gz))

mv release/$1_linux_arm release/arm/linux/$1
cd release/arm/linux; chmod +x $1
cd release/arm/linux; tar -zcvf $1.tar.gz $1
cd release/arm/linux; rm $1
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,arm,linux,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),arm,linux,$1.tar.gz))

mv release/$1_windows_amd64.exe release/amd64/windows/$1.exe
cd release/amd64/windows; tar -zcvf $1.tar.gz $1.exe
cd release/amd64/windows; rm $1.exe
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,amd64,windows,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),amd64,windows,$1.tar.gz))

endef

define make-target-linux-only
$(info ============ make-target-linux-only starting for: $1 ============)

mv release/$1_linux_amd64 release/amd64/linux/$1
cd release/amd64/linux; chmod +x $1
cd release/amd64/linux; tar -zcvf $1.tar.gz $1
cd release/amd64/linux; rm $1
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,amd64,linux,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),amd64,linux,$1.tar.gz))


mv release/$1_linux_arm release/arm/linux/$1
cd release/arm/linux; chmod +x $1
cd release/arm/linux; tar -zcvf $1.tar.gz $1
cd release/arm/linux; rm $1
$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-target-as-release,$1,arm,linux,$1.tar.gz),$(call publish-target-as-snapshot,$1,$(BRANCH_NAME),arm,linux,$1.tar.gz))

endef

define make-everything-tarball

$(if $(filter $(strip $(BRANCH_NAME)),$(strip $(RELEASE_BRANCH))),$(call publish-all-as-release),$(call publish-all-as-snapshot,$(BRANCH_NAME)))

endef

ziti-cli:
	cd cli; $(call make-target,ziti)

ziti-controller:
	cd fabric; $(call make-target,ziti-controller)

ziti-fabric:
	cd fabric; $(call make-target,ziti-fabric)

ziti-fabric-gw:
	cd fabric; $(call make-target,ziti-fabric-gw)

ziti-fabric-test:
	cd fabric; $(call make-target,ziti-fabric-test)

ziti-router:
	cd fabric; $(call make-target,ziti-router)

ziti-tunnel:
	cd tunnel; $(call make-target,ziti-tunnel)

ziti-enroller:
	cd sdk; $(call make-target,ziti-enroller)

ziti-proxy:
	cd sdk; $(call make-target,ziti-proxy)

everything-tarball:
	$(call make-everything-tarball,ziti)

launch-smoketest:
ifneq ("$(wildcard $(SMOKE_FREE_FILE))","")
	echo "$(SMOKE_FREE_FILE) detected, skipping SmokeTest"
	curl -d "job=ziti-smoke-test&token=a5ce73aa-dad0-429b-a45e-0f82ab24a2d9&location=us-east-1&autodelete=yes&branch=$(BRANCH_NAME)&version=$(VERSION)&committer=$(BUILD_COMMITTER)&deploy_branch=$(ZITI_DEPLOY_BRANCH)&smokefree=true" -i -X POST https://jenkinstest.tools.netfoundry.io/buildByToken/buildWithParameters
else
	curl -d "job=ziti-smoke-test&token=a5ce73aa-dad0-429b-a45e-0f82ab24a2d9&location=us-east-1&autodelete=yes&branch=$(BRANCH_NAME)&version=$(VERSION)&committer=$(BUILD_COMMITTER)&deploy_branch=$(ZITI_DEPLOY_BRANCH)" -i -X POST https://jenkinstest.tools.netfoundry.io/buildByToken/buildWithParameters
endif

ziti-fabric-common:
	$(info ============ ziti-fabric-common starting ============)
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=/opt/osxcross/target/bin/o64-clang CXX=/opt/osxcross/target/bin/o64-clang++ gox -cgo -os="darwin" -arch=amd64 $(BUILDFLAGS_DARWIN_AMD64) bitbucket.org/netfoundry/ziti/fabric/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) gox -cgo -os="linux" -arch=amd64 $(BUILDFLAGS_LINUX_AMD64) bitbucket.org/netfoundry/ziti/fabric/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=arm-linux-gnueabihf-gcc gox -cgo -os="linux" -arch=arm $(BUILDFLAGS_LINUX_ARM) bitbucket.org/netfoundry/ziti/fabric/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ gox -cgo -os="windows" -arch=amd64 $(BUILDFLAGS_WINDOWS_AMD64) bitbucket.org/netfoundry/ziti/fabric/...

ziti-tunnel-common:
	$(info ============ ziti-tunnel-common starting ============)
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=/opt/osxcross/target/bin/o64-clang CXX=/opt/osxcross/target/bin/o64-clang++ gox -cgo -os="darwin" -arch=amd64 $(BUILDFLAGS_DARWIN_AMD64) bitbucket.org/netfoundry/ziti/tunnel/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) gox -cgo -os="linux" -arch=amd64 $(BUILDFLAGS_LINUX_AMD64) bitbucket.org/netfoundry/ziti/tunnel/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=arm-linux-gnueabihf-gcc gox -cgo -os="linux" -arch=arm $(BUILDFLAGS_LINUX_ARM) bitbucket.org/netfoundry/ziti/tunnel/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ gox -cgo -os="windows" -arch=amd64 $(BUILDFLAGS_WINDOWS_AMD64) bitbucket.org/netfoundry/ziti/tunnel/...

ziti-cli-common:
	$(info ============ ziti-cli-common starting ============)
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=/opt/osxcross/target/bin/o64-clang CXX=/opt/osxcross/target/bin/o64-clang++ gox -cgo -os="darwin" -arch=amd64 $(BUILDFLAGS_DARWIN_AMD64) bitbucket.org/netfoundry/ziti/cli/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) gox -cgo -os="linux" -arch=amd64 $(BUILDFLAGS_LINUX_AMD64) bitbucket.org/netfoundry/ziti/cli/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=arm-linux-gnueabihf-gcc gox -cgo -os="linux" -arch=arm $(BUILDFLAGS_LINUX_ARM) bitbucket.org/netfoundry/ziti/cli/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ gox -cgo -os="windows" -arch=amd64 $(BUILDFLAGS_WINDOWS_AMD64) bitbucket.org/netfoundry/ziti/cli/...

ziti-sdk-common:
	$(info ============ ziti-sdk-common starting ============)
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=/opt/osxcross/target/bin/o64-clang CXX=/opt/osxcross/target/bin/o64-clang++ gox -cgo -os="darwin" -arch=amd64 $(BUILDFLAGS_DARWIN_AMD64) bitbucket.org/netfoundry/ziti/sdk/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) gox -cgo -os="linux" -arch=amd64 $(BUILDFLAGS_LINUX_AMD64) bitbucket.org/netfoundry/ziti/sdk/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=arm-linux-gnueabihf-gcc gox -cgo -os="linux" -arch=arm $(BUILDFLAGS_LINUX_ARM) bitbucket.org/netfoundry/ziti/sdk/...
	cd release; CGO_ENABLED=$(CGO_ENABLED) CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ gox -cgo -os="windows" -arch=amd64 $(BUILDFLAGS_WINDOWS_AMD64) bitbucket.org/netfoundry/ziti/sdk/...

build-publish:
	jfrog rt bce ziti $(VERSION)
	jfrog rt bp --apikey $(JFROG_API_KEY) --url https://netfoundry.jfrog.io/netfoundry ziti $(VERSION)

release: getgox ziti-cli-common ziti-cli ziti-tunnel-common ziti-fabric-common ziti-sdk-common $(EDGE_TARGETS) $(FABRIC_TARGETS) $(TUNNEL_TARGETS) $(SDK_TARGETS) everything-tarball build-publish launch-smoketest

pre-release: check semver-bump

clean:
	rm -rf release

.PHONY: release clean

FGT := $(GOPATH)/bin/fgt
$(FGT):
	go get github.com/GeertJohan/fgt

LINTFLAGS:=-min_confidence 1.1

GOLINT := $(GOPATH)/bin/golint
$(GOLINT):
	go get github.com/golang/lint/golint

$(PKGS): $(GOLINT) $(FGT)
	@echo "LINTING"
	@$(FGT) $(GOLINT) $(LINTFLAGS) $(GOPATH)/src/$@/*.go
	@echo "VETTING"
	@go vet -v $@
	@echo "TESTING"
	@go test -v $@

.PHONY: lint
lint: vendor | $(PKGS) $(GOLINT) # ❷
	@cd $(BASE) && ret=0 && for pkg in $(PKGS); do \
	    test -z "$$($(GOLINT) $$pkg | tee /dev/stderr)" || ret=1 ; \
	done ; exit $$ret

.PHONY: vet
vet: tools.govet
	@echo "--> checking code correctness with 'go vet' tool"
	@go vet ./...

tools.govet:
	@go tool vet 2>/dev/null ; if [ $$? -eq 3 ]; then \
		echo "--> installing govet"; \
		go get golang.org/x/tools/cmd/vet; \
	fi

GAS := $(GOPATH)/bin/gas
$(GAS):
	go get github.com/GoASTScanner/gas/cmd/gas/...

.PHONY: sec
sec: $(GAS)
	@echo "SECURITY"
	@mkdir -p scanning
	$(GAS) -fmt=yaml -out=scanning/results.yaml ./...


