package receiver

import (
	"log"
	"net/http"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/storage"
)

//Server holds handlers
type Server struct {
	handler *handler.Handler
}

func (s *Server) setupHandlers() {
	http.HandleFunc("/", s.handler.UnknownHandler)
	http.HandleFunc("/api/webhooks", s.handler.WebhookHandler)
	http.HandleFunc("/api/webhooks/", s.handler.MessageHandler)
}

// Setup setups a new webhook receiver server.
// Initializes storage and handlers
func Setup() *Server {
	log.Println("Setting up webhook receiver")
	server := &Server{}
	inMemoryStore := storage.NewInMemoryStore()
	handler := handler.NewHandler(inMemoryStore)
	server.handler = handler
	server.setupHandlers()

	return server
}

// Start starts the webhook receiver
func (s *Server) Start() {
	log.Println("Starting webhook receiver on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Couldn't start server: %s", err)
	}
}
