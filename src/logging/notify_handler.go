package logging

import (
	"log/slog"
	"context"
	"time"
)
// slog handler that checks whether to send notifications

type notifyHandler struct {
	handler   slog.Handler
	notify NotificationClient
}

type Notification struct {
	Time time.Time `json:"time"`
	Level string `json:"level"`
	Message string `json:"message"`
	Attrs map[string]any `json:"attributes"`

}

func (h *notifyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *notifyHandler) Handle(ctx context.Context, r slog.Record) error {
	if shouldNotify(r) {
		// send notification in another goroutine
		notifyStruct := recordToStruct(r)
		go h.notify.SendNotification(notifyStruct) 
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

func recordToStruct(r slog.Record) Notification {
	attrs := make(map[string]any, r.NumAttrs())

	r.Attrs(func(a slog.Attr) bool {
		// filter out notify control key
		if a.Key != "notify" {
			attrs[a.Key] = a.Value.Any()
		}
		return true
	})

	return Notification{
		Time: r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrs,
	}
}