#!/usr/bin/env bash

# this one-shot script demonstrates how to use a ziti router as a transparent proxy sidecar

set -o errexit -o nounset -o pipefail #-o xtrace

function cleanup() {
    if ! (( I_AM_ROBOT ))
    then
        echo "WARNING: destroying docker volumes in 30s; set I_AM_ROBOT=1 to suppress this message" >&2
        sleep 30
    fi
    docker compose --profile host --profile client --profile ziti down --volumes
}

function bye() {
    echo "Leaving $PWD"
}

: "${I_AM_ROBOT:=0}"

cd "$(mktemp -d)"

trap bye EXIT

declare -f cleanup > cleanup.sh
cat << YAML > compose.yml
services:
    ziti-ctrl:
        profiles:
            - ziti
        image: openziti/ziti-cli
        networks:
            testnet:
                aliases:
                    - ziti-controller
                    - ziti-router
        command: >
            edge quickstart
            --home /home/ziggy/quickstart
            --ctrl-address ziti-controller
            --ctrl-port 1280
            --router-address ziti-router
            --router-port 3022
            --password ziggy123
        working_dir: /home/ziggy
        environment:
            HOME: /home/ziggy
        volumes:
            - ziti-ctrl:/home/ziggy/quickstart
        expose:
            - 1280
            - 3022
        healthcheck:
            test:
                - CMD
                - ziti
                - agent
                - stats
            interval: 3s
            timeout: 3s
            retries: 5
            start_period: 30s
    wait-for-ziti-ctrl:
        profiles:
            - ziti
        depends_on:
            ziti-ctrl:
                condition: service_healthy
        image: busybox
        command: echo "INFO Ziti is cooking"

    web-server:
        profiles:
            - host
        image: openziti/hello-world
        expose:
            - 8000
        networks:
            - testnet
    ziti-host:
        profiles:
            - host
        image: openziti/ziti-host
        networks:
            testnet:
        volumes:
            - ziti-host:/ziti-edge-tunnel
        environment:
            - ZITI_ENROLL_TOKEN

    web-client:
        profiles:
            - client
        image: busybox
        network_mode: service:ziti-client
        restart: unless-stopped
        entrypoint:
            - /bin/sh
            - -c
            - |
                wget -qO- http://www.ziti.internal:80/
                sleep 1
        depends_on:
            ziti-client:
                condition: service_healthy
    ziti-client:
        profiles:
            - client
        image: openziti/ziti-router:1.1.9
        expose:
            - 3022
        networks:
            testnet:
        environment:
            ZITI_CTRL_ADVERTISED_ADDRESS: ziti-controller
            ZITI_ENROLL_TOKEN:
            ZITI_ROUTER_MODE: tproxy
        volumes:
            - ziti-client:/ziti-router
        dns:
            - 127.0.0.1
            - 1.1.1.1
        user: root
        cap_add:
            - NET_ADMIN
        healthcheck:
            test:
                - CMD
                - ziti
                - agent
                - stats
            interval: 3s
            timeout: 3s
            retries: 5
            start_period: 30s

networks:
    testnet:

volumes:
    ziti-ctrl:
    ziti-host:
    ziti-client:

YAML

docker compose run --rm --entrypoint= --user=root --no-TTY ziti-ctrl chown -R "2171:2171" /home/ziggy/quickstart/
docker compose up wait-for-ziti-ctrl

docker compose exec --no-TTY ziti-ctrl bash << BASH

set -o errexit -o nounset -o pipefail -o xtrace

ziti edge login https://ziti-controller:1280 \
--ca=/home/ziggy/quickstart/pki/root-ca/certs/root-ca.cert \
--username=admin \
--password=ziggy123 \

ziti edge create edge-router "web-client-router" \
    --tunneler-enabled \
    --jwt-output-file /tmp/web-client-router.erott.jwt \

ziti edge list edge-routers

ziti edge update identity "web-client-router" \
    --role-attributes web-clients

ziti edge create identity "web-host-tunneler" \
    --jwt-output-file /tmp/web-host-tunneler.ott.jwt \
    --role-attributes web-hosts

ziti edge list identities

ziti edge create config "web-client-config" intercept.v1 \
    '{"protocols":["tcp"],"addresses":["www.ziti.internal"], "portRanges":[{"low":80, "high":80}]}'

ziti edge create config "web-host-config" host.v1 \
    '{"protocol":"tcp", "address":"web-server","port":8000}'

ziti edge list configs

ziti edge create service "web-service" \
    --configs web-client-config,web-host-config \
    --role-attributes web-services

ziti edge list services

ziti edge create service-policy "web-host-policy" Bind \
    --service-roles '#web-services' \
    --identity-roles '#web-hosts'

ziti edge create service-policy "web-client-policy" Dial \
    --service-roles '#web-services' \
    --identity-roles '#web-clients'

ziti edge list service-policies

ziti edge list service-edge-router-policies

ziti edge list edge-router-policies

ziti edge policy-advisor services web-service --quiet

BASH

ZITI_ENROLL_TOKEN="$(docker compose exec --no-TTY ziti-ctrl cat /tmp/web-host-tunneler.ott.jwt)" \
docker compose --profile=host up --detach

ZITI_ENROLL_TOKEN="$(docker compose exec --no-TTY ziti-ctrl cat /tmp/web-client-router.erott.jwt)" \
docker compose --profile=client up --detach

timeout 10s docker compose logs web-client --no-log-prefix --follow || true

read -p "Done! Press ENTER to destroy..."

cleanup
