#!/bin/sh
# Ensure we are at the project root
cd /app

# Pull the latest code from the main branch
git pull origin main

# (Optional) Install dependencies and build your Vue 3 app if needed
# For example, if your web image is built from your source:
npm install
npm run build

# Rebuild the AIRTV_host image (assumes your Dockerfile picks up the build output)
docker-compose build AIRTV_host

# Restart the AIRTV_host container with the updated image
docker-compose up -d AIRTV_host