package output

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// YAMLFormatter formats output as YAML.
// It produces the same structure as JSONFormatter but in YAML format.
type YAMLFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *YAMLFormatter) Format(w *bytes.Buffer, r *Result) error {
	output := BuildStructuredOutput(r)

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(output); err != nil {
		return err
	}
	return encoder.Close()
}

func init() {
	Register("yaml", func() Formatter {
		return &YAMLFormatter{}
	})
}

// Ensure YAMLFormatter implements Formatter.
var _ Formatter = (*YAMLFormatter)(nil)
