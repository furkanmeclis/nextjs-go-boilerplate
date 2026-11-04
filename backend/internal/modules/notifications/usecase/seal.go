package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// RedactedBody stands in for the content of a security email on its row. Same
// literal as migration 000033.
const RedactedBody = "[redacted]"

var errSecretBoxMissing = errors.New("notifications: secret box not configured")

// sealedContent is what a security email must still deliver. Subjects count:
// the login, link and deactivation templates put the code in the subject.
type sealedContent struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// sealContent encrypts a security email for the row's sealed_content column.
// Without a box it fails rather than storing the secret in plaintext.
func (s *Service) sealContent(title, body string) (pgtype.Text, error) {
	if s.box == nil {
		return pgtype.Text{}, errSecretBoxMissing
	}
	raw, err := json.Marshal(sealedContent{Title: title, Body: body})
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("seal notification: %w", err)
	}
	enc, err := s.box.Encrypt(string(raw))
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("seal notification: %w", err)
	}
	return pgtype.Text{String: enc, Valid: true}, nil
}

// deliverVia hands the provider the real content of a sealed row. row is a copy:
// the plaintext never reaches the caller, the history or a status write.
func (s *Service) deliverVia(ctx context.Context, p providers.Provider, row db.Notification, userUUID *uuid.UUID) (providers.DeliveryResult, error) {
	if row.SealedContent.Valid {
		if s.box == nil {
			return providers.DeliveryResult{}, errSecretBoxMissing
		}
		raw, err := s.box.Decrypt(row.SealedContent.String)
		if err != nil {
			return providers.DeliveryResult{}, fmt.Errorf("open sealed notification: %w", err)
		}
		var c sealedContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return providers.DeliveryResult{}, fmt.Errorf("open sealed notification: %w", err)
		}
		row.Title, row.Body = c.Title, c.Body
	}
	return p.Deliver(ctx, row, userUUID)
}

// redactedTitle identifies a sealed row in operator views without its subject.
func redactedTitle(templateCode string) string {
	if templateCode != "" {
		return templateCode
	}
	return RedactedBody
}
