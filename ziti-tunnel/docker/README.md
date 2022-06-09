# Ziti-tunnel Docker Image

Run the ziti-tunnel NetFoundry client. ziti-tunnel captures network traffic that
is destined for Ziti services and proxes the packet payloads to the associated
Ziti service. See the [ziti-tunnel README](../README.md) for more details.

This image requires access to a Ziti enrollment token (JWT), and a persistent
volume mounted at "/netfoundry" to save the configuration file that is created
when the one-time enrollment token is consumed.

## Variables

- `NF_REG_NAME`: Required - file basename of the enroll token JWT file or identity config JSON file that ziti-tunnel will use
- `NF_REG_TOKEN`: Optional - one-time enrollment token to use instead of `${NF_REG_NAME}.jwt`

If `${NF_REG_NAME}.jwt` does not exist and `NF_REG_TOKEN` is not defined the enroller will try to get the token from
stdin if it is not a TTY.

## Volumes

- `/netfoundry`: Configuration files that result from enrollment will be stored
  here. This volume should be persistent unless you don't mind losing the key for
  your enrollment token.

## Files

The enrollment token (jwt) must be mounted into the ziti-tunnel container as a volume.
The token must be in a file named `${NF_REG_NAME}.jwt` that must be in one of the
following directories:

- `/netfoundry`: This would be used when running in the Docker engine (or IoT Edge).
   This could be a bind mount or a docker volume.
- `/enrollment-token`: Alternative mount point for the enrollment token only.
- `/var/run/secrets/netfoundry.io/enrollment-token`: When running in Kubernetes,
   the enrollment token should be mounted as a secret at this mount point.

## Examples

### Docker

The ziti-tunnel image can be used in a vanilla Docker environment.

    # enroll with token from inherited env var and run hosting-only mode without any nameserver or IP routes
    $ mkdir ./ziti_id
    $ export NF_REG_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbSI6Im90dCIsImV4cCI6MTY1MzE2NzcwOCwiaXNzIjoiaHR0cHM6Ly81MGEyMDc4Zi01MGQyLTRhZTAtYWI4Ny0wYTdjMjM1OWVjOTYucHJvZHVjdGlvbi5uZXRmb3VuZHJ5LmlvOjQ0MyIsImp0aSI6IjFiMjJkMzc2LTMzMWItNDMxNS1iZTFiLWJkOTUzYThiYWM4ZiIsInN1YiI6InRGLnZnLjdwM1kifQ.TWbk3-kjBRKwQCXMoD93sXtyQhZOMZ1iJzV73Sqft-cEOkV2kjbA4TBRwl0nuLPCdJqkhPl9Yc1WG7YaYWYXV4ghE1Hk0Gta_HlpWdNjNlB1cVrzMyxaoCXhaX5xqGnwDuqfOK7q6DItuNsKouM2G6KKZvhGOacax6TvP-sunsxFz6AdYQJizNBoL5fJ14r1_O6yczGd5GSd8x9-eP5rNMQuRdUtiu69b-rEu1gO2SXVTrADTD5p8sP9khbT_eQzIDD9jgagXoJJvOKTVdsUAWS7YKfk1On0BpxNvv30bQ6eAkliwU7GTXDR2IPW-blZYt1Wtf3sgeuTDCCtVO_7gjSn7WM7YJqpsB72V-43Xz8I7LCDa0u48baSmmpPDUSphIBDa_nksPZk8jfwx4pHoHYbSbD4r47Af_9P-JUQRT8hzNuvktG56kmcqVCfEZHT-4xgK0Lvxxp4mzqdcNyCB0VwC6kdk7OlxwqmftDWrJhNuxKMy2MtyTjG0mwHxt4P0y_fZo1ZYJZU5AzvxrE9OLTjsQ4nV5NliJ3Qxaw_taCaIWxn98BeiHfAiIc3EL7bWGjAfXz-XWvWG8-AcCmK-cxIj7Pvj0U7lIWH7bVcRmVVc0fQnXdACkeQe7ixWr4IopV5YJ607zm6Qk_am_MrERla6xyXblvroWIITN2aEUk
    $ docker run --rm \
        -v $(pwd)/ziti_id:/netfoundry \
        -e NF_REG_NAME=ziti_id \
        -e NF_REG_TOKEN \
        openziti/ziti-tunnel:latest

    # enroll with a JWT file and run a transparent proxy with built-in nameserver
    $ mkdir ./ziti_id
    $ cp ~/Downloads/ziti_id.jwt ./ziti_id
    $ docker run -t --network=host --cap-add=NET_ADMIN \
        -v $(pwd)/ziti_id:/netfoundry \
        -e NF_REG_NAME=ziti_id \
        openziti/ziti-tunnel:latest

