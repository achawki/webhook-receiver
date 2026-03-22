package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/google/uuid"

	_ "github.com/mattn/go-sqlite3"
)

const sqliteTimeFormat = time.RFC3339Nano
const webhookTTL = 48 * time.Hour
const defaultMessagePageSize = 25
const maxMessagePageSize = 100
const maxMessagesPerWebhook = 100

const sqliteIndexes = `
CREATE INDEX IF NOT EXISTS idx_webhooks_expires_at ON webhooks(expires_at);
CREATE INDEX IF NOT EXISTS idx_messages_webhook_row_id ON messages(webhook_id, row_id);
`

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS webhooks (
	row_id INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	username TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL DEFAULT '',
	token_name TEXT NOT NULL DEFAULT '',
	token_value_hash TEXT NOT NULL DEFAULT '',
	hmac_header TEXT NOT NULL DEFAULT '',
	hmac_secret_ciphertext BLOB,
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	row_id INTEGER PRIMARY KEY AUTOINCREMENT,
	webhook_id TEXT NOT NULL,
	method TEXT NOT NULL,
	path TEXT NOT NULL,
	query TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL,
	headers_json TEXT NOT NULL DEFAULT '{}',
	status_code INTEGER NOT NULL DEFAULT 200,
	error_message TEXT NOT NULL DEFAULT '',
	received_at TEXT NOT NULL,
	FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
);
`

// SQLiteStore persists webhooks and messages in SQLite.
type SQLiteStore struct {
	db     *sql.DB
	cipher *secretCipher
}

// NewSQLiteStore creates or loads a SQLite-backed store.
func NewSQLiteStore(path string, encryptionKey string) (*SQLiteStore, error) {
	secretCipher, err := newSecretCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000", path))
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{db: db, cipher: secretCipher}
	if err := store.init(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}

	return store, nil
}

// Close releases the underlying database resources.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// InsertWebhook inserts provided webhooks.
func (s *SQLiteStore) InsertWebhook(webhook *model.Webhook) (string, error) {
	webhookID := uuid.New().String()
	webhook.ID = webhookID
	if webhook.ExpiresAt.IsZero() {
		webhook.ExpiresAt = time.Now().UTC().Add(webhookTTL)
	} else {
		webhook.ExpiresAt = webhook.ExpiresAt.UTC()
	}

	var encryptedHMACSecret []byte
	var err error
	if webhook.HasHMAC() {
		encryptedHMACSecret, err = s.cipher.Encrypt(webhook.HMACSecret())
		if err != nil {
			webhook.ID = ""
			return "", err
		}
	}

	_, err = s.db.Exec(
		`INSERT INTO webhooks (id, username, password_hash, token_name, token_value_hash, hmac_header, hmac_secret_ciphertext, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		webhook.ID,
		webhook.Username,
		webhook.PasswordHash(),
		webhook.TokenName,
		webhook.TokenValueHash(),
		webhook.HMACHeader,
		encryptedHMACSecret,
		webhook.ExpiresAt.Format(sqliteTimeFormat),
	)
	if err != nil {
		webhook.ID = ""
		return "", err
	}

	return webhookID, nil
}

// GetWebhook retrieves webhook with given ID.
func (s *SQLiteStore) GetWebhook(id string) (*model.Webhook, error) {
	now := time.Now().UTC().Format(sqliteTimeFormat)
	row := s.db.QueryRow(
		`SELECT id, username, password_hash, token_name, token_value_hash, hmac_header, hmac_secret_ciphertext, expires_at
		 FROM webhooks WHERE id = ? AND expires_at > ?`,
		id,
		now,
	)

	webhook, err := s.scanWebhook(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &WebhookNotFoundError{WebhookId: id}
		}
		return nil, err
	}

	return webhook, nil
}

