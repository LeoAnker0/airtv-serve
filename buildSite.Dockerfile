FROM node:23-alpine

ARG GITHUB_PAT
ENV REPO_URL="https://${GITHUB_PAT}@github.com/LeoAnker0/airtv-v3.git"
ENV CLONE_DIR="/app"
ENV BUILD_DIR="/build"

RUN apk update && apk add --no-cache git

WORKDIR $CLONE_DIR

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]