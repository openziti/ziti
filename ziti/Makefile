
SHELL := /bin/bash
NAME := ziti
# GO := GO15VENDOREXPERIMENT=1 go
GO := go
VERSION_FROM_SRC := $(shell cat ../common/version/VERSION)
VERSION := $(shell echo $(VERSION_FROM_SRC)-$(BITBUCKET_BUILD_NUMBER))
ROOT_PACKAGE := bitbucket.org/netfoundry/ziti
S3_BUCKET_NAME := ziti-cd
GO_VERSION := $(shell $(GO) version | sed -e 's/^[^0-9.]*\([0-9.]*\).*/\1/')
PACKAGE_DIRS := $(shell $(GO) list ./... | grep -v /vendor/)
PKGS := $(shell go list ./... | grep -v /vendor | grep -v generated)


GO_DEPENDENCIES := cmd/*/*.go cmd/*/*/*.go pki/*/*.go 

REV        := $(shell git rev-parse --short HEAD 2> /dev/null  || echo 'unknown')
# BRANCH     := $(shell git rev-parse --abbrev-ref HEAD 2> /dev/null  || echo 'unknown')
BRANCH     := master
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)
BUILDFLAGS := -ldflags \
  " -X $(ROOT_PACKAGE)/common/version.Version=$(VERSION)\
		-X $(ROOT_PACKAGE)/common/version.Revision=$(REV)\
		-X $(ROOT_PACKAGE)/common/version.Branch=$(BRANCH)\
		-X $(ROOT_PACKAGE)/common/version.BuildDate=$(BUILD_DATE)\
		-X $(ROOT_PACKAGE)/common/version.OS=linux\
		-X $(ROOT_PACKAGE)/common/version.Arch=amd64\
		-X $(ROOT_PACKAGE)/common/version.GoVersion=$(GO_VERSION)"

CGO_ENABLED = 1

VENDOR_DIR=vendor

all: build

check: fmt build 

build: $(GO_DEPENDENCIES)
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILDFLAGS) -o build/$(NAME) cmd/ziti/main.go

builddoc: $(GO_DEPENDENCIES)
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILDFLAGS) -o build/$(NAME)doc cmd/doc/main.go

test: 
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) test -count=1 -coverprofile=cover.out -failfast -short -parallel 12 ./...

test1:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test $(PACKAGE_DIRS) -test.v -run $(TEST)

testbin:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test -c bitbucket.org/netfoundry/ziti-cli/cmd/ziti -o build/ziti-test

full: $(PKGS)

install: $(GO_DEPENDENCIES)
	GOBIN=${GOPATH}/bin $(GO) install $(BUILDFLAGS) cmd/ziti/main.go

fmt:
	@FORMATTED=`$(GO) fmt $(PACKAGE_DIRS)`
	@([[ ! -z "$(FORMATTED)" ]] && printf "Fixed unformatted files:\n$(FORMATTED)") || true

arm:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm $(GO) build $(BUILDFLAGS) -o build/$(NAME)-arm cmd/ziti/main.go

win:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 $(GO) build $(BUILDFLAGS) -o build/$(NAME).exe cmd/ziti/main.go

bootstrap: vendoring

vendoring:
	$(GO) get -u github.com/golang/dep/cmd/dep
	GO15VENDOREXPERIMENT=1 dep ensure

getgox:
	$(GO) get github.com/mitchellh/gox

release: check getgox
	rm -rf build release && mkdir build release

	cd release; gox -os="linux darwin windows" -arch=amd64 $(BUILDFLAGS) bitbucket.org/netfoundry/ziti-cli/...
	
	cd release; mkdir linux darwin windows

	mv release/$(NAME)_darwin_amd64 release/darwin/$(NAME)
	cd release/darwin; chmod +x $(NAME)
	cd release/darwin; tar -zcvf $(NAME).tar.gz $(NAME)
	cd release/darwin; rm $(NAME)

	mv release/$(NAME)_linux_amd64 release/linux/$(NAME)
	cd release/linux; upx $(NAME); chmod +x $(NAME)
	cd release/linux; tar -zcvf $(NAME).tar.gz $(NAME)
	cd release/linux; rm $(NAME)

	mv release/$(NAME)_windows_amd64.exe release/windows/$(NAME).exe
	cd release/windows; zip --junk-paths $(NAME).zip $(NAME).exe; rm $(NAME).exe

	aws s3 sync ./release s3://$(S3_BUCKET_NAME)/$(NAME)/$(VERSION)

	cd release; echo $(VERSION) > LATEST

	aws s3 cp ./release/LATEST s3://$(S3_BUCKET_NAME)/$(NAME)/LATEST
	aws s3api put-object-tagging --bucket ziti-cd --key $(NAME)/LATEST --tagging '{"TagSet":[{"Key":"version","Value":"$(VERSION)"}]}'

clean:
	rm -rf build dist release

linux:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build $(BUILDFLAGS) -o build/linux/ziti cmd/ziti/main.go


.PHONY: release clean arm

FGT := $(GOPATH)/bin/fgt
$(FGT):
	go get github.com/GeertJohan/fgt


LINTFLAGS:=-min_confidence 1.1

GOLINT := $(GOPATH)/bin/golint
$(GOLINT):
	go get github.com/golang/lint/golint

#	@echo "FORMATTING"
#	@$(FGT) gofmt -l=true $(GOPATH)/src/$@/*.go

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


