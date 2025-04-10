
# HA Quickstart

You can explore Ziti HA by running three local processes on unique TCP ports. This is an interactive quickstart based on the [the HA test script](/quickstart/test/ha-test.sh).

1. Create an empty working directory. All commands are run here.

    ```bash
    cd $(mktemp -d)
    ```

1. Run the first member in the background to create the cluster.

    ```bash
    nohup ziti edge quickstart \
        --instance-id="ctrl1" \
        --ctrl-port="1281" \
        --router-port="3021" \
        --home="${PWD}" \
        --ctrl-address="127.0.0.1" \
        --router-address="127.0.0.1" \
        --trust-domain="ha-quickstart" \
    &> ctrl1.log &
    ```

    Confirm the first job is running and check the log for startup errors.

    ```bash
    tail ctrl1.log; echo; jobs
    ```

    Expected output:

    ```text
    .
    .
    .
    ... logs ...
    .
    .
    .

    [1]  + running    nohup ziti edge quickstart --instance-id="ctrl1" --ctrl-port="1281"      &
    ```

1. Run the second member and join the cluster.

    ```bash
    nohup ziti edge quickstart join \
        --instance-id="ctrl2" \
        --ctrl-port="1282" \
        --router-port="3022" \
        --home="${PWD}" \
        --ctrl-address="127.0.0.1" \
        --router-address="127.0.0.1" \
        --trust-domain="ha-quickstart" \
        --cluster-member="tls:127.0.0.1:1281" \
    &> ctrl2.log &
    ```

    Confirm the second job is running and check the log for startup errors.

    ```bash
    tail ctrl2.log; echo; jobs
    ```

    Expected output:

    ```text
    .
    .
    .
    ... logs ...
    .
    .
    .

    [1]  - running    nohup ziti edge quickstart --instance-id="ctrl1" --ctrl-port="1281"      &
    [2]  + running    nohup ziti edge quickstart join --instance-id="ctrl2" --ctrl-port="1282"     
    ```

1. Run the third member and join the cluster.

    ```bash
    nohup ziti edge quickstart join \
        --instance-id="ctrl3" \
        --ctrl-port="1283" \
        --router-port="3023" \
        --home="${PWD}" \
        --ctrl-address="127.0.0.1" \
        --router-address="127.0.0.1" \
        --trust-domain="ha-quickstart" \
        --cluster-member="tls:127.0.0.1:1281" \
    &> ctrl3.log &
    ```

    Confirm the third job is running and check the log for startup errors.

    ```bash
    tail ctrl3.log; echo; jobs
    ```

    Expected output:

    ```text
    .
    .
    .
    ... logs ...
    .
    .
    .

    [1]    running    nohup ziti edge quickstart --instance-id="ctrl1" --ctrl-port="1281"      &
    [2]  - running    nohup ziti edge quickstart join --instance-id="ctrl2" --ctrl-port="1282"     
    [3]  + running    nohup ziti edge quickstart join --instance-id="ctrl3" --ctrl-port="1283"     
    ```

1. Optionally, follow interleaved logs in another window.

    ```bash
    tail -F -n +1 *.log
    ```

1. List agent applications.

    ```bash
    ziti agent list                          
    ```

    Expected output:

    ```text
    ╭────────┬────────────┬────────┬─────────────────────────────┬────────────┬─────────────┬───────────╮
    │    PID │ EXECUTABLE │ APP ID │ UNIX SOCKET                 │ APP TYPE   │ APP VERSION │ APP ALIAS │
    ├────────┼────────────┼────────┼─────────────────────────────┼────────────┼─────────────┼───────────┤
    │ 276912 │ ziti       │ ctrl1  │ /tmp/gops-agent.276912.sock │ controller │ v0.0.0      │           │
    │ 277714 │ ziti       │ ctrl2  │ /tmp/gops-agent.277714.sock │ controller │ v0.0.0      │           │
    │ 281490 │ ziti       │ ctrl3  │ /tmp/gops-agent.281490.sock │ controller │ v0.0.0      │           │
    ╰────────┴────────────┴────────┴─────────────────────────────┴────────────┴─────────────┴───────────╯
    ```

