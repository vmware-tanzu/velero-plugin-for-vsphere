#!/bin/bash

if [ "$#" != 1 ]
then
  echo "Usage: bash $0 BRANCH"
  exit 1
fi

BRANCH=$1
COMMIT=$(git log --oneline | head -1 | awk '{print $1}')
TIMESTAMP=$(date +%s)

if [ $BRANCH = "v0.9.0"  ]
then
  make push LOCALMODE=true VERSION=$BRANCH-$COMMIT-$TIMESTAMP
else
  make push VERSION=$BRANCH-$COMMIT-$TIMESTAMP
fi
