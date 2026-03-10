FROM golang:1.25-alpine3.23 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /backend .

FROM alpine:3.23.3
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /backend /backend
ENTRYPOINT ["/backend"]
