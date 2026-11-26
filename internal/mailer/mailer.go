// Package mailer sends the few emails Nutreek needs (password recovery, #334) through
// an SMTP server the operator runs or rents.
package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// How the connection to the server is secured (NUTREEK_SMTP_TLS).
const (
	TLSStartTLS = "starttls" // plain connection, then STARTTLS (port 587); the default
	TLSImplicit = "tls"      // TLS from the first byte (port 465)
	TLSNone     = "none"     // no encryption: a relay on the same machine or network
)

// Config is what the operator sets with NUTREEK_SMTP_*.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string // "Nutreek <nutreek@example.com>" or a bare address
	TLS      string
}

// Mailer sends through one server.
type Mailer struct {
	cfg  Config
	from *mail.Address
}

// New checks the settings; it makes no connection.
func New(cfg Config) (*Mailer, error) {
	if cfg.Host == "" {
		return nil, errors.New("mailer: no host")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("mailer: port %d", cfg.Port)
	}
	switch cfg.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return nil, fmt.Errorf("mailer: TLS mode %q (want %s, %s or %s)", cfg.TLS, TLSStartTLS, TLSImplicit, TLSNone)
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mailer: from address: %w", err)
	}
	return &Mailer{cfg: cfg, from: from}, nil
}

// Send delivers one message with a plain-text and an HTML version. A header value
// with a line break is refused: it could add headers of its own.
func (m *Mailer) Send(ctx context.Context, to, subject, text, html string) error {
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mailer: recipient: %w", err)
	}
	if strings.ContainsAny(subject, "\r\n") {
		return errors.New("mailer: line break in the subject")
	}
	msg, err := m.build(rcpt, subject, text, html)
	if err != nil {
		return err
	}
	c, err := m.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if m.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err := c.Mail(m.from.Address); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(rcpt.Address); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: end of data: %w", err)
	}
	return c.Quit()
}

func (m *Mailer) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	d := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mailer: connect: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	if m.cfg.TLS == TLSImplicit {
		conn = tls.Client(conn, tlsCfg)
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mailer: greeting: %w", err)
	}
	if m.cfg.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			_ = c.Close()
			return nil, errors.New("mailer: the server does not offer STARTTLS (set NUTREEK_SMTP_TLS to none for a relay you trust)")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("mailer: STARTTLS: %w", err)
		}
	}
	return c, nil
}

func (m *Mailer) build(to *mail.Address, subject, text, html string) ([]byte, error) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	boundary := hex.EncodeToString(id)
	domain := "nutreek.invalid"
	if i := strings.LastIndex(m.from.Address, "@"); i >= 0 {
		domain = m.from.Address[i+1:]
	}
	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", m.from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", subject))
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")

	mw := multipart.NewWriter(&b)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, err
	}
	for _, p := range []struct{ typ, body string }{{"text/plain", text}, {"text/html", html}} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.typ + `; charset="utf-8"`},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := io.WriteString(qp, p.body); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
