module github.com/netfoundry/ziti-cmd

go 1.13

// replace github.com/netfoundry/ziti-foundation => ../ziti-foundation

// replace github.com/netfoundry/ziti-fabric => ../ziti-fabric

// replace github.com/netfoundry/ziti-sdk-golang => ../ziti-sdk-golang

// replace github.com/netfoundry/ziti-edge => ../ziti-edge

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/fatih/color v1.7.0
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.2
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/netfoundry/ziti-edge v0.12.14
	github.com/netfoundry/ziti-fabric v0.11.4
	github.com/netfoundry/ziti-foundation v0.8.1
	github.com/netfoundry/ziti-sdk-golang v0.11.6
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.8.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rs/cors v1.7.0
	github.com/russross/blackfriday v1.5.2
	github.com/shirou/gopsutil v2.19.11+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.5.0
	github.com/stretchr/testify v1.3.0
	github.com/urfave/negroni v1.0.0
	golang.org/x/net v0.0.0-20191126235420-ef20fe5d7933
	google.golang.org/grpc v1.25.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.2.7
	rsc.io/goversion v1.2.0
)
