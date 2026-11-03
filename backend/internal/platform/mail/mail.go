package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
)

// defaultTimeout bounds a send when none is configured.
const defaultTimeout = 10 * time.Second

// Message is an outbound email.
type Message struct {
	To      []string
	Subject string
	Body    string
}

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// SMTPSender sends mail via SMTP (MailHog in development).
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	timeout  time.Duration
	// tlsMode is one of auto|starttls|implicit|none (see config.SMTPConfig).
	tlsMode  string
	insecure bool
}

// NewSMTPSender builds a sender from config.
func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if mode == "" {
		mode = "auto"
	}
	return &SMTPSender{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
		fromName: cfg.FromName,
		timeout:  timeout,
		tlsMode:  mode,
		insecure: cfg.TLSInsecureSkipVerify,
	}
}

// Send delivers a plain-text email.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if s == nil {
		return fmt.Errorf("mail: sender is nil")
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("mail: recipient required")
	}
	from := s.from
	if from == "" {
		from = "noreply@localhost"
	}
	fromHeader := from
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.fromName, from)
	}
	// Header values must not carry CR/LF: subjects interpolate user-chosen
	// names and a newline would inject extra headers (Bcc, etc.).
	for _, to := range msg.To {
		if strings.ContainsAny(to, "\r\n") {
			return fmt.Errorf("mail: invalid recipient")
		}
	}
	subject := mime.QEncoding.Encode("utf-8", stripHeaderBreaks(msg.Subject))
	payload := strings.Builder{}
	payload.WriteString("From: ")
	payload.WriteString(fromHeader)
	payload.WriteString("\r\n")
	payload.WriteString("To: ")
	payload.WriteString(strings.Join(msg.To, ", "))
	payload.WriteString("\r\n")
	payload.WriteString("Subject: ")
	payload.WriteString(subject)
	payload.WriteString("\r\n")
	payload.WriteString("MIME-Version: 1.0\r\n")
	payload.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	payload.WriteString("\r\n")
	payload.WriteString(msg.Body)

	if err := s.deliver(ctx, from, msg.To, []byte(payload.String())); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

// deliver opens an SMTP connection (with timeout and the configured transport
// security) and sends one message. net/smtp's SendMail offers neither a
// timeout nor implicit TLS, so the client flow is driven explicitly here.
func (s *SMTPSender) deliver(ctx context.Context, from string, to []string, payload []byte) error {
	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	// Overall deadline: the smaller of the configured timeout and any deadline
	// already on the context.
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := &net.Dialer{Timeout: timeout}
	// Bound dial + handshake by the same deadline and make them cancellable.
	dctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Resolve transport: implicit TLS wraps the socket immediately (port 465);
	// opportunistic/required STARTTLS upgrades a plaintext connection.
	mode := s.tlsMode
	implicit := mode == "implicit" || (mode == "auto" && s.port == 465)
	useSTARTTLS := mode == "starttls" || (mode == "auto" && !implicit)
	requireSTARTTLS := mode == "starttls"

	tlsConfig := &tls.Config{
		ServerName:         s.host,
		InsecureSkipVerify: s.insecure, //nolint:gosec // guarded: config refuses this outside development
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	var err error
	if implicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(dctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(dctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	if useSTARTTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if requireSTARTTLS {
			return fmt.Errorf("server at %s does not advertise STARTTLS", addr)
		}
		// In "auto" mode a server that does not advertise STARTTLS is spoken to
		// in plaintext (opportunistic). An on-path attacker could strip the
		// advertisement, so production should set SMTP_TLS=starttls or implicit
		// rather than rely on auto.
	}

	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt to: %w", err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := wc.Write(payload); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	// The server has accepted the message at this point; a failed QUIT (e.g.
	// the server drops the connection) must not report the send as failed, or
	// the queue would retry and deliver a duplicate.
	_ = c.Quit()
	return nil
}

func stripHeaderBreaks(v string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(v)
}

// NoopSender discards mail (tests).
type NoopSender struct{}

// Send implements Sender.
func (NoopSender) Send(context.Context, Message) error { return nil }

var (
	_ Sender = (*SMTPSender)(nil)
	_ Sender = NoopSender{}
)
