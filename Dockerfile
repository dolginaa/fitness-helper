# Build
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H -u 65532 botuser \
    && mkdir /data && chown botuser /data
COPY --from=build /out/bot /usr/local/bin/bot
USER botuser
# The MTProto session lives here; mount a volume to keep the bot logged in.
VOLUME /data
ENV TELEGRAM_SESSION_FILE=/data/telegram-session.json
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bot"]
