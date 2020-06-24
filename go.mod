module github.com/openziti/edge

go 1.14

// replace github.com/openziti/foundation => ../foundation

// replace github.com/openziti/fabric => ../fabric

// replace github.com/openziti/sdk-golang => ../sdk-golang

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/coreos/go-iptables v0.4.5
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/go-openapi/errors v0.19.6
	github.com/go-openapi/loads v0.19.5
	github.com/go-openapi/runtime v0.19.19
	github.com/go-openapi/spec v0.19.8
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/gobuffalo/packr v1.30.1
	github.com/golang-migrate/migrate v3.5.4+incompatible
	github.com/golang/protobuf v1.3.5
	github.com/google/go-cmp v0.5.0
	github.com/google/uuid v1.1.1
	github.com/gorilla/handlers v1.4.2
	github.com/jessevdk/go-flags v1.4.0
	github.com/kataras/go-events v0.0.3-0.20170604004442-17d67be645c3
	github.com/mdlayher/netlink v1.1.0
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/miekg/dns v1.1.29
	github.com/mitchellh/mapstructure v1.3.2
	github.com/openziti/fabric v0.11.52
	github.com/openziti/foundation v0.11.10
	github.com/openziti/sdk-golang v0.13.16
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v0.0.7
	github.com/stretchr/testify v1.6.1
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.5-0.20200615073812-232d8fc87f50
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/sys v0.0.0-20200615200032-f1bc736245b1
	gopkg.in/resty.v1 v1.12.0
)