Notes:

- The container that runs ziti-tunnel will only be able to intercept traffic for other processes within the same
  container, unless the container uses the "host" network mode.
- No special network mode or additional capabilities are required when running `ziti-tunnel host`, which is the
  hosting-only run mode without any built-in nameserver or IP routers or iptables rules
- The container requires NET_ADMIN capability to create iptables rules in the host network when running in the default
  mode `ziti-tunnel tproxy` (implied by `ziti-tunnel run`).
- The `NF_REG_NAME` environment variable must be set to the name of the ziti identity that ziti-tunnel will assume when
  connecting to the controller.
- The "/netfoundry" directory must be mounted on its own volume.
  - The one-time enrollment token "/netfoundry/${NF_REG_NAME}.jwt" is sought by the enroller when the container is
    started for the first time. This is the JWT that was downloaded from the controller when the Ziti identity was
    created.
  - If the JWT file is not found in one of the well-known directories then the token is subsequently sought from the
    env var `NF_REG_TOKEN`, then stdin.
  - "/netfoundry/${NF_REG_NAME}.json" is created when the identity is enrolled. The "/netfoundry" volume must be
    persistent (that is, ${NF_REG_NAME}.json must endure container restarts), since the enrollment token is only valid
    for one enrollment.

### Docker Compose

This example uses Compose to store in a file the Docker `build` and `run` parameters for several modes of operation of `ziti-tunnel`.

#### Docker Compose Setup

1. Install Docker Engine.
2. `docker-compose` is a utility you can install with the Python Package Index (PyPi) e.g. `pip install --upgrade
   docker-compose`.
3. Save your Ziti identity enrollment token e.g. `my-ziti-identity-file.jwt` in the same directory as the file named
   `docker-compose.yml`. Your identity will be enrolled the first time you run the container, and the permanent
   identity file will be saved e.g. `my-ziti-identity-file.json`.
