# Zitifying SSH

As we learned in the [opening post](../README.md) - "zitifying" an application means to embed a Ziti SDK into an
application and leverage the power of
the [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network) to provide secure, truly
zero-trust access to your application wherever in the world that application goes. In this post we are going to see how
we have zitified `ssh` and why. Future posts will expand on this even further by showing how it is that NetFoundry
uses `zssh` to support our customers.

In this post you will see why a zitified `ssh` client makes

<hr>

## Why SSH?

As I sit here typing these words I can tell you're skeptical. I can tell you're wondering why in the world we would even
attempt to mess with `ssh` at all. After all, `ssh` has been a foundation of the administration of not only home
networks but also corporate networks and the Internet itself. Surely if millions (billions?) of computers can interact
every day safely and securely using `ssh` there is "no need" for us to be spending time zitifying `ssh`
right? (Spoiler alert: wrong)

I'm sure you've guessed that this is not the case whatsoever. After all, attackers don't leave `ssh` alone just because
it's not worth it to try! Put a machine on the open internet, expose `ssh` on port 22 and watch for yourself all the
attempts to access `ssh` using known default/weak/bad passwords flood in. Attacks don't only come from the Internet
either! Attacks from a single compromised machine on your network very well could behave in the same way as an outside
attacker. This is particularly true for ransomware style attacks as the compromised machine attempts to expand/multiply.
The problems don't just stop here either. DoS attacks, other zero-day type bugs and more are all waiting for any service
sitting on the open internet.

A zitified `ssh` client is superior since the port used by `ssh` can be elimitated from the Internet-based firewall or
perhaps could even be configured to listen only on localhost, preventing any connections whatsoever from any network
client. In this configuration the `ssh` process is effectively "dark"
meaning the service is not only invisible to the Internet but also the local area network as well! The only way to
`ssh` to a machine using a [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network) will
be to have an identity authorized for
that [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network). Cool right? Let's see how
we did that, and how you can do the same thing using
a [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network).

<hr>

## How It's Done

There are a few steps necessary before being able to use `zssh`:

* Establish a [Ziti Network in AWS](https://github.com/openziti/ziti/blob/release-next/quickstart/aws.md) (using AWS in
  this example)
* Create and enroll two Ziti Endpoints (one for our `ssh` server, one for the client)
    * the `sshd` server will run `ziti-tunnel` for this demonstration. Conviniently it will run on the same machine that
      I used to setup the [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network).
    * the client in this case will run `zssh` from my local machine and I'll `zssh` to the other endpoint
* Create the [Ziti Service](https://openziti.github.io/ziti/services/overview.html) we'll use and authorize the two
  endpoints to use this service
* Use the `zssh` binary from the client side and the `ziti-tunnel` binary from the serving side to connect
* Harden `sshd` further by removing port 22 from any Internet-based firewall configuration (for example from within the
  security-groups wizard in AWS) or by forcing `sshd` to only listen on `localhost/127.0.0.1`

After preforming these steps you'll have an `sshd` server which is dark to the Internet or your LAN (depending on how
you decided to harden `sshd`). Accessing the server via `ssh` must now occur using the Ziti Network. Since the service
is no longer accessible directly through a network it is no longer susceptible to the types of attacks mentioned
previously!

<hr>

## Zssh in Action

Once the prerequisites are satisfied, we can see `zssh` in action. Simply download the binary for your platform:

* [linux](https://github.com/openziti-incubator/zssh/releases/download/latest-tag/zssh-linux-amd64)
* [windows](https://github.com/openziti-incubator/zssh/releases/download/latest-tag/zssh-windows-amd64.exe)
* [MacOs](https://github.com/openziti-incubator/zssh/releases/download/latest-tag/zssh-macos-amd64)

Once you have the executable download, make sure it is named `zssh` and for simplicity's sake we'll assume it's on the
path. The usage for `zssh` is very similar to the usage of `ssh` so anyone familiar with `zssh` should be able to pick
up `zssh` easily, Should you forget the usage, executing the binary with no arguments returns the expected usage as
well. The general format when uzing `zssh` will be: `zssh <remoteUsername>@<targetIdentity>`

Below you can see me `zssh` from my local machine to the AWS machine secured by `ziti-tunnel`:

    ./zssh ubuntu@ziti-tunnel-aws
    INFO[0000] connection to edge router using token 95c45123-9415-49d6-930a-275ada9ae06f
    connected.
    ubuntu@ip-172-31-27-154:~$

It really was that simple! Now let's break down the current flags for `zssh` and exactly how this worked.

<hr>

## Zssh Flags

We know that `zssh` requires access to
a [Ziti Network](https://openziti.github.io/ziti/overview.html#overview-of-a-ziti-network) but it is not clear from the
example above is where `zzsh` found the credentials required to access that network. `zssh` supports three basic flags:

    -i, --SshKeyPath string   Path to ssh key. default: $HOME/.ssh/id_rsa
    -c, --ZConfig string      Path to ziti config file. default: $HOME/.ziti/zssh.json
    -d, --debug               pass to enable additional debug information

What you see above is exactly the output `zssh` provides should you pass the `-h/--help` flag or execute `zssh`
without any parameters. The `-i/--SshKeyPath` flag is congruent to the `-i` flag for `ssh`. You would use it to supply
your key to the `ssh` client. Under the hood of `zssh` is a full-fledged `ssh` client which works just like `ssh` does.
If your `~/.ssh/id_rsa` file is in the `authorized_keys` of the remote machine - then you won't need to specify the
`-i/` flag (as I didn't in my example). It is required to use a key when using `zssh`.

The `-c/--ZConfig` flag controls access to the network. It is also required when using `zssh`. By default `zssh`
will look at your home directory in a folder named `.ziti` for a file named `zssh.json`. In bash this is would be the
equivalent of `$HOME`. In powershell this is the equivalent the environment variable named `USERPROFILE`. If that file
exists you will not need to supply this flag. If you need to `zssh` across various networks you can always simply
specify the flag to change which network you are using.

The `-d/--debug` flag is simply used to output additional information to assist you with debugging. An example is shown:

    $ ./zssh ubuntu@ziti-tunnel-aws -d
    INFO[0000]     sshKeyPath set to: /home/myUser/.ssh/id_rsa
    INFO[0000]        ZConfig set to: /home/myUser/.ziti/zssh.json
    INFO[0000]       username set to: ubuntu
    INFO[0000] targetIdentity set to: ziti-tunnel-aws
    INFO[0000] connection to edge router using token 95c45123-a234-412e-8997-96139fbd1938
    connected.
    ubuntu@ip-172-31-27-154:~$

Shown above is also one additional piece of information, the remote username. In the exmaple above since I have
`zssh`ed to an ubuntu image in AWS, the default username is `ubuntu` so in order to `zssh` there I need to tell the
remote `sshd` server that I wish to attach as the `ubuntu` user. If your username is the same for your local environment
as the remote machine you do not need to specify the username. For example my local username is `cd`
(my initials). When I `zssh` to my dev machine I can simply use `zssh ClintLinux`:

    $ ./zssh ClintLinux
    INFO[0000] connection to edge router using token 909dfb4f-fa83-4f73-af8e-ed251bcd30be
    connected.
    cd@clint-linux-vm ~

Hopefully this post has been helpful and insightful. Zitifying an application is _POWERFUL_!!!!