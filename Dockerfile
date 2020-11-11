FROM golang:1.15 as build
WORKDIR /go/webhook_receiver
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=build /go/webhook_receiver/webhook-receiver /go/
EXPOSE 8080
CMD ["/go/webhook-receiver"]