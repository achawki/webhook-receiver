package main

import (
	"github.com/achawki/webhook-receiver/cmd/receiver"
)

func main() {
	server := receiver.Setup()
	server.Start()
}
