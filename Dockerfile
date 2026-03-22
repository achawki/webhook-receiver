FROM golang:1.25-alpine AS build
RUN apk --no-cache add build-base
WORKDIR /go/webhook_receiver
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o webhook-receiver .

FROM alpine:3.22
RUN apk --no-cache add ca-certificates libgcc \
  && addgroup -S webhook \
  && adduser -S -G webhook webhook \
  && mkdir -p /app /data \
  && chown -R webhook:webhook /app /data
WORKDIR /app
ENV WEBHOOK_RECEIVER_LISTEN_ADDR=:8080
ENV WEBHOOK_RECEIVER_STORE_PATH=/data/webhook-receiver.db
COPY --from=build /go/webhook_receiver/webhook-receiver /usr/local/bin/webhook-receiver
USER webhook
EXPOSE 8080
CMD ["/usr/local/bin/webhook-receiver"]
