FROM golang:1.23

WORKDIR /app

# Copy everything
COPY . .

# Initialize modules if needed
RUN test -f go.mod || go mod init cron
RUN go mod tidy

# Build and run
RUN go build -o cron .
EXPOSE 8080
ENTRYPOINT ["./cron"]