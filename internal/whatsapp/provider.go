// Package whatsapp implementa la integración con WhatsApp mediante whatsmeow.
package whatsapp

import (
	"context"
	"errors"
	"time"
)

// Status representa el estado de la conexión con WhatsApp.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusWaitingQR    Status = "waiting_qr"
	StatusConnected    Status = "connected"
	StatusLoggedOut    Status = "logged_out"
)

var (
	// ErrNotConnected indica que el cliente de WhatsApp no está disponible.
	ErrNotConnected = errors.New("whatsapp: no conectado")
)

// IncomingMessage es un mensaje normalizado recibido de WhatsApp.
type IncomingMessage struct {
	ID        string
	Chat      string
	Sender    string
	PushName  string
	Text      string
	Timestamp time.Time
	FromMe    bool
	HasMedia  bool
	MediaType string // image | video | audio | document | sticker | ""
}

// Contact es la información de contacto resumida.
type Contact struct {
	JID      string
	FullName string
	PushName string
	Business string
}

// OutboundMedia describe un adjunto a enviar.
type OutboundMedia struct {
	Data     []byte
	FileName string
	MimeType string
	Caption  string
	IsImage  bool
}

// Provider es la abstracción del canal WhatsApp.
type Provider interface {
	Start(ctx context.Context) error
	Stop()
	Status() Status
	QRCode() string
	Incoming() <-chan IncomingMessage
	SendText(ctx context.Context, to, text string) (string, error)
	SendMedia(ctx context.Context, to string, media OutboundMedia) (string, error)
	SendLocation(ctx context.Context, to string, lat, lng float64, name, address string) (string, error)
	DownloadMedia(ctx context.Context, messageID string) ([]byte, error)
	MarkSeen(ctx context.Context, chat, sender, messageID string) error
	SendTyping(ctx context.Context, chat string, composing bool) error
	GetContact(ctx context.Context, jid string) (Contact, error)
	ListContacts(ctx context.Context) ([]Contact, error)
	Logout(ctx context.Context) error
}
