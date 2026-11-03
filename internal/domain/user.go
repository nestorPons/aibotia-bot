// Package domain contiene las entidades y reglas de negocio.
package domain

// Role es el rol de un usuario.
type Role int

const (
	RoleAdmin  Role = 0
	RoleStaff  Role = 1
	RoleClient Role = 2
)

// User representa a un usuario de WhatsApp.
type User struct {
	ID          string
	Name        string
	Alias       string
	Active      bool
	Topic       string
	LastTopic   string
	Lang        string
	Role        Role
	BufferMs    int
	Temperature float64
	IsNew       bool
	// TestMode evita que las acciones de derivación desactiven la IA durante
	// una conversación de pruebas y añade una nota explicativa.
	TestMode bool
}

// IsClient indica si el usuario es cliente.
func (u User) IsClient() bool { return u.Role == RoleClient }
