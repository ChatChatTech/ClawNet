FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /letchat ./cmd/letchat/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /letchat /usr/local/bin/letchat

EXPOSE 4001/tcp 4001/udp 3847/tcp

ENTRYPOINT ["letchat"]
CMD ["start"]
