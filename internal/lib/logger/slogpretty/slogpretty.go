// Package slogpretty implements an [slog.Handler] that renders records in
// a human-friendly, coloured form suitable for local development.
package slogpretty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdLog "log" //nolint:depguard // standard logger used for direct terminal output in a slog.Handler implementation
	"log/slog"

	"github.com/fatih/color"
)

// PrettyHandlerOptions configures [PrettyHandler]. SlogOpts is forwarded to
// the embedded JSON handler (primarily to set the minimum [slog.Level]).
type PrettyHandlerOptions struct {
	SlogOpts *slog.HandlerOptions
}

// PrettyHandler writes log records with ANSI colour and indented JSON for
// the attribute map. It embeds the standard JSON handler to inherit
// WithAttrs/WithGroup behaviour.
type PrettyHandler struct {
	slog.Handler

	l     *stdLog.Logger
	attrs []slog.Attr
}

// NewPrettyHandler builds a PrettyHandler writing to out.
func (opts PrettyHandlerOptions) NewPrettyHandler(
	out io.Writer,
) *PrettyHandler {
	h := &PrettyHandler{
		Handler: slog.NewJSONHandler(out, opts.SlogOpts),
		l:       stdLog.New(out, "", 0),
	}

	return h
}

// Handle renders a single record: coloured level, timestamp, message and a
// JSON dump of the attribute map.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	level := r.Level.String() + ":"

	switch r.Level {
	case slog.LevelDebug:
		level = color.MagentaString(level)
	case slog.LevelInfo:
		level = color.BlueString(level)
	case slog.LevelWarn:
		level = color.YellowString(level)
	case slog.LevelError:
		level = color.RedString(level)
	}

	fields := make(map[string]any, r.NumAttrs())

	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()

		return true
	})

	for _, a := range h.attrs {
		fields[a.Key] = a.Value.Any()
	}

	var (
		b   []byte
		err error
	)

	if len(fields) > 0 {
		b, err = json.MarshalIndent(fields, "", "  ")
		if err != nil {
			return fmt.Errorf("slogpretty: marshal fields: %w", err)
		}
	}

	timeStr := r.Time.Format("[15:05:05.000]")
	msg := color.CyanString(r.Message)

	h.l.Println(
		timeStr,
		level,
		msg,
		color.WhiteString(string(b)),
	)

	return nil
}

// WithAttrs returns a new handler with attrs added to every subsequent
// record.
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler,
		l:       h.l,
		attrs:   attrs,
	}
}

// WithGroup returns a new handler nested inside the named group.
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler.WithGroup(name),
		l:       h.l,
	}
}
