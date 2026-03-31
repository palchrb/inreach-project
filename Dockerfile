FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o inreach ./cmd/inreach/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/inreach /usr/local/bin/inreach

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["inreach"]
CMD ["run"]
