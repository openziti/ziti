# Runtime Operations Agent

The ziti controller and ziti router can both be introspected at runtime using the ziti command line tool.

## Basic Use
The agent uses Unix domain sockets to communicate. These are represented by files in the file system. 
The practical effect of this is that `ziti agent` commands need to be run as the same user as the
ziti controller or router. Running as the root user will also work.

The domain socket files are generally stored in the temp directory and are named `gops-agent.<pid>.sock=`.

Example:

```
$ l /tmp/gops-agent.*
/tmp/gops-agent.29050.sock=  /tmp/gops-agent.3759.sock=
```

Agent commands can find the process they're connecting to in a few different ways. If there's only one
agent process running, you don't have specify anything.

Example:

```
$ ziti agent goversion
go1.18
```

If you have multiple processes running, but they have different names, you can pick one using the
process name:

Example:

```
$ ziti agent goversion
error: too many gops-agent process found, including [ziti-controller (pid 29050), ziti-router (pid 29425)]
$ ziti agent goversion ziti-controller
go1.18
```

The application PID can also be used to specify a target process.

Example:

```
$ ziti agent goversion 29425
go1.18
```

Finally, if the agent has been configured to use network sockets instead of unix domain sockets, the network 
address can be specified.

Example:

```
$ziti agent goversion tcp:my-host:10001
go1.18
```

## Configuring Agent 

By default, the agent will listen on a Unix socket at `/tmp/gops-agent.<pid>.sock`. You can change this to a custom unix socket or use a network socket instead.
Use unix sockets to limit security risk. Only the user on the machine who started the application, or the root user should be able to access the socket.

Examples:

1. `ziti-controller --cli-agent-addr unix:/tmp/my-special-agent-file.sock`
2. `ziti-controller --cli-agent-addr tcp:127.0.0.1:10001`

### Disabling the Agent

The agent is enabled by default. It can be disabled using `--cliagent false`.

## Available Operations

1. Get the stack traces of all go-routines the running process
   1. `ziti agent stack`
   1. Stacks are usually quite large and are piped to a file
   1. Ex: `ziti agent stack > stack.dump`
1. Force garbage collection
   1. `ziti agent gc`
1. View memory statistics
   1. `ziti agent memstats`
   1. Example:

      ```
      $ ziti agent memstats
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
    1. `ziti agent goversion`
1. Gets snapshot of the heap 
    1. `ziti agent pprof-heap`
    1. pprof data is binary and so should be piped to a file
    1. pprof data can be viewed using `go tool pprof`
    1. Ex: 
        ```
        $ ziti agent pprof-heap > heap.pprof
        $ go tool pprof -web heap.pprof
        ```
1. Run cpu profiling for 30 seconds and returns the results
    1. `ziti agent pprof-cpu`
    1. pprof data is binary and so should be piped to a file
    1. pprof can be viewed using `go tool pprof`
    1. Ex: 
        ```
        $ ziti agent pprof-heap > heap.pprof
        $ go tool pprof -web heap.pprof
        ```
1. Get Go runtime statistics such as number of goroutines, GOMAXPROCS, and NumCPU
    1. `ziti agent stats`
    1. Example:

    ```bash
    $ ziti agent stats
    goroutines: 50
    OS threads: 19
    GOMAXPROCS: 12
    num CPU: 12
    ```
1. Run tracing for 5 seconds and return the result
    1. `ziti agent trace`
    1. trace data is binary and so should be piped to a file
    1. trace data can be viewed using `go tool trace`
    1. Ex: 
        ```
        $ ziti agent trace > debug.trace
        $ go tool trace debug.trace
        ```
1. Set the GC target percentage
    1. `ziti agent setgc <percentage>`
