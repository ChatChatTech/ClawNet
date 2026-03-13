FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /clawnet ./cmd/clawnet/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /clawnet /usr/local/bin/clawnet

EXPOSE 4001/tcp 4001/udp 3847/tcp

ENTRYPOINT ["clawnet"]
CMD ["start"]
