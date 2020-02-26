#!/bin/bash

if [ "$#" != 1 ]
then
  echo "Usage: bash $0 BRANCH"
  exit 1
fi

BRANCH=$1

if [ "$BRANCH" == "0.9.0"  ]
then
  make push LOCALMODE=true
else
  make push
fi
