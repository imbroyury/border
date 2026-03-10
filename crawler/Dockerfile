FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY migrations/ /migrations/
COPY crawler/go.mod crawler/go.sum ./
RUN go mod download
COPY crawler/ .
RUN CGO_ENABLED=0 go build -o /crawler .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /crawler /crawler
ENTRYPOINT ["/crawler"]