4. You may change `ZITI_VERSION` to [another release version from our ziti-release
   repository](https://netfoundry.jfrog.io/ui/repos/simple/Properties/ziti-release%2Fziti-tunnel%2Famd64%2Flinux%2F0.15.2%2Fziti-tunnel.tar.gz).

#### Docker Transparent Proxy for Linux

1. Modify "ziti-tproxy" under "services" in the file named `docker-compose.yml` in this Git repo, optionally overriding
   the default value of `command` to pass additional parameters to `ziti-tunnel`.

```yaml
version: "3.3"
services:
    ziti-tproxy:
        image: netfoundry/ziti-tunnel:tproxy
        build:
            context: .
            args:
                ZITI_VERSION: 0.15.3
        volumes:
        - .:/netfoundry
        network_mode: host
        cap_add:
        - NET_ADMIN
        environment:
        - NF_REG_NAME
        - NF_REG_TOKEN
#        command: run --resolver udp://127.0.0.123:53
#        command: run --resolver none
```

2. Run `NF_REG_NAME=my-ziti-identity-file docker-compose up --build ziti-tproxy` in the same directory.

This will cause the container to configure the Linux host to transparently proxy any domain names or IP addresses that match a Ziti service.

#### Docker Proxy or MacOS or Windows

1. Modify "ziti-proxy" under "services" in the file named `docker-compose.yml` in this Git repo. Change the service
   name(s) and port number(s) to suit your actual services. You must align the mapped ports under `ports` with the
   bound ports in the `command`.

```yaml
version: "3.3"
services:
    ziti-proxy:
        image: netfoundry/ziti-tunnel:proxy
        build:
            context: .
            args:
                ZITI_VERSION: 0.15.3
        volumes:
        - .:/netfoundry
        environment:
        - NF_REG_NAME
        - NF_REG_TOKEN
        ports:
        - "8888:8888"
        - "9999:9999"
        command: proxy "my example service":8888 "my other example service":9999
```

2. Run in the same directory:

```bash
NF_REG_NAME=my-ziti-identity-file docker-compose up --build ziti-proxy
```

This will cause the container to listen on the mapped port(s) and proxy any received traffic to the Ziti service that
are bound to that port.

### Kubernetes

The ziti-tunnel image can be used in Kubernetes either as a sidecar, which would
intercept packets only from other containers in the pod definition or as a dedicated
pod that sets `hostNetwork` to true.

The following example manifest shows how to run ziti-tunnel as a sidecar:

    $ cat app-with-ziti-tunnel-sidecar.yaml

```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: tunnel-sidecar-pv-claim
    spec:
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 1Gi
    ---
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: ziti-tunnel-sidecar-demo
    spec:
      replicas: 1
      selector:
        matchLabels:
          app: ziti-tunnel-sidecar-demo
      strategy:
        type: Recreate
      template:
        metadata:
          labels:
            app: ziti-tunnel-sidecar-demo
        spec:
          containers:
          - image: centos
            name: testclient
            command: ["sh","-c","while true; set -x; do curl -sSLf ethzero.ziti.ui 2>&1; set +x; sleep 5; done"]
          - image: netfoundry/ziti-tunnel:0.5.8-2554
            name: ziti-tunnel
            env:
            - name: NF_REG_NAME
              value: tunnel-sidecar
            volumeMounts:
            - name: tunnel-sidecar-jwt
              mountPath: "/var/run/secrets/netfoundry.io/enrollment-token"
              readOnly: true
            - name: ziti-tunnel-persistent-storage
              mountPath: /netfoundry
            securityContext:
              capabilities:
              add:
                - NET_ADMIN
          dnsPolicy: "None"
          dnsConfig:
            nameservers:
              - 127.0.0.1
              - 8.8.8.8
          restartPolicy: Always
          volumes:
          - name: ziti-tunnel-persistent-storage
            persistentVolumeClaim:
              claimName: tunnel-sidecar-pv-claim
          - name: tunnel-sidecar-jwt
            secret:
              secretName: tunnel-sidecar.jwt
```

### Azure IoT Hub

This image can be used to run a module on an Azure IoT runtime.

    # createOption.HostConfig for tun (instead of tproxy): \"Devices\":[{\"PathOnHost\":\"/dev/net/tun\",\"PathInContainer\":\"/dev/net/tun\",\"CgroupPermissions\":\"rwm\"}],


    $ cat module.json

```json
    {
        "modulesContent": {
            "$edgeAgent": {
                "properties.desired": {
                    "modules": {
                        "ziti-tunnel": {
                            "settings": {
                                "image": "netfoundry/ziti-tunnel:0.5.8-2554",
                                "createOptions": "{\"HostConfig\":{\"CapAdd\":[\"NET_ADMIN\"],\"Mounts\":[{\"Type\":\"bind\",\"Source\":\"/opt/netfoundry\",\"Target\":\"/netfoundry\"}],\"NetworkMode\":\"host\"},\"NetworkingConfig\":{\"EndpointsConfig\":{\"host\":{}}}}"
                            },
                            "type": "docker",
                            "version": "1.0",
                            "status": "running",
                            "restartPolicy": "always"
                        }
                    },
                    "runtime": {     
                        "settings": {
                            "minDockerVersion": "v1.25"
                        },
                        "type": "docker"
                    },
                    "schemaVersion": "1.0",
                    "systemModules": {
                        "edgeAgent": {
                            "settings": {
                                "image": "mcr.microsoft.com/azureiotedge-agent:1.0",
                                "createOptions": ""
                            },
                            "type": "docker"
                        },
                        "edgeHub": {
                            "settings": {
                                "image": "mcr.microsoft.com/azureiotedge-hub:1.0",
                                "createOptions": "{\"HostConfig\":{\"PortBindings\":{\"443/tcp\":[{\"HostPort\":\"443\"}],\"5671/tcp\":[{\"HostPort\":\"5671\"}],\"8883/tcp\":[{\"HostPort\":\"8883\"}]}}}"
                            },
                            "type": "docker",
                            "status": "running",
                            "restartPolicy": "always"
                        }
                    }
                }
            },
            "$edgeHub": {
                "properties.desired": {
                    "routes": {},
                    "schemaVersion": "1.0",
                    "storeAndForwardConfiguration": {
                        "timeToLiveSecs": 7200
                    }
                }
            },
            "ziti-tunnel": {
                "properties.desired": {}
            }
        }
    }
```
