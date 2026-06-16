#!/usr/bin/env bash
#
# create-model.sh - create a representative data model through the ziti CLI / REST API for the
# service-collapse migration gate. Creating data via the API (not direct store writes) is the
# truthful way: it runs the same validation, defaulting, config-schema, policy-denormalization, and
# event paths a real install does, so the data -- and any migration problem it triggers -- is
# realistic.
#
# Run against a RUNNING pre-collapse controller (you must be logged in first, or set ZITI_USER /
# ZITI_PWD / ZITI_CTRL and this script will log in). Usage:
#   ZITI=/path/to/ziti ZITI_CTRL=localhost:1280 ./create-model.sh
#
# Coverage matrix (each value 0/1/2 and each index multiplicity is exercised by at least one entity):
#   role attributes per service ........ 0: svc-min   1: svc-mid   2: svc-rich
#   configs per service ................ 0: svc-min   1: svc-mid   2: svc-rich
#   config -> services (reverse) ....... 0: cfg-unused 1: cfg-solo  2: cfg-shared
#   terminators per service ............ 0: svc-min   1: svc-mid   2: svc-rich (+ fab-with-terms)
#   dial policies per service .......... 1: svc-min (#all)          2: svc-mid (#all+#web)
#   bind policies per service .......... 1: svc-min (#all)          2: svc-mid (#all+#web)
#   SERPs per service .................. 1: svc-min (#all)          2: svc-mid (#all+#web)
#   dial-identity refcount ............. 1: id-user->svc-mid        2: id-web->svc-mid
#   bind-identity refcount ............. 1: id-host->svc-min        2: id-host->svc-mid
#   edge-router refcount (svc->er) ..... 1: svc-min->er-pub         2: svc-mid->er-pub
#   edge-router -> services (reverse) .. 0: er-priv   2+: er-pub
#   identity -> dial services .......... 0: id-none   2+: id-user
#   identityServices (config overrides). 0: svc-min   1: svc-mid   2: svc-rich   (if CLI supports)
#   role-attr index key members ........ 1: "api" (svc-rich)        2: "web" (svc-mid, svc-rich)
#   encryptionRequired ................. true: svc-min,svc-rich     false: svc-mid
#   isFabricOnly (0 edge links) ........ fab-with-terms, fab-no-terms (excluded from all edge links)
#
set -euo pipefail
ZITI="${ZITI:-ziti}"
ZITI_CTRL="${ZITI_CTRL:-localhost:1280}"

if [ -n "${ZITI_USER:-}" ]; then
  "$ZITI" edge login "$ZITI_CTRL" -u "$ZITI_USER" -p "${ZITI_PWD:-admin}" -y
fi

e() { "$ZITI" edge "$@"; }

echo "== config types: use built-in host.v1 / intercept.v1 =="

echo "== configs (config->services reverse: 0/1/2) =="
e create config cfg-unused host.v1 '{"address":"localhost","port":1,"protocol":"tcp"}'        # 0 services
e create config cfg-shared host.v1 '{"address":"localhost","port":8080,"protocol":"tcp"}'     # 2 services
e create config cfg-solo intercept.v1 '{"protocols":["tcp"],"addresses":["svc.test"],"portRanges":[{"low":80,"high":80}]}' # 1 service

echo "== edge routers =="
e create edge-router er-pub  -a pub
e create edge-router er-priv -a priv

echo "== identities =="
e create identity id-user -a users
e create identity id-web  -a users,web-users
e create identity id-host -a hosts
e create identity id-none

echo "== edge services (role attrs / configs / encryption / terminator-count vary) =="
e create service svc-min  -e ON                                                # 0 attrs, 0 configs
e create service svc-mid  -a web        -c cfg-shared          -e OFF          # 1 attr, 1 config
e create service svc-rich -a web,api    -c cfg-shared,cfg-solo -e ON           # 2 attrs, 2 configs

echo "== service policies (#all gives every edge svc refcount 1; #web overlap gives web svcs refcount 2) =="
e create service-policy sp-dial-all Dial --identity-roles '#users'     --service-roles '#all'
e create service-policy sp-dial-web Dial --identity-roles '#web-users' --service-roles '#web'
e create service-policy sp-bind-all Bind --identity-roles '#hosts'     --service-roles '#all'
e create service-policy sp-bind-web Bind --identity-roles '#hosts'     --service-roles '#web'

echo "== service edge router policies (#all + #web overlap on er-pub => edge-router refcount 2 for web svcs) =="
e create service-edge-router-policy serp-all --service-roles '#all' --edge-router-roles '#pub'
e create service-edge-router-policy serp-web --service-roles '#web' --edge-router-roles '#pub'

echo "== fabric / management services (isFabricOnly; excluded from all edge links) =="
"$ZITI" fabric create service fab-with-terms
"$ZITI" fabric create service fab-no-terms

echo "== identity service-config overrides (identityServices set: 0/1/2 identities per service) =="
# svc-min: 0 overrides; svc-mid: 1 (id-user); svc-rich: 2 (id-user, id-web). cfg-unused (host.v1) is
# the override config -- this populates the service's identityServices back-link set and the config's
# identityServices compound links (config.services, used above, is unaffected).
# `update identity-configs <identity> <service> <config>`.
e update identity-configs id-user svc-mid  cfg-unused
e update identity-configs id-user svc-rich cfg-unused
e update identity-configs id-web  svc-rich cfg-unused

echo "== terminators (service->router fabric link: 0/1/2). Not moved by the migration, but verifies it is left intact =="
# `fabric create terminator <service> <router> <address>` (binding defaults to edge_transport).
# svc-min: 0, svc-mid: 1, svc-rich: 2; fab-with-terms: 1, fab-no-terms: 0.
"$ZITI" fabric create terminator svc-mid         er-pub  tcp:localhost:1001
"$ZITI" fabric create terminator svc-rich        er-pub  tcp:localhost:1002
"$ZITI" fabric create terminator svc-rich        er-priv tcp:localhost:1003
"$ZITI" fabric create terminator fab-with-terms  er-pub  tcp:localhost:1004

echo "model created"
