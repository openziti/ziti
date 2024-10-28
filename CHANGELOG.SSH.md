## What's New

* Ziti Component Management Access (Experimental)

## Ziti Component Management Access

This release contains an experimental feature allowing Ziti Administrators to allow access to management services for ziti components.

This initial release is focused on providing access to SSH, but other management tools could potentially use the same data pipe.

### Why

Ideally one shouldn't use a system to manage itself. However, it can be nice to have a backup way to access a system, when things 
go wrong. This could also be a helpful tool for small installations. 

Accessing controllers and routers via the management plane and control plane is bad from a separation of data concerns perspective,
but good from minimizing requirements perspective. To access a Ziti SSH service, An SDK client needs access to the REST API, the 
edge router with a control channel connection and links to the public routers. With this solution, only the REST API and the control
channel are needed.
 
### Security

In order to access a component the following is required:

1. The user must be a Ziti administrator
2. The user must be able to reach the Fabric Management API (which can be locked down)
3. The feature must be enabled on the controller used for access
4. The feature must be enabled on the destination component
5. A destination must be configured on the destination component
6. The destination must be to a port on 127.0.0.1. This can't be used to access external systems.
8. The user must have access to the management component. If SSH, this would be an SSH key or other SSH credentials
9. If using SSH, the SSH server only needs to listen on the loopback interface. So SSH doesn't need to be listening on the network

**Warnings**
1. If you do not intend to use the feature, do not enable it.
2. If you enable the feature, follow best practices for good SSH hygiene (audit logs, locked down permissions, etc)

### What's the Data Flow?

The path for accessing controllers is: 

* Ziti CLI to
* Controller Fabric Management API to
* a network service listing on the loopback interface, such as SSH.

The path for accessing routers is: 

* Ziti CLI to
* Controller Fabric Management API to 
* a router via the control channel to
* a network service listing on the loopback interface, such as SSH.

What does this look like? 

Each controller you want to allow access through, must enable the feature.

Example controller config:

```
mgmt:
  pipe:
    enabled: true
    enableExperimentalFeature: true 
    destination: 127.0.0.1:22
```

Note that if you want to allow access through the controller, but not to the controller itself, you can 
leave out the `destination` setting.

The router config is identical.

```
mgmt:
  pipe:
    enabled: true
    enableExperimentalFeature: true
    destination: 127.0.0.1:22
```

### SSH Access

If your components are set up to point to an SSH server, you can access them as follows:


```
    ziti fabric ssh  --key /path/to/keyfile ctrl_client
    ziti fabric ssh  --key /path/to/keyfile ubuntu@ctrl_client
    ziti fabric ssh  --key /path/to/keyfile -u ubuntu ctrl_client
```

Using the OpenSSH Client is also supported with the `--proxy-mode` flag. This also opens up access to `scp`. 

```
    ssh -i ~/.fablab/instances/smoketest/ssh_private_key.pem -o ProxyCommand='ziti fabric ssh router-east-1 --proxy-mode' ubuntu@router-east-1
    scp -i ~/.fablab/instances/smoketest/ssh_private_key.pem -o ProxyCommand='ziti fabric ssh ctrl1 --proxy-mode' ubuntu@ctrl1:./fablab/bin/ziti .
```

Note that you must have credentials to the host machine in addition to being a Ziti Administrator. 

### Alternate Access

You can use the proxy mode to get a pipe to whatever service you've got configured.

`ziti fabric ssh ctrl1 --proxy-mode`

It's up to you to connect whatever your management client is to that local pipe. Right now it only supports 
proxy via the stdin/stdout of the process. Supporting TCP or Unix Domain Socket proxies wouldn't be difficult
if there was use case for them.
