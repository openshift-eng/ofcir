#!/bin/bash

# This script is for testing / debugging purpouses only

if [ $# -lt 1 ]; then
    echo "Please specify at least one command:"
    echo " - acquire"
    echo " - status <cir-id>"
    echo " - release <cir-id>"
    echo " - change-state <cir-id> <state>"
    echo " - set-pool-size <pool-id> <size>"

    exit 1
fi

case $1 in
    acquire)
        res=$(curl -s -X POST http://localhost:8087/v1/ofcir)
        echo $res
        ;;

    status)
        if [ $# -ne 2 ]; then
            echo "Command requires <cir-id>"
            exit 1
        fi
        res=$(curl -s http://localhost:8087/v1/ofcir/$2)
        echo $res
        ;;

    release)
        if [ $# -ne 2 ]; then
            echo "Command requires <cir-id>"
            exit 1
        fi
        res=$(curl -s -X DELETE http://localhost:8087/v1/ofcir/$2)
        echo $res
        ;;

    change-state)
        if [ $# -ne 3 ]; then
            echo "Command requires <cir-id> <state>"
            exit 1
        fi
        res=$(kubectl patch cir $2 --type merge --patch '{"spec": {"state": "'$3'"}}')
        echo $res
        ;;

    set-pool-size)
        if [ $# -ne 3 ]; then
            echo "Command requires <pool-id> <size>"
            exit 1
        fi
        res=$(kubectl patch cipool $2 --type merge --patch '{"spec": {"size": '$3'}}')
        echo $res
        ;;

esac