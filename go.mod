module github.com/openziti/edge

go 1.14

//replace github.com/openziti/foundation => ../foundation

//replace github.com/openziti/fabric => ../fabric

//replace github.com/openziti/sdk-golang => ../sdk-golang

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/coreos/go-iptables v0.4.5
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/go-openapi/errors v0.19.6
	github.com/go-openapi/loads v0.19.5
	github.com/go-openapi/runtime v0.19.21
	github.com/go-openapi/spec v0.19.9
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.2
	github.com/google/uuid v1.1.2
	github.com/gorilla/handlers v1.4.2
	github.com/jessevdk/go-flags v1.4.0
	github.com/kataras/go-events v0.0.3-0.20170604004442-17d67be645c3
	github.com/mdlayher/netlink v1.1.0
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/miekg/dns v1.1.31
	github.com/mitchellh/mapstructure v1.3.3
	github.com/netfoundry/secretstream v0.1.2
	github.com/openziti/fabric v0.13.6
	github.com/openziti/foundation v0.14.5
	github.com/openziti/sdk-golang v0.13.44
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.6.1
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.5-0.20200615073812-232d8fc87f50
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)
