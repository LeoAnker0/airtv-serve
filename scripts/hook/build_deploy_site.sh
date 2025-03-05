#!/bin/bash

SERVICE_NAME="airtvbuilder"

echo "Triggering build for $SERVICE_NAME..."
docker-compose up --build --force-recreate "$SERVICE_NAME"

echo "Build completed."