// ListWebhooks lists all stored webhooks in reverse creation order.
func (s *SQLiteStore) ListWebhooks() (webhooks []*model.Webhook, err error) {
	now := time.Now().UTC().Format(sqliteTimeFormat)
	rows, err := s.db.Query(
		`SELECT id, username, password_hash, token_name, token_value_hash, hmac_header, hmac_secret_ciphertext, expires_at
		 FROM webhooks
		 WHERE expires_at > ?
		 ORDER BY row_id DESC`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	webhooks = []*model.Webhook{}
	for rows.Next() {
		webhook, err := s.scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		webhooks = append(webhooks, webhook)
	}

	return webhooks, rows.Err()
}

// InsertMessage inserts message for given webhook ID.
func (s *SQLiteStore) InsertMessage(webhookID string, message *model.Message) (err error) {
	headersJSON, err := json.Marshal(message.Headers)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); err == nil && rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = rollbackErr
		}
	}()

	exists, err := s.webhookExistsTx(tx, webhookID)
	if err != nil {
		return err
	}
	if !exists {
		return &WebhookNotFoundError{WebhookId: webhookID}
	}

	_, err = tx.Exec(
		`INSERT INTO messages (webhook_id, method, path, query, payload, headers_json, status_code, error_message, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		webhookID,
		message.Method,
		message.Path,
		message.Query,
		message.Payload,
		string(headersJSON),
		message.StatusCode,
		message.ErrorMessage,
		message.Time.Format(sqliteTimeFormat),
	)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM messages
		 WHERE webhook_id = ?
		   AND row_id NOT IN (
			SELECT row_id FROM messages
			WHERE webhook_id = ?
			ORDER BY row_id DESC
			LIMIT ?
		   )`,
		webhookID,
		webhookID,
		maxMessagesPerWebhook,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// GetMessagePageForWebhook retrieves a page of messages for given webhook ID.
func (s *SQLiteStore) GetMessagePageForWebhook(webhookID string, page int, pageSize int, outcome model.MessageOutcome) (messagePage *model.MessagePage, err error) {
	page, pageSize = normalizePagination(page, pageSize)
	outcome, _ = model.ParseMessageOutcome(string(outcome))

	exists, err := s.webhookExists(webhookID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &WebhookNotFoundError{WebhookId: webhookID}
	}

	var totalMessages int
	countQuery := `SELECT COUNT(*) FROM messages WHERE webhook_id = ?`
	countArgs := []interface{}{webhookID}
	if querySuffix, queryArgs := outcomeQueryFilter(outcome); querySuffix != "" {
		countQuery += querySuffix
		countArgs = append(countArgs, queryArgs...)
	}
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&totalMessages); err != nil {
		return nil, err
	}

	totalPages := 0
	if totalMessages > 0 {
		totalPages = (totalMessages + pageSize - 1) / pageSize
		if page > totalPages {
			page = totalPages
		}
	} else {
		page = 1
	}

	offset := (page - 1) * pageSize
	messageQuery := `SELECT method, path, query, payload, headers_json, status_code, error_message, received_at
		 FROM messages
		 WHERE webhook_id = ?`
	messageArgs := []interface{}{webhookID}
	if querySuffix, queryArgs := outcomeQueryFilter(outcome); querySuffix != "" {
		messageQuery += querySuffix
		messageArgs = append(messageArgs, queryArgs...)
	}
	messageQuery += `
		 ORDER BY row_id DESC
		 LIMIT ? OFFSET ?`
	messageArgs = append(messageArgs, pageSize, offset)

	rows, err := s.db.Query(messageQuery, messageArgs...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	messages := []*model.Message{}
	for rows.Next() {
		var (
			method       string
			path         string
			query        string
			payload      string
			headersJSON  string
			statusCode   int
			errorMessage string
			receivedAt   string
		)

		if err := rows.Scan(&method, &path, &query, &payload, &headersJSON, &statusCode, &errorMessage, &receivedAt); err != nil {
			return nil, err
		}

		headers := map[string][]string{}
		if headersJSON != "" && headersJSON != "null" {
			if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
				return nil, err
			}
		}

		message := &model.Message{
			Method:       method,
			Path:         path,
			Query:        query,
			Payload:      payload,
			Headers:      headers,
			StatusCode:   statusCode,
			ErrorMessage: errorMessage,
		}
		parsedTime, err := time.Parse(sqliteTimeFormat, receivedAt)
		if err != nil {
			return nil, err
		}
		message.Time = parsedTime

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	messagePage = &model.MessagePage{
		Messages:        messages,
		Page:            page,
		PageSize:        pageSize,
		TotalMessages:   totalMessages,
		TotalPages:      totalPages,
		HasNextPage:     totalPages > 0 && page < totalPages,
		HasPreviousPage: page > 1 && totalPages > 0,
	}

	return messagePage, nil
}

func (s *SQLiteStore) init() error {
	if _, err := s.db.Exec(sqliteSchema); err != nil {
		return err
	}

	if _, err := s.db.Exec(sqliteIndexes); err != nil {
		return err
	}

	_, err := s.DeleteExpiredWebhooks()
	return err
}

func (s *SQLiteStore) webhookExists(webhookID string) (bool, error) {
	return s.webhookExistsQuery(s.db, webhookID)
}

func (s *SQLiteStore) webhookExistsTx(tx *sql.Tx, webhookID string) (bool, error) {
	return s.webhookExistsQuery(tx, webhookID)
}

type webhookExistenceQuerier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

func (s *SQLiteStore) webhookExistsQuery(queryer webhookExistenceQuerier, webhookID string) (bool, error) {
	var exists int
	err := queryer.QueryRow(
		`SELECT 1 FROM webhooks WHERE id = ? AND expires_at > ?`,
		webhookID,
		time.Now().UTC().Format(sqliteTimeFormat),
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStore) scanWebhook(scanner rowScanner) (*model.Webhook, error) {
	var (
		id                   string
		username             string
		passwordHash         string
		tokenName            string
		tokenValueHash       string
		hmacHeader           string
		hmacSecretCiphertext []byte
		expiresAtRaw         string
	)

	if err := scanner.Scan(&id, &username, &passwordHash, &tokenName, &tokenValueHash, &hmacHeader, &hmacSecretCiphertext, &expiresAtRaw); err != nil {
		return nil, err
	}

	hmacSecret := ""
	if len(hmacSecretCiphertext) > 0 {
		decryptedSecret, err := s.cipher.Decrypt(hmacSecretCiphertext)
		if err != nil {
			return nil, err
		}
		hmacSecret = decryptedSecret
	}

	expiresAt, err := time.Parse(sqliteTimeFormat, expiresAtRaw)
	if err != nil {
		return nil, err
	}

	return model.NewStoredWebhook(id, username, passwordHash, tokenName, tokenValueHash, hmacHeader, hmacSecret, expiresAt), nil
}

// DeleteExpiredWebhooks removes expired webhooks and their captured messages.
func (s *SQLiteStore) DeleteExpiredWebhooks() (deletedCount int, err error) {
	cutoff := time.Now().UTC().Format(sqliteTimeFormat)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); err == nil && rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = rollbackErr
		}
	}()

	if _, err := tx.Exec(
		`DELETE FROM messages
		 WHERE webhook_id IN (
			SELECT id FROM webhooks WHERE expires_at <= ?
		 )`,
		cutoff,
	); err != nil {
		return 0, err
	}

	result, err := tx.Exec(`DELETE FROM webhooks WHERE expires_at <= ?`, cutoff)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	deletedRows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	deletedCount = int(deletedRows)
	return deletedCount, nil
}

