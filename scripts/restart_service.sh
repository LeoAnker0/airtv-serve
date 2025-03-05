#!/bin/bash

echo "Restarting service: $1"
docker-compose restart "$1"
