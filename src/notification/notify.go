package notification

import (
	"context"
	"explo/src/config"
	"fmt"
	"log/slog"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/matrix"
)

type NotificationClient struct {
	Cfg config.NotifyConfig
}

func sendMatrix(cfg config.MatrixNotif, msg string) {
	srvc, err := matrix.New(cfg.UserID, cfg.RoomID, cfg.HomeServer, cfg.AccessToken)
	if err != nil {
    slog.Error(fmt.Sprintln("failed to create new Matrix notification service: %s", err.Error()))
  }

  notifier := notify.New()
  notifier.UseServices(srvc)

  err = notifier.Send(context.Background(), "", "message")
  if err != nil {
    slog.Error(fmt.Sprintf("failed to send Matrix notification: %s", err.Error()))
  }

  slog.Info("notification sent")
}

func SendNotif(msg, service string) {
	switch service {
		case "matrix":
			sendMatrix()
	}
}
