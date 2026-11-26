package mailer

import (
	"bufio"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a server that speaks just enough SMTP, with no encryption.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	got  []string // raw messages
	rcpt []string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *fakeSMTP) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	say := func(l string) { _, _ = io.WriteString(c, l+"\r\n") }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 fake")
		case strings.HasPrefix(cmd, "MAIL"):
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(line))
			s.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(strings.TrimPrefix(l, "."))
			}
			s.mu.Lock()
			s.got = append(s.got, b.String())
			s.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (s *fakeSMTP) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

func mailerFor(t *testing.T, s *fakeSMTP, tls string) *Mailer {
	t.Helper()
	m, err := New(Config{Host: "127.0.0.1", Port: s.port(), From: "Nutreek <nutreek@example.test>", TLS: tls})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSendBuildsAMultipartMessage(t *testing.T) {
	s := newFakeSMTP(t)
	m := mailerFor(t, s, TLSNone)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Send(ctx, "Carlo <carlo@example.test>", "Reimposta la password è facile", "Ciao è\nlink: https://x.test/reset/abc", "<p>Ciao è</p>"); err != nil {
		t.Fatal(err)
	}
	msgs := s.messages()
	if len(msgs) != 1 {
		t.Fatalf("messages = %d", len(msgs))
	}
	msg, err := mail.ReadMessage(strings.NewReader(msgs[0]))
	if err != nil {
		t.Fatal(err)
	}
	subj, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subj != "Reimposta la password è facile" {
		t.Fatalf("subject = %q, %v", subj, err)
	}
	if got := msg.Header.Get("From"); !strings.Contains(got, "nutreek@example.test") {
		t.Fatalf("from = %q", got)
	}
	if msg.Header.Get("Message-Id") == "" || msg.Header.Get("Date") == "" {
		t.Fatal("Message-ID and Date are expected")
	}
	typ, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || typ != "multipart/alternative" {
		t.Fatalf("content type = %q, %v", typ, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var kinds, bodies []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(quotedprintable.NewReader(p))
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		kinds = append(kinds, ct)
		bodies = append(bodies, string(raw))
	}
	if len(kinds) != 2 || kinds[0] != "text/plain" || kinds[1] != "text/html" {
		t.Fatalf("parts = %v", kinds)
	}
	if !strings.Contains(bodies[0], "Ciao è") || !strings.Contains(bodies[0], "https://x.test/reset/abc") || !strings.Contains(bodies[1], "<p>Ciao è</p>") {
		t.Fatalf("bodies = %q", bodies)
	}
}

func TestSendRefusesHeaderInjection(t *testing.T) {
	s := newFakeSMTP(t)
	m := mailerFor(t, s, TLSNone)
	ctx := context.Background()
	if err := m.Send(ctx, "a@example.test", "hi\r\nBcc: x@example.test", "t", "h"); err == nil {
		t.Fatal("a line break in the subject must be refused")
	}
	if err := m.Send(ctx, "a@example.test\r\nBcc: x@example.test", "hi", "t", "h"); err == nil {
		t.Fatal("a line break in the recipient must be refused")
	}
	if n := len(s.messages()); n != 0 {
		t.Fatalf("nothing should be sent, got %d", n)
	}
}

func TestStartTLSIsRequiredWhenAsked(t *testing.T) {
	s := newFakeSMTP(t) // offers no STARTTLS
	m := mailerFor(t, s, TLSStartTLS)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := m.Send(ctx, "a@example.test", "hi", "t", "h")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v, want a refusal that names STARTTLS", err)
	}
	if n := len(s.messages()); n != 0 {
		t.Fatalf("nothing should be sent in clear, got %d", n)
	}
}

func TestNewChecksTheSettings(t *testing.T) {
	good := Config{Host: "smtp.example.test", Port: 587, From: "n@example.test", TLS: TLSStartTLS}
	if _, err := New(good); err != nil {
		t.Fatal(err)
	}
	for name, mod := range map[string]func(*Config){
		"no host":  func(c *Config) { c.Host = "" },
		"bad port": func(c *Config) { c.Port = 0 },
		"bad tls":  func(c *Config) { c.TLS = "maybe" },
		"bad from": func(c *Config) { c.From = "not an address" },
	} {
		c := good
		mod(&c)
		if _, err := New(c); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
