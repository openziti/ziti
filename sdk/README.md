Ziti SDK for Go
---------------

# Enrollment
Prerequisite: Ziti Enrollment token in JWT format (e.g. `device.jwt`)

Run enrollment process to generate SDK configuration file -- `device.json`
```
$ ziti-enroller -jwt device.jwt -out device.json
```

----

Note: additional options (`-cert`, `-key`, `-engine`) 
are available to enroll with `ottCa` and `CA` methods
---

# Using SDK

SDK is using `ZITI_SDK_CONFIG` environment variable to load configuration file.

# Using `ziti-proxy`

`ziti-proxy` opens up local tcp port for proxy-ing to the target service. 
Services are identified by names in Ziti Controlller. It supports multiple services.
Usage:
```
$ ziti-proxy <service-name1>:<port1> [<service-name2>:<port2>]*
```
`ZITI_SDK_CONFIG` enviroment has to be set.

Example:
```
$ export ZITI_SDK_CONFIG=device.json
$ ziti-proxy netcat:33169
```


