module github.com/openziti/ziti/v2

go 1.25.3

// pinned
replace github.com/michaelquigley/pfxlog => github.com/michaelquigley/pfxlog v0.6.10

replace github.com/openziti/foundation/v2 => github.com/openziti/foundation/v2 v2.0.86

require (
	github.com/AppsFlyer/go-sundheit v0.6.0
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.0
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.10.0
	github.com/Jeffail/gabs v1.4.0
	github.com/Jeffail/gabs/v2 v2.7.0
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/antchfx/jsonquery v1.3.6
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/coreos/go-iptables v0.8.0
	github.com/dgryski/dgoogauth v0.0.0-20190221195224-5a805980a5f3
	github.com/dineshappavoo/basex v0.0.0-20170425072625-481a6f6dc663
	github.com/ef-ds/deque v1.0.4
	github.com/fatih/color v1.18.0
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/gaissmai/extnetip v1.3.1
	github.com/go-acme/lego/v4 v4.31.0
	github.com/go-jose/go-jose/v4 v4.1.3
	github.com/go-openapi/errors v0.22.6
	github.com/go-openapi/jsonpointer v0.22.4
	github.com/go-openapi/loads v0.23.2
	github.com/go-openapi/runtime v0.29.2
	github.com/go-openapi/spec v0.22.3
	github.com/go-openapi/strfmt v0.25.0
	github.com/go-openapi/swag v0.25.4
	github.com/go-openapi/swag/jsonutils v0.25.4
	github.com/go-openapi/validate v0.25.1
	github.com/go-resty/resty/v2 v2.17.1
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/go-cmp v0.7.0
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/gorilla/handlers v1.5.2
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/securecookie v1.1.2
	github.com/gorilla/websocket v1.5.3
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/raft v1.7.3
	github.com/hashicorp/raft-boltdb/v2 v2.3.1
	github.com/jedib0t/go-pretty/v6 v6.7.8
	github.com/jessevdk/go-flags v1.6.1
	github.com/jinzhu/copier v0.4.0
	github.com/judedaryl/go-arrayutils v0.0.1
	github.com/kataras/go-events v0.0.3
	github.com/lucsky/cuid v1.2.1
	github.com/mdlayher/netlink v1.8.0
	github.com/michaelquigley/pfxlog v1.0.0
	github.com/miekg/dns v1.1.72
	github.com/mitchellh/mapstructure v1.5.0
	github.com/natefinch/lumberjack v2.0.0+incompatible
	github.com/openziti/agent v1.0.33
	github.com/openziti/channel/v4 v4.3.4
	github.com/openziti/cobra-to-md v1.0.1
	github.com/openziti/edge-api v0.26.53
	github.com/openziti/foundation/v2 v2.0.87
	github.com/openziti/identity v1.0.125
	github.com/openziti/jwks v1.0.6
	github.com/openziti/metrics v1.4.3
	github.com/openziti/runzmd v1.0.89
	github.com/openziti/sdk-golang v1.4.1
	github.com/openziti/secretstream v0.1.47
	github.com/openziti/storage v0.4.38
	github.com/openziti/transport/v2 v2.0.209
	github.com/openziti/x509-claims v1.0.3
	github.com/openziti/xweb/v3 v3.0.3
	github.com/openziti/ziti-db-explorer v1.1.3
	github.com/orcaman/concurrent-map/v2 v2.0.1
	github.com/pkg/errors v0.9.1
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
	github.com/russross/blackfriday v1.6.0
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/sirupsen/logrus v1.9.4
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/teris-io/shortid v0.0.0-20220617161101-71ec9f2aa569
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/zitadel/oidc/v3 v3.45.4
	go.etcd.io/bbolt v1.4.3
	go.uber.org/atomic v1.11.0
	go4.org v0.0.0-20260112195520-a5071408f32f
	golang.org/x/crypto v0.48.0
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96
	golang.org/x/net v0.50.0
	golang.org/x/oauth2 v0.35.0
	golang.org/x/sync v0.19.0
	golang.org/x/sys v0.41.0
	golang.org/x/term v0.40.0
	golang.org/x/text v0.34.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
	gopkg.in/resty.v1 v1.12.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	rsc.io/goversion v1.2.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/go-amqp v1.5.1 // indirect
	github.com/MichaelMure/go-term-text v0.3.1 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antchfx/xpath v1.3.5 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/biogo/store v0.0.0-20200525035639-8c94ae1e7c9c // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/c-bata/go-prompt v0.2.6 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.4.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/creack/pty v1.1.11 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-chi/chi/v5 v5.2.5 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/analysis v0.24.2 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/gomarkdown/markdown v0.0.0-20250810172220-2e2c11897d1a // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.5 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/kyokomi/emoji/v2 v2.2.13 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/mattn/go-tty v0.0.3 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/miekg/pkcs11 v1.1.2 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/muhlemmer/httpforwarded v0.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/openziti-incubator/cf v0.0.3 // indirect
	github.com/openziti/dilithium v0.3.5 // indirect
	github.com/openziti/go-term-markdown v1.0.1 // indirect
	github.com/parallaxsecond/parsec-client-go v0.0.0-20221025095442-f0a77d263cf9 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pion/dtls/v3 v3.0.10 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/transport/v4 v4.0.1 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rodaine/table v1.0.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.7 // indirect
	github.com/speps/go-hashids v2.0.0+incompatible // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zitadel/logging v0.7.0 // indirect
	github.com/zitadel/schema v1.3.2 // indirect
	go.mongodb.org/mongo-driver v1.17.7 // indirect
	go.mozilla.org/pkcs7 v0.9.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	nhooyr.io/websocket v1.8.17 // indirect
)
