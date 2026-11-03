package landing

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type mailer struct {
	opts Options
}

func newMailer(opts Options) *mailer {
	return &mailer{opts: opts}
}

func (m *mailer) Send(ctx context.Context, name, email, message string) error {
	if m.opts.SMTPHost == "" {
		return fmt.Errorf("smtp no configurado")
	}

	host := m.opts.SMTPHost
	port := m.opts.SMTPPort
	addr := net.JoinHostPort(host, port)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("conectar smtp: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	scheme := strings.ToLower(m.opts.SMTPSecure)
	if scheme == "" && port == "465" {
		scheme = "tls"
	}

	var client *smtp.Client
	if scheme == "tls" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("tls smtp: %w", err)
		}
		client, err = smtp.NewClient(tlsConn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		return fmt.Errorf("cliente smtp: %w", err)
	}
	defer client.Close()

	if scheme != "none" && scheme != "tls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if scheme == "starttls" {
			return fmt.Errorf("el servidor smtp no ofrece starttls")
		}
	}

	if m.opts.SMTPUsername != "" {
		auth := smtp.PlainAuth("", m.opts.SMTPUsername, m.opts.SMTPPassword, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("autenticar smtp: %w", err)
		}
	}

	from := m.opts.SMTPFrom
	if from == "" {
		from = m.opts.SMTPUsername
	}
	if from == "" {
		return fmt.Errorf("remitente smtp no configurado")
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("remitente: %w", err)
	}
	if err := client.Rcpt(m.opts.ContactTo); err != nil {
		return fmt.Errorf("destinatario: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("datos smtp: %w", err)
	}
	if _, err := w.Write(buildMessage(from, m.opts.ContactTo, name, email, message)); err != nil {
		_ = w.Close()
		return fmt.Errorf("escribir mensaje: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("cerrar mensaje: %w", err)
	}
	_ = client.Quit()
	return nil
}

func buildMessage(from, to, name, email, body string) []byte {
	name = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(name))
	message := strings.ReplaceAll(body, "\r\n", "\n")
	message = strings.ReplaceAll(message, "\n", "\r\n")

	replyTo := (&mail.Address{Name: name, Address: email}).String()

	var b strings.Builder
	b.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	b.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	b.WriteString("Reply-To: " + replyTo + "\r\n")
	b.WriteString("Subject: =?UTF-8?Q?Nuevo_mensaje_desde_aibotia.com?=\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString("Nombre: " + name + "\r\n")
	b.WriteString("Email: " + email + "\r\n\r\n")
	b.WriteString(message + "\r\n")
	return []byte(b.String())
}

func sanitizeHeader(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "").Replace(s))
}
