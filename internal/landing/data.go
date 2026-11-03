package landing

type Site struct {
	Name        string
	Tagline     string
	Email       string
	Phone       string
	WhatsApp    string
	WhatsAppURL string
	Location    string
	GitHub      string
	Web         string
	LegalNote   string
}

type Feature struct {
	Icon  string
	Title string
	Text  string
}

type Plan struct {
	Eyebrow     string
	Name        string
	Description string
	Price       string
	PriceNote   string
	Features    []string
	Highlight   bool
	CTA         string
}

type Advantage struct {
	Icon  string
	Title string
	Text  string
}

type SummaryPoint struct {
	Title string
	Text  string
}

type FormState struct {
	OK      bool
	Error   string
	Name    string
	Email   string
	Message string
}

type PageData struct {
	Title       string
	Description string
	Canonical   string
	Path        string
	Year        int

	Site       Site
	Features   []Feature
	Plans      []Plan
	Advantages []Advantage
	Summary    []SummaryPoint

	Form FormState

	// TurnstileSiteKey muestra el widget de Cloudflare si no está vacío.
	TurnstileSiteKey string
}
