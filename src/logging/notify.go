package logging

import (
	"context"
	"encoding/json"
	"explo/src/config"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/discord"
	nhttp "github.com/nikoksr/notify/service/http"
	"github.com/nikoksr/notify/service/matrix"
	"maunium.net/go/mautrix/id"
)

// TODO: reuse notifier instead of creating a new one every time. right now it's fine cause Explo sends 1 message per run


type NotificationClient struct {
	Cfg config.NotifyConfig
}

func InitNotify(cfg config.NotifyConfig) NotificationClient {
	return NotificationClient{
		Cfg: cfg,
	}
}

func sendMatrix(cfg config.MatrixNotif, msg string) error {
	// UserID and RoomID need to be cast as specific types
	srvc, err := matrix.New(id.UserID(cfg.UserID), id.RoomID(cfg.RoomID), cfg.HomeServer, cfg.AccessToken)
	if err != nil {
    return fmt.Errorf("failed to create new Matrix notification service: %s", err.Error())
  }

  notifier := notify.New()
  notifier.UseServices(srvc)

  err = notifier.Send(context.Background(), "Explo", msg)
  if err != nil {
    return err
  }

  return nil
}

/* discordgo module (which notify uses) doesn't handle errors correctly
 no errors are given even when authentication fails*/
func sendDiscord(cfg config.DiscordNotif, msg string) error {
	srvc := discord.New()
	if err := srvc.AuthenticateWithBotToken(cfg.BotToken); err != nil {
		return fmt.Errorf("failed to authenticate against Discord: %s", err.Error())
	}

	srvc.AddReceivers(cfg.ChannelIDs...)

	notifier := notify.New()
	notifier.UseServices(srvc)

	if err := notifier.Send(context.Background(), "**Explo**", msg); err != nil {
		return fmt.Errorf("failed to send Discord notification: %s", err.Error())
	}

	return nil	
}

func sendHttp(cfg config.HttpNotif, msg string) error {
	httpNotify := nhttp.New()
	webhooks := getNotifWebhooks(cfg.ReceiverURLs)

	httpNotify.AddReceivers(webhooks...)
	notifier := notify.NewWithServices(httpNotify)

	if err := notifier.Send(context.Background(), "", msg); err != nil {
		return fmt.Errorf("failed to send HTTP notification: %s", err.Error())
	}
	return nil
}

func getNotifWebhooks(urls []string) []*nhttp.Webhook{
	var webhooks []*nhttp.Webhook

	for _, url := range urls {

		webhooks = append(webhooks, &nhttp.Webhook{
			ContentType: "application/json",
			URL: url,
			Method: http.MethodPost,
			BuildPayload: func(subject, message string) (payload any) {
				payl := json.RawMessage(message)
				return payl
			},

		})
	}
	return webhooks
}

// format notification for message client
func formatRecordMsgClient(n Notification) string {
	attrs := make([]string, 0, len(n.Attrs))

	for k, v := range n.Attrs {
		attrs = append(attrs, fmt.Sprintf("%s=%v", k, v))
	}
	return fmt.Sprintf(
		"[%s] %s\n%s",
		n.Level,
		n.Message,
		strings.Join(attrs, ", "),
	)
}

func formatRecordJSON(n Notification) string {
	nJSON, err := json.Marshal(n)
	if err != nil {
		slog.Error("failed to marshal notification", "err", err)
	}
	return string(nJSON)
}


func (c NotificationClient) SendNotification(n Notification) {
	var err error
	switch c.Cfg.Service {
		case "matrix":
			msg := formatRecordMsgClient(n)
			err = sendMatrix(c.Cfg.Matrix, msg)
		
		case "discord":
			msg := formatRecordMsgClient(n)
			err = sendDiscord(c.Cfg.Discord, msg)

		case "http":
			msg := formatRecordJSON(n)
			err = sendHttp(c.Cfg.Http, msg)
		
		case "": // no system defined
			return
		default:
			err = fmt.Errorf("wrong system defined for notifications: %s", c.Cfg.Service)
	}
	if err != nil {
		slog.Error(err.Error())
	} else {
		slog.Info("notification sent", "service", c.Cfg.Service)
	}
}
