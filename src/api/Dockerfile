FROM golang:1.23

WORKDIR /app

# Copy only the necessary files for module download
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application
COPY . .

# Build and run
RUN go build -o api .
EXPOSE 8080
ENTRYPOINT ["./api"]