1. Identify the cluster leader.

    ```bash
    ziti agent cluster list --app-id ctrl1
    ```

    Expected output:

    ```text
    ╭───────┬────────────────────┬───────┬────────┬─────────┬───────────╮
    │ ID    │ ADDRESS            │ VOTER │ LEADER │ VERSION │ CONNECTED │
    ├───────┼────────────────────┼───────┼────────┼─────────┼───────────┤
    │ ctrl1 │ tls:127.0.0.1:1281 │ true  │ true   │ v0.0.0  │ true      │
    │ ctrl2 │ tls:127.0.0.1:1282 │ true  │ false  │ v0.0.0  │ true      │
    │ ctrl3 │ tls:127.0.0.1:1283 │ true  │ false  │ v0.0.0  │ true      │
    ╰───────┴────────────────────┴───────┴────────┴─────────┴───────────╯
    ```

1. Simulate a member availability incident.

    Identify the job number for the cluster leader. It may not be `%1`.

    ```bash
    jobs
    ```

    Job 1 belongs to ctrl1, the current leader.

    ```text
    [1]    running    nohup ziti edge quickstart --instance-id="ctrl1" --ctrl-port="1281"       
    [2]  - running    nohup ziti edge quickstart join --instance-id="ctrl3" --ctrl-port="1283"     
    [3]  + running    nohup ziti edge quickstart join --instance-id="ctrl2" --ctrl-port="1282"     
    ```

    ```bash
    kill %1
    ```

1. Inspect the cluster via another member ID.

    ```bash
    ziti agent cluster list --app-id ctrl2
    ```

    Expected output:

    ```text
    ╭───────┬────────────────────┬───────┬────────┬─────────────────┬───────────╮
    │ ID    │ ADDRESS            │ VOTER │ LEADER │ VERSION         │ CONNECTED │
    ├───────┼────────────────────┼───────┼────────┼─────────────────┼───────────┤
    │ ctrl1 │ tls:127.0.0.1:1281 │ true  │ false  │ <not connected> │ false     │
    │ ctrl2 │ tls:127.0.0.1:1282 │ true  │ true   │ v0.0.0          │ true      │
    │ ctrl3 │ tls:127.0.0.1:1283 │ true  │ false  │ v0.0.0          │ true      │
    ╰───────┴────────────────────┴───────┴────────┴─────────────────┴───────────╯
    ```

1. Restart any disconnected member.

    Any member can be restarted with this `ha` subcommand.

    ```bash
    nohup ziti edge quickstart \
        --instance-id="ctrl1" \
        --home="${PWD}" \
    &>> ctrl1.log &
    ```

1. Inspect the cluster.

    Once restarted, ctrl1 does not necessarily resume being the leader.

    ```bash
    ziti agent cluster list --app-id ctrl2
    ```

    Expected output:

    ```text
    ╭───────┬────────────────────┬───────┬────────┬─────────────────┬───────────╮
    │ ID    │ ADDRESS            │ VOTER │ LEADER │ VERSION         │ CONNECTED │
    ├───────┼────────────────────┼───────┼────────┼─────────────────┼───────────┤
    │ ctrl1 │ tls:127.0.0.1:1281 │ true  │ false  │ v0.0.0          │ true      │
    │ ctrl2 │ tls:127.0.0.1:1282 │ true  │ false  │ v0.0.0          │ true      │
    │ ctrl3 │ tls:127.0.0.1:1283 │ true  │ true   │ v0.0.0          │ true      │
    ╰───────┴────────────────────┴───────┴────────┴─────────────────┴───────────╯
    ```

1. Stop all background jobs.

    BASH

    ```bash
    kill $(jobs -p)
    ```

    ZSH

    ```bash
    kill ${${(v)jobstates##*:*:}%=*}
    ```

    Expected output:

    ```text
    [1]  + done       nohup ziti edge quickstart --instance-id="ctrl1" --ctrl-port="1281"
    [2]    done       nohup ziti edge quickstart join --instance-id="ctrl2" --ctrl-port="1282"
    [3]  + done       nohup ziti edge quickstart join --instance-id="ctrl3" --ctrl-port="1283"
    ```
