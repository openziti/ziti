# Ziti-tunnel Docker Image

Run the ziti-tunnel NetFoundry client. ziti-tunnel captures network traffic that
is destined for Ziti services and proxes the packet payloads to the associated
Ziti service. See the [ziti-tunnel README](../README.md) for more details.

This image requires access to a Ziti enrollment token (JWT), and a persistent
volume mounted at "/netfoundry" to save the configuration file that is created
when the one-time enrollment token is consumed.

## Variables

- `NF_REG_NAME`: The name of the identity that ziti-tunnel will assume.

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
- `/var/run/secrets/netfoundry.io/enrollment-token`: When running in Kubernetes,
   the enrollment token should be mounted as a secret at this mount point.

# Examples

## Docker

The ziti-tunnel image can be used in a vanilla Docker environment.

    $ mkdir ./ziti_id
    $ cp ~/Downloads/ziti_id.jwt ./ziti_id
    $ docker run -t --network=host --cap-add=NET_ADMIN \
        -v $(pwd)/ziti_id:/netfoundry \
        -e NF_REG_NAME=ziti_id \
        netfoundry/ziti-tunnel:0.5.7-2546

Notes:

- The container that runs ziti-tunnel will only be able to intercept traffic for
  other processes within the same container, unless the container uses the "host"
  network mode.
- The container requires NET_ADMIN capability to create iptables rules.
- The `NF_REG_NAME` environment variable must be set to the name of the ziti
  identity that ziti-tunnel will assume when connecting to the controller.
- The "/netfoundry" directory must be mounted on its own volume.
  - The one-time enrollment token "/netfoundry/${NF_REG_NAME}.jwt" must exist when
    the container is started for the first time. This is the JWT that was downloaded
    from the controller when the Ziti identity was created.
  - "/netfoundry/${NF_REG_NAME}.json" is created when the identity is enrolled.
    The "/netfoundry" volume must be persistent (that is, ${NF_REG_NAME}.json must
    endure container restarts), since the enrollment token is only valid for one
    enrollment.

## Kubernetes

The ziti-tunnel image can be used in Kubernetes either as a sidecar, which would
intercept packets only from other containers in the pod definition or as a dedicated
pod that sets `hostNetwork` to true.

The following example manifest shows how to run ziti-tunnel as a sidecar:

    $ cat app-with-ziti-tunnel-sidecar.yaml
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


## Azure IoT Hub

This image can be used to run a module on an Azure IoT runtime.

    # createOption.HostConfig for tun (instead of tproxy): \"Devices\":[{\"PathOnHost\":\"/dev/net/tun\",\"PathInContainer\":\"/dev/net/tun\",\"CgroupPermissions\":\"rwm\"}],


    $ cat module.json
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