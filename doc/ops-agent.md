# Runtime Operations Agent

The ziti controller and ziti router can both be introspected at runtime using the ziti command line tool.

## Available Operations

1. Get the stack traces of all go-routines the running process
   1. `ziti ps stack`
1. Force garbage collection
   1. `ziti ps gc`
1. View memory statistics
   1. `ziti ps memstats`
   1. Example:

      ```
      $ ziti ps memstats
      alloc: 22.89MB (24005552 bytes)
      total-alloc: 1.49GB (1602895000 bytes)
      sys: 75.02MB (78660608 bytes)
      lookups: 0
      mallocs: 23141725
      frees: 22895477
      heap-alloc: 22.89MB (24005552 bytes)
      heap-sys: 63.00MB (66060288 bytes)
      heap-idle: 34.71MB (36397056 bytes)
      heap-in-use: 28.29MB (29663232 bytes)
      heap-released: 31.31MB (32833536 bytes)
      heap-objects: 246248
      stack-in-use: 1.00MB (1048576 bytes)
      stack-sys: 1.00MB (1048576 bytes)
      stack-mspan-inuse: 500.44KB (512448 bytes)
      stack-mspan-sys: 576.00KB (589824 bytes)
      stack-mcache-inuse: 20.34KB (20832 bytes)
      stack-mcache-sys: 32.00KB (32768 bytes)
      other-sys: 2.97MB (3109868 bytes)
      gc-sys: 5.70MB (5974840 bytes)
      next-gc: when heap-alloc >= 24.58MB (25776064 bytes)
      last-gc: 2020-11-30 16:35:46.766977147 -0500 EST
      gc-pause-total: 10.939682ms
      gc-pause: 228800
      num-gc: 140
      enable-gc: true
      debug-gc: false
      ```

1. Get the go version used to build the executable
    1. `ziti ps goversion`
1. Gets snapshot of the heap 
    1. `ziti ps pprof-heap`
1. Run cpu profiling for 30 seconds and returns the results
    1. `ziti ps pprof-cpu`
1. Get Go runtime statistics such as number of goroutines, GOMAXPROCS, and NumCPU
    1. `ziti ps stats`
    1. Example:

    ```bash
    $ ziti ps stats
    goroutines: 50
    OS threads: 19
    GOMAXPROCS: 12
    num CPU: 12
    ```

1. Run tracing for 5 seconds and return the result
    1. `ziti ps trace`
1. Set the GC target percentage
    1. `ziti ps setgc <percentage>`

## Disabling the Agent

The agent is enabled by default. It can be disabled using `--cliagent false`.

## Configuring Agent

By default, the agent will listen on a Unix socket at `/tmp/gops-agent.<pid>.sock`. You can change this to a custom unix socket or use a network socket instead.

Examples:

1. `ziti-controller --cli-agent-addr unix:/tmp/my-special-agent-file.sock`
2. `ziti-controller --cli-agent-addr tcp:127.0.0.1:10001`

The agent uses unix sockets to limit security risk. Only the user on the machine who started the application, or the root user should be able to access the socket.

## Selecting Agent

When running `ziti ps`, the target agent may be specified in a few different ways.

1. If no target is provided it will scan in `/tmp` for valid sockets. If only one is found, that one will be used. If multiple are found and error will be reported along with the different running processes.
1. The target can be listed by name. So if you have a ziti-controller and a ziti-router running, you can do `ziti ps goversion ziti-router` it will find the ziti-router and talk with that. If you have multiple ziti-routers running, this will fail.
1. You can specify the application PID. Example: `ziti ps goversion 1224770`
1. If you are using network sockets you can specify the address. Example: `ziti ps goversion tcp:my-host:10001`
