package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"
)

// groupOrAttrs tracks either a WithGroup name or a batch of WithAttrs,
// in the order they were applied, so Handle can rebuild the right nesting.
type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}

// prettyJSONHandler is a slog.Handler that renders each record as an
// indented (human-readable) JSON object instead of JSON-Lines.
type prettyJSONHandler struct {
	level slog.Leveler
	goas  []groupOrAttrs
	mu    *sync.Mutex
	out   io.Writer
}

// newPrettyJSONHandler builds a handler writing pretty-printed JSON to out,
// emitting records at or above minLevel.
func newPrettyJSONHandler(out io.Writer, minLevel slog.Leveler) *prettyJSONHandler {
	return &prettyJSONHandler{level: minLevel, mu: &sync.Mutex{}, out: out}
}

func (h *prettyJSONHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// WithGroup and WithAttrs clone the handler, recording the group/attrs so
// Handle can nest fields correctly without mutating shared state.
func (h *prettyJSONHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}

func (h *prettyJSONHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}

func (h *prettyJSONHandler) withGroupOrAttrs(goa groupOrAttrs) *prettyJSONHandler {
	h2 := *h
	h2.goas = append(append([]groupOrAttrs{}, h.goas...), goa)
	return &h2
}

// Handle renders one record as a single indented JSON object followed by a
// newline, honoring any groups/attrs accumulated via WithGroup/WithAttrs.
func (h *prettyJSONHandler) Handle(_ context.Context, r slog.Record) error {
	fields := map[string]any{
		"time":  r.Time.Format(time.RFC3339),
		"level": r.Level.String(),
		"msg":   r.Message,
	}

	cur := fields
	for _, goa := range h.goas {
		if goa.group != "" {
			next := map[string]any{}
			cur[goa.group] = next
			cur = next
			continue
		}
		for _, a := range goa.attrs {
			addAttr(cur, a)
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		addAttr(cur, a)
		return true
	})

	b, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.out.Write(append(b, '\n'))
	return err
}

// addAttr flattens a as a (possibly nested, for KindGroup) entry in m.
func addAttr(m map[string]any, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		sub := map[string]any{}
		for _, ga := range a.Value.Group() {
			addAttr(sub, ga)
		}
		m[a.Key] = sub
		return
	}
	m[a.Key] = attrValue(a.Value)
}

// attrValue converts a slog.Value to a JSON-friendly representation.
func attrValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.Any()
	default:
		return v.String()
	}
}

// n8nLogger is the only logger that renders pretty (multi-line) JSON; every
// other log site uses the compact, single-line default installed below.
var n8nLogger *slog.Logger

// setupLogging installs a compact single-line JSON handler as slog's
// default, and a separate pretty-JSON n8nLogger for n8n delivery logs.
func setupLogging(out io.Writer, minLevel slog.Leveler) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: minLevel})))
	n8nLogger = slog.New(newPrettyJSONHandler(out, minLevel))
}
