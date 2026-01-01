package notification

import (
	"context"
	"explo/src/config"
	"fmt"
	"log/slog"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/discord"
	"github.com/nikoksr/notify/service/matrix"
	"github.com/nikoksr/notify/service/http"
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
    return fmt.Errorf("failed to send Matrix notification: %s", err.Error())
  }

  return nil
}

func sendDiscord(cfg config.DiscordNotif, msg string) error {
	srvc := discord.New()

	if err := srvc.AuthenticateWithBotToken(cfg.BotToken); err != nil {
		return fmt.Errorf("failed to autenticate against Discord: %s", err.Error())
	}

	srvc.AddReceivers(cfg.ChannelIDs...)

	notifier := notify.New()
	notifier.UseServices(srvc)

	if err := notifier.Send(context.Background(), "Explo", msg); err != nil {
		return fmt.Errorf("failed to send Discord notification: %s", err.Error())
	}

	return nil	
}

func sendHttp(cfg config.HttpNotif, msg string) error {
	httpNotify := http.New()

	httpNotify.AddReceiversURLs(cfg.ReceiverURLs...)
	notifier := notify.NewWithServices(httpNotify)

	if err := notifier.Send(context.Background(), "Explo", msg); err != nil {
		return fmt.Errorf("failed to send HTTP notification: %s", err.Error())
	}
	return nil
}

func (c NotificationClient) SendNotification(msg string) {
	var err error
	switch c.Cfg.Service {
		case "matrix":
			err = sendMatrix(c.Cfg.Matrix, msg)
		
		case "discord":
			err = sendDiscord(c.Cfg.Discord, msg)
		
		case "": // no system defined
			return
		default:
			err = fmt.Errorf("wrong system defined for notifications: %s", c.Cfg.Service)
	}
	if err != nil {
		slog.Error(err.Error())
	} else {
		slog.Info("notification sent")
	}
}