#!/bin/bash
#
# This script is invoked from the pre-bump-semver.sh script after a semver bump
# but before that bump is pushed back to the Git server.
#

MANIFEST="common/version/VERSION"
declare -x SCRIPTPATH="${0}"
FULLPATH=${SCRIPTPATH%/*}/$MANIFEST

if [ -f $FULLPATH ]
then
	LINE=$(grep -o ${FULLPATH} -e '^[0]\.[0-9]*\.[0-9]*');
	IFS='.' read -r -a array <<< "$LINE"
	$(git tag -a ${array[0]}.${array[1]}.${array[2]} -m "${array[0]}.${array[1]}.${array[2]}")
fi;     
