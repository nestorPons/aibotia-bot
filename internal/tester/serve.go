package tester

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReportsHandler sirve el directorio de reportes con un índice navegable. Los
// archivos HTML son logs autocontenidos pensados para enseñar o imprimir.
func ReportsHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(renderReportsIndex(dir)))
	})
}

type reportItem struct {
	Name       string
	ClientName string
	Title      string
	Mod        time.Time
	JSON       bool
	MD         bool
}

func renderReportsIndex(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "<!doctype html><meta charset=\"utf-8\"><p>No se pudo leer el directorio de reportes.</p>"
	}
	items := make([]reportItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".html") {
			continue
		}
		// La página de cliente se enlaza desde su reporte técnico, no como
		// entrada duplicada en el índice.
		if strings.HasSuffix(strings.ToLower(e.Name()), "-cliente.html") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		base := strings.TrimSuffix(path, ".html")
		clientName := ""
		if fileExists(base + "-cliente.html") {
			clientName = filepath.Base(base + "-cliente.html")
		}
		items = append(items, reportItem{
			Name:       e.Name(),
			ClientName: clientName,
			Title:      reportTitle(path),
			Mod:        info.ModTime(),
			JSON:       fileExists(base + ".json"),
			MD:         fileExists(base + ".md"),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Mod.After(items[j].Mod) })

	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"es\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>Logs de pruebas · aibotia</title><style>")
	b.WriteString(`*{box-sizing:border-box}body{margin:0;background:#f0f2f5;color:#111b21;font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
.wrap{max-width:820px;margin:0 auto;padding:32px 16px 64px}h1{margin:0 0 4px;font-size:21px}p.sub{margin:0 0 20px;color:#667781;font-size:13px}
.list{background:#fff;border:1px solid #e1e6ea;border-radius:6px;overflow:hidden}
.item{display:flex;align-items:center;gap:12px;padding:13px 16px;border-bottom:1px solid #f0f2f5}
.item:last-child{border-bottom:0}.item:hover{background:#f5f7f8}
.item a.main{color:#111b21;text-decoration:none;font-weight:600;flex:1;min-width:0}
.item a.main span{display:block;font-weight:400;color:#667781;font-size:11.5px;margin-top:2px}
.extras{display:flex;gap:8px;flex:none}.extras a{font-size:11.5px;color:#008069;text-decoration:none;border:1px solid #cfe6e1;border-radius:6px;padding:3px 7px}
.extras a:hover{background:#e7f5f2}.empty{padding:20px 16px;color:#667781}
@media print{body{background:#fff}.extras{display:none}}`)
	b.WriteString("</style></head><body><div class=\"wrap\">")
	b.WriteString("<h1>Logs de pruebas</h1><p class=\"sub\">Conversaciones guardadas del bot de pruebas. Abre la página de cliente para enseñarla o guardarla en PDF.</p>")
	b.WriteString("<div class=\"list\">")
	if len(items) == 0 {
		b.WriteString("<div class=\"empty\">Todavía no hay reportes. Ejecuta una prueba con <code>make tester</code>.</div>")
	}
	for _, it := range items {
		href := it.Name
		if it.ClientName != "" {
			href = it.ClientName
		}
		b.WriteString("<div class=\"item\"><a class=\"main\" href=\"" + html.EscapeString(href) + "\">" +
			html.EscapeString(it.Title) + "<span>" + it.Mod.Format("2006-01-02 15:04") + " · " + html.EscapeString(it.Name) + "</span></a>")
		b.WriteString("<div class=\"extras\">")
		if it.ClientName != "" {
			b.WriteString("<a href=\"" + html.EscapeString(it.Name) + "\">Técnico</a>")
		}
		if it.MD {
			b.WriteString("<a href=\"" + html.EscapeString(strings.TrimSuffix(it.Name, ".html")+".md") + "\">MD</a>")
		}
		if it.JSON {
			b.WriteString("<a href=\"" + html.EscapeString(strings.TrimSuffix(it.Name, ".html")+".json") + "\">JSON</a>")
		}
		b.WriteString("</div></div>")
	}
	b.WriteString("</div></div></body></html>\n")
	return b.String()
}

// reportTitle lee el <title> del HTML; si no lo encuentra usa el nombre del archivo.
func reportTitle(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(path)
	}
	if len(raw) > 8192 {
		raw = raw[:8192]
	}
	s := string(raw)
	start := strings.Index(s, "<title>")
	end := strings.Index(s, "</title>")
	if start >= 0 && end > start+len("<title>") {
		return strings.TrimSpace(s[start+len("<title>") : end])
	}
	return filepath.Base(path)
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// ServeReports publica el directorio de reportes por HTTP hasta que se cancele
// ctx. Pensado para revisar los logs en el navegador o enseñarlos en una llamada.
// No incluye autenticación: en un VPS conviene escuchar en 127.0.0.1 y abrir un
// túnel SSH.
func ServeReports(ctx context.Context, addr, dir string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("tester: el directorio de reportes %q no existe; ejecuta primero una prueba o indica -report-dir", abs)
	}
	if addr == "" {
		addr = "127.0.0.1:8091"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           ReportsHandler(abs),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	shown := addr
	if strings.HasPrefix(shown, ":") {
		shown = "127.0.0.1" + shown
	}
	fmt.Printf("🌐 Logs de pruebas en http://%s  (Ctrl+C para salir)\n", shown)
	log.Info("sirviendo reportes del tester", "addr", addr, "dir", abs)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
