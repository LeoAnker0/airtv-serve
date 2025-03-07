FROM ncarlier/webhookd:edge-distrib

USER root
RUN apk add --no-cache docker-cli curl
RUN mkdir -p /usr/local/lib/docker/cli-plugins && \
    curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 -o /usr/local/lib/docker/cli-plugins/docker-compose && \
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
USER 1000

# Set working directory to mounted compose project
WORKDIR /app

COPY --chmod=755 ./scripts /scripts
ENTRYPOINT ["webhookd"]