# Build
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H -u 65532 botuser
COPY --from=build /out/bot /usr/local/bin/bot
USER botuser
ENTRYPOINT ["/usr/local/bin/bot"]
