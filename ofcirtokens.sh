#!/bin/bash

set -f

function usage(){
    cat << EOF
$0 list
    List all tokens and their pools
$0 new [-p pools] [-t token]
    Create a new token
    -p POOLS
        new token will allow access to POOLS (comma seperated)
    -t TOKEN
        new token will copy pools from TOKEN
$0 update TOKEN POOLS
    Update TOKEN with POOLS
$0 delete TOKEN
    Delete TOKEN
EOF
    exit 1
}

function list(){
    oc get secret/ofcir-tokens -o json | jq -r '.data | keys[] as $k | "\($k) \(.[$k])"' |
    while read TOKEN POOL ; do
        printf "%36s %s\n" $TOKEN $(echo $POOL | base64 -d)
    done |
    sort
}

function new(){
    while getopts "p:t:" O ; do
        case $O in
            p)  POOLS=$OPTARG
                POOLS=$(echo -n $POOLS | base64 -w 0)
                ;;
            t)  POOLS=$(oc get secret/ofcir-tokens -o json | jq -r ".data[\"$OPTARG\"]")
                ;;
            *)  usage
                ;;
        esac
    done
    TOKEN=$(uuidgen)
    oc patch secret/ofcir-tokens --patch "{\"data\":{\"$TOKEN\":\"$POOLS\"}}"
}

function update(){
    POOLS=$(echo -n $2 | base64 -w 0)
    oc patch secret/ofcir-tokens --patch "{\"data\":{\"$1\":\"$POOLS\"}}"
}

function delete(){
    oc patch secret/ofcir-tokens --type=json -p="[{\"op\": \"remove\", \"path\":\"/data/$1\"}]"
}

CMD=$1
[ -z "$CMD" ] && usage
shift
$CMD $@
