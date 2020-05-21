module github.com/netfoundry/ziti-edge

go 1.14

// replace github.com/netfoundry/ziti-foundation => ../ziti-foundation

// replace github.com/netfoundry/ziti-fabric => ../ziti-fabric

// replace github.com/netfoundry/ziti-sdk-golang => ../ziti-sdk-golang

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/antlr/antlr4 v0.0.0-20191115170859-54daca92f7b0
	github.com/coreos/go-iptables v0.4.5
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/gobuffalo/packr v1.30.1
	github.com/golang-migrate/migrate v3.5.4+incompatible
	github.com/golang/protobuf v1.3.5
	github.com/google/go-cmp v0.4.1
	github.com/google/uuid v1.1.1
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.7.4
	github.com/jinzhu/gorm v1.9.12
	github.com/kataras/go-events v0.0.3-0.20170604004442-17d67be645c3
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/mdlayher/netlink v1.1.0
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/miekg/dns v1.1.29
	github.com/mitchellh/mapstructure v1.3.0
	github.com/netfoundry/ziti-fabric v0.11.32
	github.com/netfoundry/ziti-foundation v0.10.0
	github.com/netfoundry/ziti-sdk-golang v0.11.35
	github.com/oleiade/reflections v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.5.1
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.4
	golang.org/x/crypto v0.0.0-20200204104054-c9f3fb736b72
	golang.org/x/sys v0.0.0-20200202164722-d101bd2416d5
	gopkg.in/Masterminds/squirrel.v1 v1.0.0-20170825200431-a6b93000bd21
	gopkg.in/oleiade/reflections.v1 v1.0.0
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.2.8
)
