FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/pos-backend ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /bin/pos-backend /usr/local/bin/pos-backend
USER app
EXPOSE 8080
CMD ["pos-backend"]
