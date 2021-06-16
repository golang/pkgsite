# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful bash functions and variables.

RED=; GREEN=; YELLOW=; BLUE=; BOLD=; RESET=;

case $TERM in
  '' | xterm) ;;
  # If xterm is not xterm-16color, xterm-88color, or xterm-256color, tput will
  # return the error:
  #   tput: No value for $TERM and no -T specified
  *)
      RED=`tput setaf 1`
      GREEN=`tput setaf 2`
      YELLOW=`tput setaf 3`
      NORMAL=`tput sgr0`
esac

EXIT_CODE=0

info() { echo -e "${GREEN}$@${NORMAL}" 1>&2; }
warn() { echo -e "${YELLOW}$@${NORMAL}" 1>&2; }
err() { echo -e "${RED}$@${NORMAL}" 1>&2; EXIT_CODE=1; }

die() {
  err $@
  exit 1
}

# runcmd prints an info log describing the command that is about to be run, and
# then runs it. It sets EXIT_CODE to non-zero if the command fails, but does not exit
# the script.
runcmd() {
  msg="$@"
  # Truncate command logging for narrow terminals.
  # Account for the 2 characters of '$ '.
  case ${TERM} in
    '' | xterm) ;;
    *)
        maxwidth=$(( $(tput cols) - 2 ))
        if [[ ${#msg} -gt $maxwidth ]]; then
          msg="${msg::$(( maxwidth - 3 ))}..."
        fi
  esac

  info "\$ $msg"
  $@ || err "command failed"
}
