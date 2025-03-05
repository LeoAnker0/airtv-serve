# Use the base webhookd image
FROM ncarlier/webhookd:edge-distrib

# Copy scripts into the container and set executable permissions
COPY --chmod=755 ./scripts /scripts

# Use the default webhookd entrypoint
ENTRYPOINT ["webhookd"]