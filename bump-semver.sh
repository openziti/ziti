#!/bin/bash
#
# This script is invoked from the pre-bump-semver.sh script if the following is true:
#   1) the branch involved is master
#	2) the most recent commit was a merge-commit
#
# What we do here is increment the "patch" level in the VERSION file
#

MANIFEST="common/version/VERSION"
declare -x SCRIPTPATH="${0}"
FULLPATH=${SCRIPTPATH%/*}/$MANIFEST

if [ -f $FULLPATH ]
then
	LINE=$(grep -o ${FULLPATH} -e '^[0]\.[0-9]*\.[0-9]*');
	IFS='.' read -r -a array <<< "$LINE"
	VERSION=(${array[2]});
	INCREMENTED=$(($VERSION+1))
	sed "s/${array[0]}\.${array[1]}\.${array[2]}/${array[0]}\.${array[1]}\.${INCREMENTED}/" $FULLPATH > $FULLPATH.tmp && mv $FULLPATH.tmp $FULLPATH
fi;     
