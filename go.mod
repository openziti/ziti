module github.com/openziti/edge

go 1.15

//replace github.com/openziti/foundation => ../foundation

//replace github.com/openziti/fabric => ../fabric

//replace github.com/openziti/sdk-golang => ../sdk-golang

//replace github.com/kataras/go-events => ../go-events

require (
	github.com/AppsFlyer/go-sundheit v0.2.0
	github.com/Jeffail/gabs v1.4.0
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/blang/semver v3.5.1+incompatible
	github.com/coreos/go-iptables v0.5.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgryski/dgoogauth v0.0.0-20190221195224-5a805980a5f3
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/go-openapi/errors v0.19.9
	github.com/go-openapi/loads v0.20.1
	github.com/go-openapi/runtime v0.19.26
	github.com/go-openapi/spec v0.20.2
	github.com/go-openapi/strfmt v0.20.0
	github.com/go-openapi/swag v0.19.13
	github.com/go-openapi/validate v0.20.1
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.4
	github.com/google/uuid v1.2.0
	github.com/gorilla/handlers v1.5.1
	github.com/jessevdk/go-flags v1.4.0
	github.com/kataras/go-events v0.0.3-0.20201007151548-c411dc70c0a6
	github.com/mdlayher/netlink v1.1.1
	github.com/michaelquigley/pfxlog v0.0.0-20190813191113-2be43bd0dccc
	github.com/miekg/dns v1.1.35
	github.com/mitchellh/mapstructure v1.4.1
	github.com/netfoundry/secretstream v0.1.2
	github.com/openziti/fabric v0.15.18
	github.com/openziti/foundation v0.15.10
	github.com/openziti/sdk-golang v0.15.13
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/sirupsen/logrus v1.7.0
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.7.0
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.5-0.20200615073812-232d8fc87f50
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68
	google.golang.org/protobuf v1.25.0
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)
