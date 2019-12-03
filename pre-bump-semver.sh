#!/bin/sh
#
# If we are on the master branch, and the last commit was a merge commit
# then we have work to do.
#
# We need to auto-bump the semver.
#
# We will then commit the new semver file and publish it back to the repo,
# which will cause a new build to be done using the new semver.
# 

ENABLED_BRANCH=master

BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "$BRANCH" = "$ENABLED_BRANCH" ]
then

    REV=$(git rev-parse --short HEAD 2> /dev/null)
    MSHA=$(git rev-list -1 --merges ${REV}~1..${REV})
    if [ ! -z "$MSHA" ]
    then

        # Ensure we are topped off
        git pull origin $BRANCH > /dev/null 2>&1

        # If the VERSION file was NOT updated as part of this commit (i.e. not manually updated due to branch having a breaking-change)
        MODS=$(git diff --name-only --diff-filter=M @~..@)
        VMOD=$(echo ${MODS} | grep common/version/VERSION)
        if [ -z "$VMOD" ]
        then

            # Auto-bump the semver
            ./bump-semver.sh

            # Commit the new/bumped semver
            git commit -am "semver bump" --no-verify > /dev/null 2>&1

            # Lay down the tag for the new semver
            ./apply-semver-tag.sh

            # Publish the bumped semver up into the repo so everyone receives it
            git push origin HEAD > /dev/null 2>&1

            # Publish the tags as well
            git push origin --tags

            # Inform Makefile to stop executing and exit. The push that was performed above
            # will cause another build to start, and it will do the entire build using the
            # newly-bumped semver.
            exit 200

        # If the VERSION file WAS updated as part of this commit (i.e. it was manually updated due to branch having a breaking-change)
        else
            # Add/publish the tag for the new manually updated semver
            ./apply-semver-tag.sh
            git push origin --tags
        fi
    fi
fi

exit 100
