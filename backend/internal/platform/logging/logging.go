// Package logging builds the structured logger used by every service and
// enforces the LGPD rule that personal data never reaches the log stream.
package logging

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// Fields whose value is replaced by a placeholder before being logged.
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"passwordhash":  {},
	"token":         {},
	"authorization": {},
	"email":         {},
	"document":      {},
	"cpf":           {},
	"text":          {},
	"body":          {},
}

var (
	cpfPattern   = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	emailPattern = regexp.MustCompile(`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`)
	phonePattern = regexp.MustCompile(`\b\(?\d{2}\)?\s?9?\d{4}-?\d{4}\b`)
)

// Redact masks anything that looks like personal data inside a free-form
// string. It is applied to values before they are logged, so message contents
// and identifiers never leak (LGPD, RNF004).
func Redact(value string) string {
	value = cpfPattern.ReplaceAllString(value, "[CPF]")
	value = emailPattern.ReplaceAllString(value, "[EMAIL]")
	value = phonePattern.ReplaceAllString(value, "[TELEFONE]")
	return value
}

// redactHandler wraps a slog handler and sanitises every attribute.
type redactHandler struct{ inner slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, Redact(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAttr(attr))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		cleaned = append(cleaned, redactAttr(attr))
	}
	return redactHandler{inner: h.inner.WithAttrs(cleaned)}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if _, sensitive := sensitiveKeys[strings.ToLower(attr.Key)]; sensitive {
		return slog.String(attr.Key, "[REDACTED]")
	}
	if attr.Value.Kind() == slog.KindString {
		return slog.String(attr.Key, Redact(attr.Value.String()))
	}
	return attr
}

// New returns the process logger. `service` is attached to every record so a
// single aggregated log stream stays readable across the five services.
func New(service, env string) *slog.Logger {
	level := slog.LevelDebug
	if env == "production" {
		level = slog.LevelInfo
	}
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(redactHandler{inner: base}).With(slog.String("service", service))
}
