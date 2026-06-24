package debug

import (
	"context"
	"log/slog"
	"strings"
)

type slogHandler struct {
	attrs []slog.Attr
}

func (h *slogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *slogHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
	}
	r.Attrs(func(a slog.Attr) bool {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
		return true
	})
	Printf("[remarkable-go] %s\n", b.String())
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &slogHandler{attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *slogHandler) WithGroup(_ string) slog.Handler {
	return h
}

// SlogLogger returns a slog.Logger that forwards records to the debug file
// logger, so library logs land in remanager.log and support bundles.
func SlogLogger() *slog.Logger {
	return slog.New(&slogHandler{})
}
