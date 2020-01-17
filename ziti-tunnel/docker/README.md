# Ziti-tunnel Docker Image

This image requires access to a Ziti enrollment token (JWT), and a persistent
volume mounted at "/netfoundry" to save the configuration file that is created
when the one-time enrollment token is consumed.

Variables:

- `NF_REG_NAME`: The name of the identity that ziti-tunnel will assume.

Volumes:

- `/netfoundry`: Configuration files that result from enrollment will be stored
  here. This volume should be persistent unless you don't mind losing the key for
  your enrollment token.

Files:

The enrollment token (jwt) must be mounted into the ziti-tunnel container as a volume.
The token must be in a file named `${NF_REG_NAME}.jwt` that must be in one of the
following directories:

- `/netfoundry`: This would be used when running in the Docker engine (or IoT Edge).
   This could be a bind mount or a docker volume.
- `/var/run/secrets/netfoundry.io/enrollment-token`: When running in Kubernetes,
   the enrollment token should be mounted as a secret at this mount point.

### Docker:

    $ mkdir ./ziti_id
    $ cp ~/Downloads/ziti_id.jwt ./ziti_id
    $ docker run -t --network=host --cap-add=NET_ADMIN \
        -v $(pwd)/ziti_id:/netfoundry \
        -e NF_REG_NAME=ziti_id \
        netfoundry/ziti-tunnel:0.5.7-2546

### Kubernetes


### Azure IoT Hub

