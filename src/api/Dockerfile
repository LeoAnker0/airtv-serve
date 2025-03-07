FROM golang:1.23.0

WORKDIR /app

# Copy everything
COPY . .

# Initialize modules if needed
RUN test -f go.mod || go mod init github.com/yourusername/airtv/api
RUN go mod tidy

# Build and run
RUN go build -o api .
EXPOSE 8080
ENTRYPOINT ["./api"]