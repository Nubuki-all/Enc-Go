FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY . ./

RUN go mod download

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -tags netgo \
    -ldflags "-s -w -linkmode external -extldflags '-static'" \
    -o enc .

FROM fedora:latest

WORKDIR /root/

COPY --from=builder /app/enc .

COPY .env .

ENTRYPOINT ["./enc"]