type secretCipher struct {
	aead cipher.AEAD
}

func newSecretCipher(encryptionKey string) (*secretCipher, error) {
	key, err := decodeEncryptionKey(encryptionKey)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &secretCipher{aead: aead}, nil
}

func (c *secretCipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return append(nonce, ciphertext...), nil
}

func (c *secretCipher) Decrypt(ciphertext []byte) (string, error) {
	nonceSize := c.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext is too short")
	}

	nonce := ciphertext[:nonceSize]
	encrypted := ciphertext[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func decodeEncryptionKey(encryptionKey string) ([]byte, error) {
	trimmedKey := strings.TrimSpace(encryptionKey)
	if trimmedKey == "" {
		return nil, errors.New("encryption key must not be empty")
	}

	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		hex.DecodeString,
	}

	for _, decode := range decoders {
		decoded, err := decode(trimmedKey)
		if err != nil {
			continue
		}

		if len(decoded) == 32 {
			return decoded, nil
		}
	}

	return nil, errors.New("encryption key must be base64 or hex encoded 32-byte data")
}

func normalizePagination(page int, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultMessagePageSize
	}
	if pageSize > maxMessagePageSize {
		pageSize = maxMessagePageSize
	}

	return page, pageSize
}

func outcomeQueryFilter(outcome model.MessageOutcome) (string, []interface{}) {
	switch outcome {
	case model.MessageOutcomeAccepted:
		return " AND status_code < ?", []interface{}{400}
	case model.MessageOutcomeRejected:
		return " AND status_code >= ?", []interface{}{400}
	default:
		return "", nil
	}
}
