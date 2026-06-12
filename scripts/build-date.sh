#!/bin/bash
# Copyright IBM Corp. 2026
# SPDX-License-Identifier: MPL-2.0

readonly SCRIPT_NAME="$(basename ${BASH_SOURCE[0]})"
readonly SCRIPT_DIR="$(dirname ${BASH_SOURCE[0]})"
readonly SOURCE_DIR="$(dirname "$(dirname "${SCRIPT_DIR}")")"

function usage {
cat <<-EOF
Usage: ${SCRIPT_NAME}

Description:

   This script uses the date of the last checkin on the branch as the build date. This
   is to make the date consistent across the various platforms we build on, even if they
   start at different times. In practice this is the commit where the version string is set.
EOF
}

function git_date {
   # Arguments:
   #   $1 - Path to the git repo (optional - assumes pwd is git repo otherwise)
   #
   # Returns:
   #   0 - success
   #   * - failure
   #
   # Notes:
   #   Echos the date of the last git commit in

   local gdir="$(pwd)"
   if test -d "$1"
   then
      gdir="$1"
   fi

   pushd "${gdir}" > /dev/null

   local ret=0

   # it's tricky to do an RFC3339 format in a cross platform way, so we hardcode UTC
   local date_format="%Y-%m-%dT%H:%M:%SZ"
   # we're using this for build date because it's stable across platform builds
   local date="$(TZ=UTC0 git show -s --format=%cd --date=format-local:"$date_format" HEAD)" || ret=1

   ##local head="$(git status -b --porcelain=v2 | awk '{if ($1 == "#" && $2 =="branch.head") { print $3 }}')" || ret=1

   popd > /dev/null

   test ${ret} -eq 0 && echo "$date"
   return ${ret}
}

echoerr() { printf "%s\n" "$*" >&2; }

echoerr "[DEBUG] Getting git date for repo at ${SOURCE_DIR}"
git_date "$SOURCE_DIR"