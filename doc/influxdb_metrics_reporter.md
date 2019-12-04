# Using The Fabric InfluxDB Metrics Reporter

First you'll want to launch an InfluxDB docker container like this:

    $ docker run --name influxdb -d -p 8086:8086 -v /opt/influxdb:/var/lib/influxdb influxdb
    
The directory `/opt/influxdb` should be changed to wherever you would like InfluxDB to persist its data.

Next, open an `influx` cli connection:

    $ docker exec -it influxdb influx
    Connected to http://localhost:8086 version 1.7.7
    InfluxDB shell version: 1.7.7
    >
    
Then create the `ziti` database:

    >  create database ziti
    >
    
Exit the `influx` cli. InfluxDB is ready to go.

You'll want to add the following `metrics` section in your controller configuration:

    metrics:
      influxdb:
        enabled:            true
        url:                http://localhost:8086
        database:           ziti

Restart your controller and metrics should begin flowing into InfluxDB. You can verify like this:

    $ docker exec -it influxdb influx
    Connected to http://localhost:8086 version 1.7.7
    InfluxDB shell version: 1.7.7
    > use ziti
    Using database ziti
    > show series
    key
    ---
    egress.rx.bytesrate,source=001
    egress.rx.bytesrate,source=002
    egress.rx.bytesrate,source=003
    ----8<----
    link.OY8P.tx.msgsize,source=002
    link.OY8P.tx.msgsize,source=003
    link.yMey.latency,source=001
    link.yMey.latency,source=002
    link.yMey.rx.bytesrate,source=001
    link.yMey.rx.bytesrate,source=002
    link.yMey.rx.msgrate,source=001
    link.yMey.rx.msgrate,source=002
    link.yMey.rx.msgsize,source=001
    link.yMey.rx.msgsize,source=002
    link.yMey.tx.bytesrate,source=001
    link.yMey.tx.bytesrate,source=002
    link.yMey.tx.msgrate,source=001
    link.yMey.tx.msgrate,source=002
    link.yMey.tx.msgsize,source=001
    link.yMey.tx.msgsize,source=002
    > select * from "link.yMey.latency"
    name: link.yMey.latency
    time                count max    mean     min    p50      p75    p95    p99    p999   p9999  source stddev variance
    ----                ----- ---    ----     ---    ---      ---    ---    ---    ----   -----  ------ ------ --------
    1563471804584021939 1     615541 615541   615541 615541   615541 615541 615541 615541 615541 001    0      0
    1563471807396555092 1     549054 549054   549054 549054   549054 549054 549054 549054 549054 002    0      0
    1563471819583707653 2     630068 622804.5 615541 622804.5 630068 630068 630068 630068 630068 001    7263.5 52758432.25
    1563471822396356386 2     549054 506257   463460 506257   549054 549054 549054 549054 549054 002    42797  1831583209
    > select * from "link.yMey.latency"
    name: link.yMey.latency
    time                count max    mean     min    p50      p75    p95    p99    p999   p9999  source stddev            variance
    ----                ----- ---    ----     ---    ---      ---    ---    ---    ----   -----  ------ ------            --------
    1563471804584021939 1     615541 615541   615541 615541   615541 615541 615541 615541 615541 001    0                 0
    1563471807396555092 1     549054 549054   549054 549054   549054 549054 549054 549054 549054 002    0                 0
    1563471819583707653 2     630068 622804.5 615541 622804.5 630068 630068 630068 630068 630068 001    7263.5            52758432.25
    1563471822396356386 2     549054 506257   463460 506257   549054 549054 549054 549054 549054 002    42797             1831583209
    1563471834584197383 3     630068 555820   421851 615541   630068 630068 630068 630068 630068 001    94915.85098742289 9009018768.666666
    >

