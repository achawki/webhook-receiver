package receiver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/storage"
)

const (
	defaultStorePath      = "webhook-receiver.db"
	defaultListenAddr     = ":8080"
	webhookCleanupPeriod  = time.Minute
	defaultShutdownGrace  = 5 * time.Second
	encryptionKeyEnvName  = "WEBHOOK_RECEIVER_ENCRYPTION_KEY"
	publicBaseURLEnvName  = "WEBHOOK_RECEIVER_PUBLIC_BASE_URL"
	clientIPHeaderEnvName = "WEBHOOK_RECEIVER_CLIENT_IP_HEADER"
)

// Config contains runtime configuration for the webhook receiver.
type Config struct {
	ListenAddr     string
	StorePath      string
	EncryptionKey  string
	PublicBaseURL  string
	ClientIPHeader string
}

// Server holds the HTTP handler stack and persistent resources.
type Server struct {
	handler     *handler.Handler
	mux         *http.ServeMux
	httpServer  *http.Server
	store       *storage.SQLiteStore
	cleanupStop chan struct{}
	cleanupDone chan struct{}
	closeOnce   sync.Once
}

// Setup creates a new webhook receiver server and exits on configuration errors.
func Setup() *Server {
	server, err := NewServer(LoadConfigFromEnv())
	if err != nil {
		log.Fatalf("Could not set up webhook receiver: %s", err)
	}

	return server
}

// LoadConfigFromEnv reads runtime configuration from environment variables.
func LoadConfigFromEnv() Config {
	return Config{
		ListenAddr:     strings.TrimSpace(os.Getenv("WEBHOOK_RECEIVER_LISTEN_ADDR")),
		StorePath:      persistentStorePath(),
		EncryptionKey:  persistentStoreEncryptionKey(),
		PublicBaseURL:  strings.TrimSpace(os.Getenv(publicBaseURLEnvName)),
		ClientIPHeader: strings.TrimSpace(os.Getenv(clientIPHeaderEnvName)),
	}
}

// NewServer creates a configured webhook receiver instance.
func NewServer(config Config) (*Server, error) {
	log.Println("Setting up webhook receiver")

	if strings.TrimSpace(config.EncryptionKey) == "" {
		return nil, errors.New(encryptionKeyEnvName + " must be set to a base64 or hex encoded 32-byte key; generate one with: openssl rand -base64 32")
	}

	listenAddr := strings.TrimSpace(config.ListenAddr)
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}

	storePath := strings.TrimSpace(config.StorePath)
	if storePath == "" {
		storePath = defaultStorePath
	}

	publicBaseURL, err := normalizePublicBaseURL(config.PublicBaseURL)
	if err != nil {
		return nil, err
	}

	server := &Server{mux: http.NewServeMux()}
	persistentStore, err := storage.NewSQLiteStore(storePath, config.EncryptionKey)
	if err != nil {
		return nil, err
	}
	log.Printf("Using persistent store at %s", storePath)
	server.store = persistentStore

	handlerOptions := []handler.Option{
		handler.WithPublicBaseURL(publicBaseURL),
		handler.WithClientIPHeader(config.ClientIPHeader),
	}
	server.handler = handler.NewHandler(persistentStore, handlerOptions...)
	server.handler.Register(server.mux)
	server.httpServer = &http.Server{
		Addr:    listenAddr,
		Handler: server.mux,
	}

	return server, nil
}

// Start starts the webhook receiver
func (s *Server) Start() {
	if err := s.Run(context.Background()); err != nil {
		log.Fatalf("Couldn't start server: %s", err)
	}
}

// Run starts the HTTP server and stops it when the context is canceled.
func (s *Server) Run(ctx context.Context) error {
	if s.httpServer == nil {
		return errors.New("server is not initialized")
	}

	s.startCleanupLoop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Starting webhook receiver on %s...", s.httpServer.Addr)
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = s.Close()
			return err
		}
		return s.Close()
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownGrace)
		defer cancel()

		shutdownErr := s.Shutdown(shutdownCtx)
		serverErr := <-errCh
		if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			return serverErr
		}

		return shutdownErr
	}
}

// Shutdown gracefully stops the webhook receiver.
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopCleanupLoop()

	if s.httpServer == nil {
		return s.closeStore()
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		_ = s.closeStore()
		return err
	}

	return s.closeStore()
}

// Close releases persistent resources.
func (s *Server) Close() error {
	s.stopCleanupLoop()
	return s.closeStore()
}

func (s *Server) startCleanupLoop() {
	if s.store == nil || s.cleanupStop != nil || s.cleanupDone != nil {
		return
	}

	s.cleanupStop = make(chan struct{})
	s.cleanupDone = make(chan struct{})
	go s.runCleanupLoop()
}

func (s *Server) stopCleanupLoop() {
	if s.cleanupStop != nil && s.cleanupDone != nil {
		close(s.cleanupStop)
		<-s.cleanupDone
		s.cleanupStop = nil
		s.cleanupDone = nil
	}
}

func (s *Server) closeStore() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.store != nil {
			closeErr = s.store.Close()
			s.store = nil
		}
	})

	return closeErr
}

func (s *Server) runCleanupLoop() {
	ticker := time.NewTicker(webhookCleanupPeriod)
	defer ticker.Stop()
	defer close(s.cleanupDone)

	for {
		select {
		case <-ticker.C:
			deletedCount, err := s.store.DeleteExpiredWebhooks()
			if err != nil {
				log.Printf("Could not delete expired webhooks: %s", err)
				continue
			}
			if deletedCount > 0 {
				log.Printf("Deleted %d expired webhook(s)", deletedCount)
			}
		case <-s.cleanupStop:
			return
		}
	}
}

func persistentStorePath() string {
	configuredPath := strings.TrimSpace(os.Getenv("WEBHOOK_RECEIVER_STORE_PATH"))
	if configuredPath != "" {
		return configuredPath
	}

	return defaultStorePath
}

func persistentStoreEncryptionKey() string {
	return strings.TrimSpace(os.Getenv(encryptionKeyEnvName))
}

func normalizePublicBaseURL(publicBaseURL string) (string, error) {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if trimmedBaseURL == "" {
		return "", nil
	}

	parsedURL, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return "", err
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", errors.New(publicBaseURLEnvName + " must be an absolute URL")
	}

	return trimmedBaseURL, nil
}
