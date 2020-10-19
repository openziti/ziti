module github.com/openziti/ziti

go 1.14

// replace github.com/openziti/foundation => ../foundation

// replace github.com/michaelquigley/dilithium => ../../q/research/dilithium

// replace github.com/openziti/fabric => ../fabric

// replace github.com/openziti/sdk-golang => ../sdk-golang

// replace github.com/openziti/edge => ../edge

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/fatih/color v1.7.0
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.2
	github.com/gorilla/mux v1.8.0
	github.com/influxdata/influxdb1-client v0.0.0-20191209144304-8bf82d3c094d
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/openziti/edge v0.16.61
	github.com/openziti/fabric v0.14.10
	github.com/openziti/foundation v0.14.10
	github.com/openziti/sdk-golang v0.13.61
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rs/cors v1.7.0
	github.com/russross/blackfriday v1.5.2
	github.com/shirou/gopsutil v2.19.11+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.5.0
	github.com/stretchr/testify v1.6.1
	github.com/urfave/negroni v1.0.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	google.golang.org/grpc v1.27.0
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.3.0
	rsc.io/goversion v1.2.0
)
