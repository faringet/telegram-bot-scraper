FROM golang:1.24.7-alpine AS build

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE_CMD
RUN test -n "$SERVICE_CMD"
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ${SERVICE_CMD}

FROM alpine:3.20

RUN addgroup -S app && adduser -S -G app -u 10001 app \
    && apk add --no-cache ca-certificates tzdata \
    && mkdir -p /app /app/config /app/runtime \
    && chown -R app:app /app

WORKDIR /app

COPY --from=build /out/app /app/app

USER app

ENTRYPOINT ["/app/app"]