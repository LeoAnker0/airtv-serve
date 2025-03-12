FROM golang:1.23

WORKDIR /app

# Copy everything
COPY . .

# Initialize modules if needed
RUN go mod init api
RUN go mod tidy

# Build and run
RUN go build -o api .
EXPOSE 8080
ENTRYPOINT ["./api"]