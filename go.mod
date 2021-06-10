module github.com/openziti/ziti

go 1.16

//replace github.com/openziti/foundation => ../foundation

//replace github.com/openziti/dilithium => ../dilithium

//replace github.com/openziti/fabric => ../fabric

//replace github.com/openziti/sdk-golang => ../sdk-golang

//replace github.com/openziti/edge => ../edge

replace go.etcd.io/bbolt => github.com/openziti/bbolt v1.3.6-0.20210317142109-547da822475e

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/fatih/color v1.7.0
	github.com/go-acme/lego/v4 v4.2.0
	github.com/go-openapi/runtime v0.19.29
	github.com/go-openapi/strfmt v0.20.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/mux v1.8.0
	github.com/influxdata/influxdb1-client v0.0.0-20191209144304-8bf82d3c094d
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/michaelquigley/pfxlog v0.3.7
	github.com/openziti/edge v0.19.132-0.20210610173540-9a80d2b77566
	github.com/openziti/fabric v0.16.66
	github.com/openziti/foundation v0.15.55-0.20210610173311-53fa252b25b0
	github.com/openziti/sdk-golang v0.15.52
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rs/cors v1.7.0
	github.com/russross/blackfriday v1.5.2
	github.com/shirou/gopsutil v2.20.9+incompatible
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.7.0
	github.com/urfave/negroni v1.0.0
	go.etcd.io/bbolt v1.3.5-0.20200615073812-232d8fc87f50
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	google.golang.org/grpc v1.27.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.4.0
	rsc.io/goversion v1.2.0
)
