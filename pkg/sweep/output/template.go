package output

import (
	"bytes"
	"sync"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
)

// TemplateFormatter formats output using a custom Go text/template.
// It supports custom template functions for common formatting operations.
type TemplateFormatter struct {
	templateStr string
	template    *template.Template
	mu          sync.Mutex
}

// templateData is the data passed to the template.
// It wraps Result to add computed fields.
type templateData struct {
	*Result
	TotalSize int64
}

// NewTemplateFormatter creates a new template formatter with the given template string.
func NewTemplateFormatter(templateStr string) *TemplateFormatter {
	return &TemplateFormatter{
		templateStr: templateStr,
	}
}

// SetTemplate sets or updates the template string.
func (f *TemplateFormatter) SetTemplate(templateStr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.templateStr = templateStr
	f.template = nil // Reset compiled template
}

// templateFuncs returns the custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// date formats a time.Time using the provided layout.
		// Usage: {{date .ModTime "2006-01-02"}}
		"date": func(t time.Time, layout string) string {
			if t.IsZero() {
				return ""
			}
			return t.Format(layout)
		},

		// bytes formats a size in bytes as a human-readable string.
		// Usage: {{bytes .Size}}
		"bytes": func(size int64) string {
			return humanize.IBytes(uint64(size))
		},
	}
}

// Format writes the formatted output to the buffer.
func (f *TemplateFormatter) Format(w *bytes.Buffer, r *Result) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Compile template if needed
	if f.template == nil {
		tmpl, err := template.New("output").Funcs(templateFuncs()).Parse(f.templateStr)
		if err != nil {
			return err
		}
		f.template = tmpl
	}

	// Prepare data with computed fields
	data := templateData{
		Result:    r,
		TotalSize: r.TotalSize(),
	}

	return f.template.Execute(w, data)
}

// defaultTemplate is the template used when no custom template is provided.
const defaultTemplate = `{{range .Files}}{{.SizeHuman}}	{{.Path}}
{{end}}`

func init() {
	Register("template", func() Formatter {
		return NewTemplateFormatter(defaultTemplate)
	})
}

// Ensure TemplateFormatter implements Formatter.
var _ Formatter = (*TemplateFormatter)(nil)
