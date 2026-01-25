FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY ./enc/ ./

RUN go mod download

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -tags netgo \
    -ldflags "-s -w -linkmode external -extldflags '-static'" \
    -o enc .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/enc .

COPY .env .

ENTRYPOINT ["./enc"]
