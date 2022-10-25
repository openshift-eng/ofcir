#!/bin/bash

# This script is for testing / debugging purpouses only

if [ $# -lt 1 ]; then
    echo "Please specify at least one command:"
    echo " - acquire <type>"
    echo " - status <cir-id>"
    echo " - release <cir-id>"
    echo " - change-state <cir-id> <state>"
    echo " - resize-pool <pool-id> <size>"

    exit 1
fi

ofcirUrl=$(minikube service ofcir-service --namespace=ofcir-system --url)

case $1 in
    acquire)
        if [ $# -eq 2 ]; then
            type="?type=$2"
        fi
        res=$(curl -s -X POST ${ofcirUrl}/v1/ofcir${type})
        echo $res
        ;;

    status)
        if [ $# -ne 2 ]; then
            echo "Command requires <cir-id>"
            exit 1
        fi
        res=$(curl -s ${ofcirUrl}/v1/ofcir/$2)
        echo $res
        ;;

    release)
        if [ $# -ne 2 ]; then
            echo "Command requires <cir-id>"
            exit 1
        fi
        res=$(curl -s -X DELETE ${ofcirUrl}/v1/ofcir/$2)
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

    resize-pool)
        if [ $# -ne 3 ]; then
            echo "Command requires <pool-id> <size>"
            exit 1
        fi
        res=$(kubectl patch cipool $2 --type merge --patch '{"spec": {"size": '$3'}}')
        echo $res
        ;;

esac