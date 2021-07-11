# Webhook Receiver
[![Go Report Card](https://goreportcard.com/badge/achawki/webhook-receiver?style=flat)](https://goreportcard.com/report/achawki/webhook-receiver) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/achawki/webhook-receiver/blob/master/LICENSE)

Minimal go application to receive messages from webhooks. Content is currently stored in-memory.

### Run server

From Dockerfile
```
docker run --publish 8080:8080 --detach achawki/webhook-receiver:latest
```
Via `go get`:
```
go get github.com/achawki/webhook-receiver
webhook-receiver
```
From source. Requires golang `1.14+`
```
go run main.go
```

### Use server with ngrok

Setup ngrok as described in https://ngrok.com/  
Run server:
```
docker run --publish 8080:8080 --detach achawki/webhook-receiver:latest
ngrok http 8080
```

### Create receiver for webhook
```
curl --header "Content-Type: application/json" --request POST --data '{}' http://localhost:8080/api/webhooks
```
ID in response needs to be used to further calls
```
{"id":"010d1338-5323-4e3d-93a9-4277bae8d7c4"}
```
#### Create webhook receiver with basic auth
```
curl --header "Content-Type: application/json" --request POST --data '{"username": "username", "password": "password"}' http://localhost:8080/api/webhooks
```
#### Create webhook receiver with token
```
curl --header "Content-Type: application/json" --request POST --data '{"tokenName": "Auth-Token", "tokenValue": "token"}' http://localhost:8080/api/webhooks
```

### Send messages to webhook receiver

```
curl --header "Content-Type: application/json" --request POST --data '{"information": "content"}' http://localhost:8080/api/webhooks/WEBHOOK_ID/messages
```

In case basic auth or token is set, corresponding header needs to be set otherwise `401` will be returned:
```
curl --header "Content-Type: application/json" --header "Auth-Token: token"  --request POST  --data '{"information": "content"}' http://localhost:8080/api/webhooks/WEBHOOK_ID/messages
```

### Show messages for webhook

```
curl  http://localhost:8080/api/webhooks/WEBHOOK_ID/messages
```
```
[
  {
    "payload": "{\"information\": \"content\"}",
    "headers": {
      "Accept": [
        "*/*"
      ],
      "Content-Length": [
        "26"
      ],
      "Content-Type": [
        "application/json"
      ],
      "User-Agent": [
        "curl/7.64.1"
      ]
    },
    "time": "2020-11-10T20:00:00.000000+01:00"
  },
 ...
]
```
