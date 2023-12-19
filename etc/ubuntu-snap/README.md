# quick notes on building a snap from scratch...

* checkout branch
* install snapd if needed - https://docs.snapcraft.io/installing-snapd/6735
* install snapcraft --classic to build snaps

    snap install snapcraft --classic
    hash -r

* navigate to the root of the git checkout - not the $GOPATH
* cd ~/git_or_wherever/nf/src/bitbucket.org/netfoundry/ziti
* issue `snapcraft` you should see results (in green maybe) like (you may need the `--destructive-mode` flag):

    Cleaning later steps and re-staging ziti ('build' step changed)
    Priming ziti 
    Snapping 'netfoundry' -
    Snapped netfoundry_0.0.1_amd64.snap

* install the snap in spoooky 'dangerous' and 'devmode':

    sudo snap install --devmode --dangerous netfoundry*.snap

* ensure the snap is listed:

    snap list | grep netfoundry

    should return something like:
    netfoundry            0.0.1                      x1    -            -           devmode

* try the enroller out:

    netfoundry.ziti-enroller

* try the tunneler out:

   netfoundry.ziti-tunneler
