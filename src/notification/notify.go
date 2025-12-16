package notification

import (
	"context"
	"explo/src/config"
	"fmt"
	"log/slog"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/matrix"
	"maunium.net/go/mautrix/id"
)

type NotificationClient struct {
	Cfg config.NotifyConfig
}

func sendMatrix(cfg config.MatrixNotif, msg string) {
	// UserID and RoomID need to be cast as specific types
	srvc, err := matrix.New(id.UserID(cfg.UserID), id.RoomID(cfg.RoomID), cfg.HomeServer, cfg.AccessToken)
	if err != nil {
    slog.Error(fmt.Sprintf("failed to create new Matrix notification service: %s", err.Error()))
  }

  notifier := notify.New()
  notifier.UseServices(srvc)

  err = notifier.Send(context.Background(), "Explo", msg)
  if err != nil {
    slog.Error(fmt.Sprintf("failed to send Matrix notification: %s", err.Error()))
  }

  slog.Info("notification sent")
}

func (c NotificationClient) SendNotification(msg string) {
	switch c.Cfg.Service {
		case "matrix":
			sendMatrix(c.Cfg.Matrix, msg)
	}
}