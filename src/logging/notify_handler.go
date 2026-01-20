package logging

import (
	"log/slog"
	"context"
)
// slog handler that checks whether to send notifications

type notifyHandler struct {
	handler   slog.Handler
	notify NotificationClient
}

func (h *notifyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *notifyHandler) Handle(ctx context.Context, r slog.Record) error {
	if shouldNotify(r) {
		// send notification in another goroutine
		rec := r
		go h.notify.SendNotification(rec) 
	}
	return h.handler.Handle(ctx, r)
}

func (h *notifyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &notifyHandler{
		handler:   h.handler.WithAttrs(attrs),
		notify: h.notify,
	}
}

func (h *notifyHandler) WithGroup(name string) slog.Handler {
	return &notifyHandler{
		handler:   h.handler.WithGroup(name),
		notify: h.notify,
	}
}

func shouldNotify(r slog.Record) bool {
	notify := false

	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "notify" && a.Value.Kind() == slog.KindBool && a.Value.Bool() {
			notify = true
			return false // stop scanning
		}
		return true
	})

	return notify
}