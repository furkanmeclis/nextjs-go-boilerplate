package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStripHeaderBreaks(t *testing.T) {
	got := stripHeaderBreaks("Hi Ali\r\nBcc: attacker@example.com\nX")
	if got != "Hi Ali Bcc: attacker@example.com X" {
		t.Fatalf("got %q", got)
	}
}

// fakeSMTP is a tiny scripted SMTP server for tests. advertiseSTARTTLS controls
// whether EHLO offers STARTTLS; tlsConfig (when non-nil) wraps the listener for
// implicit-TLS tests.
type fakeSMTP struct {
	addr              string
	ln                net.Listener
	advertiseSTARTTLS bool

	mu       sync.Mutex
	received strings.Builder
	wg       sync.WaitGroup
}

func startFakeSMTP(t *testing.T, advertiseSTARTTLS bool, tlsConfig *tls.Config) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if tlsConfig != nil {
		ln = tls.NewListener(ln, tlsConfig)
	}
	f := &fakeSMTP{addr: ln.Addr().String(), ln: ln, advertiseSTARTTLS: advertiseSTARTTLS}
	f.wg.Add(1)
	go f.serve()
	t.Cleanup(func() { _ = ln.Close(); f.wg.Wait() })
	return f
}

func (f *fakeSMTP) serve() {
	defer f.wg.Done()
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	br := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s)) }

	w("220 fake ESMTP ready\r\n")
	inData := false
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if strings.TrimRight(line, "\r\n") == "." {
				inData = false
				w("250 OK queued\r\n")
				continue
			}
			f.mu.Lock()
			f.received.WriteString(line)
			f.mu.Unlock()
			continue
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if f.advertiseSTARTTLS {
				w("250-fake\r\n250 STARTTLS\r\n")
			} else {
				w("250-fake\r\n250 HELP\r\n")
			}
		case strings.HasPrefix(cmd, "HELO"):
			w("250 fake\r\n")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			w("250 OK\r\n")
		case strings.HasPrefix(cmd, "RCPT TO"):
			w("250 OK\r\n")
		case cmd == "DATA":
			inData = true
			w("354 end with .\r\n")
		case cmd == "QUIT":
			w("221 bye\r\n")
			return
		default:
			w("250 OK\r\n")
		}
	}
}

func (f *fakeSMTP) body() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.received.String()
}

func mustPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

func senderTo(t *testing.T, f *fakeSMTP, mode string, insecure bool) *SMTPSender {
	host, port := mustPort(t, f.addr)
	return &SMTPSender{
		host:     host,
		port:     port,
		from:     "noreply@example.com",
		fromName: "App",
		timeout:  2 * time.Second,
		tlsMode:  mode,
		insecure: insecure,
	}
}

// selfSignedTLS returns a server TLS config with a throwaway cert for 127.0.0.1.
func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

func TestSendPlaintextDelivers(t *testing.T) {
	f := startFakeSMTP(t, false, nil)
	s := senderTo(t, f, "none", false)

	err := s.Send(context.Background(), Message{
		To:      []string{"user@example.com"},
		Subject: "Merhaba Çağrı",
		Body:    "hello body",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	body := f.body()
	if !strings.Contains(body, "hello body") {
		t.Errorf("server did not receive body: %q", body)
	}
	if !strings.Contains(body, "To: user@example.com") {
		t.Errorf("missing To header: %q", body)
	}
}

func TestSendImplicitTLSDelivers(t *testing.T) {
	f := startFakeSMTP(t, false, selfSignedTLS(t))
	// Self-signed cert, so skip verification; this exercises the implicit-TLS
	// transport path (tls.Dialer) end to end.
	s := senderTo(t, f, "implicit", true)

	if err := s.Send(context.Background(), Message{
		To:      []string{"user@example.com"},
		Subject: "secure",
		Body:    "ciphertext body",
	}); err != nil {
		t.Fatalf("Send over implicit TLS: %v", err)
	}
	if !strings.Contains(f.body(), "ciphertext body") {
		t.Errorf("server did not receive body over TLS: %q", f.body())
	}
}

func TestSendRequiresSTARTTLSWhenNotOffered(t *testing.T) {
	f := startFakeSMTP(t, false, nil) // server does NOT advertise STARTTLS
	s := senderTo(t, f, "starttls", false)

	err := s.Send(context.Background(), Message{To: []string{"user@example.com"}, Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected error when STARTTLS is required but not offered")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("error = %v, want STARTTLS mention", err)
	}
}

func TestSendTimesOutOnStalledServer(t *testing.T) {
	// Listener accepts but never writes the greeting; the deadline must fire.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(2 * time.Second)
		_ = conn.Close()
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s := &SMTPSender{host: host, port: port, from: "a@b.c", timeout: 300 * time.Millisecond, tlsMode: "none"}

	start := time.Now()
	err = s.Send(context.Background(), Message{To: []string{"user@example.com"}, Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Errorf("send took %v, timeout not enforced", elapsed)
	}
}

func TestSendRejectsHeaderInjection(t *testing.T) {
	s := &SMTPSender{host: "127.0.0.1", port: 2525, from: "a@b.c", timeout: time.Second, tlsMode: "none"}
	err := s.Send(context.Background(), Message{To: []string{"ok@example.com\r\nBcc: evil@example.com"}, Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected error for CRLF in recipient")
	}
}
