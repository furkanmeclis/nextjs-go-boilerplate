package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	testSecretCode   = "483920"
	testInviteLink   = "https://app.example.test/invite/tok3n"
	testLoginSubject = "Your sign-in code: {{code}}"
	testLoginBody    = "Your code is {{code}}, or open {{link}}"
)

// sealQuerier keeps one row the way the database would.
type sealQuerier struct {
	pushTestQuerier
	row db.Notification
}

func (q *sealQuerier) GetTemplateByCodeChannelLang(context.Context, db.GetTemplateByCodeChannelLangParams) (db.NotificationTemplate, error) {
	return db.NotificationTemplate{Subject: testLoginSubject, Body: testLoginBody}, nil
}

func (q *sealQuerier) CreateNotification(_ context.Context, arg db.CreateNotificationParams) (db.Notification, error) {
	q.row = db.Notification{
		ID: 1, Channel: arg.Channel, Status: arg.Status, Title: arg.Title, Body: arg.Body,
		Recipient: arg.Recipient, TemplateCode: arg.TemplateCode, SealedContent: arg.SealedContent,
		Payload: arg.Payload,
	}
	return q.row, nil
}

func (q *sealQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	q.row.Status = model.StatusProcessing
	return q.row, nil
}

func (q *sealQuerier) MarkNotificationFailed(_ context.Context, arg db.MarkNotificationFailedParams) (db.Notification, error) {
	q.row.Status = model.StatusFailed
	q.row.LastError = arg.LastError
	return q.row, nil
}

func (q *sealQuerier) MarkNotificationSent(_ context.Context, arg db.MarkNotificationSentParams) (db.Notification, error) {
	q.row.Status = arg.Status
	q.row.SealedContent = pgtype.Text{}
	return q.row, nil
}

type recordingSender struct {
	failNext bool
	sent     []mail.Message
}

func (r *recordingSender) Send(_ context.Context, msg mail.Message) error {
	if r.failNext {
		r.failNext = false
		return errors.New("smtp down")
	}
	r.sent = append(r.sent, msg)
	return nil
}

type discardQueue struct{}

func (discardQueue) Enqueue(*asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}

func newSealService(t *testing.T, q Querier, sender mail.Sender) *Service {
	t.Helper()
	box, err := crypto.NewSecretBox(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(q, discardQueue{}, []providers.Provider{providers.EmailProvider{Mail: sender}}, slog.Default())
	return svc.WithSecretBox(box)
}

func securityEmail() model.EnqueueInput {
	to := "person@example.test"
	return model.EnqueueInput{
		Channels: []string{model.ChannelEmail}, Recipient: &to,
		TemplateCode: "auth.login_code", SecurityEmail: true,
		TemplateVars: map[string]string{"code": testSecretCode, "link": testInviteLink},
	}
}

func assertNoSecret(t *testing.T, where string, values ...string) {
	t.Helper()
	for _, v := range values {
		if strings.Contains(v, testSecretCode) || strings.Contains(v, testInviteLink) {
			t.Fatalf("%s leaks a secret: %q", where, v)
		}
	}
}

func TestSecurityEmailRowNeverHoldsPlaintext(t *testing.T) {
	t.Parallel()

	q := &sealQuerier{}
	svc := newSealService(t, q, &recordingSender{})
	items, err := svc.Enqueue(context.Background(), securityEmail())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if !q.row.SealedContent.Valid {
		t.Fatal("sealed_content not set")
	}
	assertNoSecret(t, "queued row", q.row.Title, q.row.Body, q.row.SealedContent.String, string(q.row.Payload))
	if q.row.Title != "auth.login_code" || q.row.Body != RedactedBody {
		t.Fatalf("row = %q / %q", q.row.Title, q.row.Body)
	}
	assertNoSecret(t, "API projection", items[0].Title, items[0].Body)
}

func TestSecurityEmailRetryDeliversRealContentThenDropsIt(t *testing.T) {
	t.Parallel()

	q := &sealQuerier{}
	sender := &recordingSender{failNext: true}
	svc := newSealService(t, q, sender)
	if _, err := svc.Enqueue(context.Background(), securityEmail()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := svc.Deliver(context.Background(), 1); err == nil {
		t.Fatal("first Deliver: want the smtp error")
	}
	if q.row.Status != model.StatusFailed || !q.row.SealedContent.Valid {
		t.Fatalf("after failure: status %q, sealed %v", q.row.Status, q.row.SealedContent.Valid)
	}
	assertNoSecret(t, "failed row", q.row.Title, q.row.Body, q.row.LastError.String)

	if err := svc.Deliver(context.Background(), 1); err != nil {
		t.Fatalf("retry Deliver: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sender.sent))
	}
	msg := sender.sent[0]
	if msg.Subject != "Your sign-in code: "+testSecretCode {
		t.Fatalf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, testSecretCode) || !strings.Contains(msg.Body, testInviteLink) {
		t.Fatalf("body = %q", msg.Body)
	}
	if q.row.Status != model.StatusSent || q.row.SealedContent.Valid {
		t.Fatalf("after send: status %q, sealed %v", q.row.Status, q.row.SealedContent.Valid)
	}
	assertNoSecret(t, "sent row", q.row.Title, q.row.Body)
}

func TestSecurityEmailWithoutSecretBoxFailsClosed(t *testing.T) {
	t.Parallel()

	q := &sealQuerier{}
	svc := New(q, discardQueue{}, nil, slog.Default())
	if _, err := svc.Enqueue(context.Background(), securityEmail()); !errors.Is(err, errSecretBoxMissing) {
		t.Fatalf("Enqueue error = %v", err)
	}
	if q.row.ID != 0 {
		t.Fatal("row written without encryption")
	}
}

func TestUndecryptableRowIsNotSentRedacted(t *testing.T) {
	t.Parallel()

	q := &sealQuerier{row: db.Notification{
		ID: 1, Channel: model.ChannelEmail, Status: model.StatusQueued, Title: "auth.login_code", Body: RedactedBody,
		Recipient:     pgtype.Text{String: "person@example.test", Valid: true},
		SealedContent: pgtype.Text{String: "bm90LXZhbGlk", Valid: true},
	}}
	sender := &recordingSender{}
	svc := newSealService(t, q, sender)
	if err := svc.Deliver(context.Background(), 1); err == nil {
		t.Fatal("Deliver: want an error")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sent = %d, want 0", len(sender.sent))
	}
}

func TestOrdinaryEmailKeepsItsContent(t *testing.T) {
	t.Parallel()

	q := &sealQuerier{}
	svc := newSealService(t, q, &recordingSender{})
	in := securityEmail()
	in.SecurityEmail = false
	if _, err := svc.Enqueue(context.Background(), in); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if q.row.SealedContent.Valid || q.row.Body == RedactedBody {
		t.Fatalf("ordinary email sealed: %+v", q.row)
	}
}
