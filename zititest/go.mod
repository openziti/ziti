module github.com/openziti/ziti/zititest

go 1.24.7

// use parent project
replace github.com/openziti/ziti => ../

// pinned
replace github.com/openziti/dilithium => github.com/openziti/dilithium v0.3.5

replace github.com/michaelquigley/pfxlog => github.com/michaelquigley/pfxlog v0.6.10

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/Jeffail/gabs/v2 v2.7.0
	github.com/go-openapi/runtime v0.29.0
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/michaelquigley/pfxlog v1.0.0
	github.com/openziti/agent v1.0.32
	github.com/openziti/channel/v4 v4.2.41
	github.com/openziti/edge-api v0.26.50
	github.com/openziti/fablab v0.5.115
	github.com/openziti/foundation/v2 v2.0.79
	github.com/openziti/identity v1.0.118
	github.com/openziti/metrics v1.4.2
	github.com/openziti/sdk-golang v1.2.10
	github.com/openziti/storage v0.4.28
	github.com/openziti/transport/v2 v2.0.198
	github.com/openziti/ziti v1.6.2
	github.com/orcaman/concurrent-map/v2 v2.0.1
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.10.1
	github.com/stretchr/testify v1.11.1
	go.etcd.io/bbolt v1.4.3
	golang.org/x/net v0.46.0
	google.golang.org/protobuf v1.36.10
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/AppsFlyer/go-sundheit v0.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.10.0 // indirect
	github.com/Azure/go-amqp v1.4.0 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/MichaelMure/go-term-markdown v0.1.4 // indirect
	github.com/MichaelMure/go-term-text v0.3.1 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.55.7 // indirect
	github.com/biogo/store v0.0.0-20200525035639-8c94ae1e7c9c // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmatcuk/doublestar/v4 v4.9.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/c-bata/go-prompt v0.2.6 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/coreos/go-iptables v0.8.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/dgoogauth v0.0.0-20190221195224-5a805980a5f3 // indirect
	github.com/dineshappavoo/basex v0.0.0-20170425072625-481a6f6dc663 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ef-ds/deque v1.0.4 // indirect
	github.com/eliukblau/pixterm/pkg/ansimage v0.0.0-20191210081756-9fb6cf8c2f75 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa // indirect
	github.com/gaissmai/extnetip v1.2.0 // indirect
	github.com/go-acme/lego/v4 v4.25.2 // indirect
	github.com/go-chi/chi/v5 v5.2.3 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/analysis v0.24.0 // indirect
	github.com/go-openapi/errors v0.22.3 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.2 // indirect
	github.com/go-openapi/loads v0.23.1 // indirect
	github.com/go-openapi/spec v0.22.0 // indirect
	github.com/go-openapi/strfmt v0.24.0 // indirect
	github.com/go-openapi/swag v0.25.1 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.1 // indirect
	github.com/go-openapi/swag/conv v0.25.1 // indirect
	github.com/go-openapi/swag/fileutils v0.25.1 // indirect
	github.com/go-openapi/swag/jsonname v0.25.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.1 // indirect
	github.com/go-openapi/swag/loading v0.25.1 // indirect
	github.com/go-openapi/swag/mangling v0.25.1 // indirect
	github.com/go-openapi/swag/netutils v0.25.1 // indirect
	github.com/go-openapi/swag/stringutils v0.25.1 // indirect
	github.com/go-openapi/swag/typeutils v0.25.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.1 // indirect
	github.com/go-openapi/validate v0.25.0 // indirect
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/gomarkdown/markdown v0.0.0-20250810172220-2e2c11897d1a // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.2 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/raft v1.7.3 // indirect
	github.com/hashicorp/raft-boltdb v0.0.0-20220329195025-15018e9b97e0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/influxdata/influxdb-client-go/v2 v2.14.0 // indirect
	github.com/influxdata/influxdb1-client v0.0.0-20191209144304-8bf82d3c094d // indirect
	github.com/influxdata/line-protocol v0.0.0-20200327222509-2487e7298839 // indirect
	github.com/jedib0t/go-pretty/v6 v6.6.8 // indirect
	github.com/jessevdk/go-flags v1.6.1 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/judedaryl/go-arrayutils v0.0.1 // indirect
	github.com/kataras/go-events v0.0.3 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kyokomi/emoji/v2 v2.2.13 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lucsky/cuid v1.2.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mattn/go-tty v0.0.3 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/michaelquigley/figlet v0.0.0-20191015203154-054d06db54b4 // indirect
	github.com/miekg/dns v1.1.68 // indirect
	github.com/miekg/pkcs11 v1.1.1 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/muhlemmer/httpforwarded v0.1.0 // indirect
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/natefinch/npipe v0.0.0-20160621034901-c1b8fa8bdcce // indirect
	github.com/oapi-codegen/runtime v1.0.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/openziti-incubator/cf v0.0.3 // indirect
	github.com/openziti/cobra-to-md v1.0.1 // indirect
	github.com/openziti/dilithium v0.3.5 // indirect
	github.com/openziti/jwks v1.0.6 // indirect
	github.com/openziti/runzmd v1.0.82 // indirect
	github.com/openziti/secretstream v0.1.41 // indirect
	github.com/openziti/x509-claims v1.0.3 // indirect
	github.com/openziti/xweb/v2 v2.3.4 // indirect
	github.com/openziti/ziti-db-explorer v1.1.3 // indirect
	github.com/parallaxsecond/parsec-client-go v0.0.0-20221025095442-f0a77d263cf9 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/pkg/sftp v1.13.9 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rodaine/table v1.0.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.1.7 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/speps/go-hashids v2.0.0+incompatible // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/spf13/viper v1.20.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/teris-io/shortid v0.0.0-20201117134242-e59966efd125 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zitadel/logging v0.6.2 // indirect
	github.com/zitadel/oidc/v3 v3.45.0 // indirect
	github.com/zitadel/schema v1.3.1 // indirect
	go.mongodb.org/mongo-driver v1.17.4 // indirect
	go.mozilla.org/pkcs7 v0.9.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go4.org v0.0.0-20180809161055-417644f6feb5 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/image v0.30.0 // indirect
	golang.org/x/mod v0.28.0 // indirect
	golang.org/x/oauth2 v0.32.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/term v0.36.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	golang.org/x/tools v0.37.0 // indirect
	gopkg.in/AlecAivazis/survey.v1 v1.8.8 // indirect
	gopkg.in/resty.v1 v1.12.0 // indirect
	nhooyr.io/websocket v1.8.17 // indirect
	rsc.io/goversion v1.2.0 // indirect
)
