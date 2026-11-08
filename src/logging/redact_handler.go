package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"

	"explo/src/config"
)

const redactedValue = "[REDACTED]"

var secretValues atomic.Value

func init() {
	secretValues.Store([]string(nil))
}

type redactingHandler struct {
	handler slog.Handler
}

func newRedactingHandler(handler slog.Handler) slog.Handler {
	return &redactingHandler{handler: handler}
}

// NewTextHandler creates a text handler that omits credential fields and
// redacts configured credential values from messages and attributes.
func NewTextHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return newRedactingHandler(slog.NewTextHandler(w, opts))
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, redactText(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if attr = redactAttr(attr); attr.Key != "" {
			clean.AddAttrs(attr)
		}
		return true
	})
	return h.handler.Handle(ctx, clean)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if attr = redactAttr(attr); attr.Key != "" {
			clean = append(clean, attr)
		}
	}
	return &redactingHandler{handler: h.handler.WithAttrs(clean)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{handler: h.handler.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if isCredentialKey(attr.Key) {
		return slog.Attr{}
	}

	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(redactText(value.String()))
	case slog.KindGroup:
		group := value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			if child = redactAttr(child); child.Key != "" {
				clean = append(clean, child)
			}
		}
		attr.Value = slog.GroupValue(clean...)
	case slog.KindAny:
		plain := fmt.Sprint(value.Any())
		if redacted := redactText(plain); redacted != plain {
			attr.Value = slog.StringValue(redacted)
		}
	}
	return attr
}

func isCredentialKey(key string) bool {
	key = strings.ToLower(key)
	key = strings.NewReplacer("_", "", "-", "", " ", "").Replace(key)
	for _, marker := range []string{"password", "passwd", "apikey", "authorization", "credential", "secret", "token"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func redactText(value string) string {
	for _, secret := range secretValues.Load().([]string) {
		value = strings.ReplaceAll(value, secret, redactedValue)
	}
	return value
}

func configureRedaction(cfg *config.Config) {
	values := []string{
		cfg.ServerCfg.Password,
		cfg.ClientCfg.Creds.APIKey,
		cfg.ClientCfg.Creds.Password,
		cfg.ClientCfg.Creds.Token,
		cfg.ClientCfg.Creds.Salt,
		cfg.ClientCfg.AdminCreds.APIKey,
		cfg.ClientCfg.AdminCreds.Password,
		cfg.DownloadCfg.Youtube.APIKey,
		cfg.DownloadCfg.Slskd.APIKey,
		cfg.DownloadCfg.Lidarr.APIKey,
		cfg.DiscoveryCfg.Listenbrainz.UserToken,
		cfg.NotifyCfg.Matrix.AccessToken,
		cfg.NotifyCfg.Discord.BotToken,
	}
	values = append(values, cfg.NotifyCfg.Http.ReceiverURLs...)
	for key, value := range cfg.ClientCfg.Creds.Headers {
		if isCredentialKey(key) {
			values = append(values, value)
		}
	}

	seen := make(map[string]struct{}, len(values))
	secrets := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	secretValues.Store(secrets)
}
