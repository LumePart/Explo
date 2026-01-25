package logging

import (
	"log/slog"
	"os"
	"runtime"
)


func Init(level string, notifyClient *NotificationClient) {
	baseHandler := slog.NewTextHandler( os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(level), 
	})

	handler := &notifyHandler{
		handler:   baseHandler,
		notify: notifyClient,
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func RuntimeAttr(ctx string) slog.Attr {
		_, file, line, ok := runtime.Caller(1)
        if ok {
			return slog.Group("runtime",
            	slog.String("file", file), 
				slog.Int("line", line),
				slog.String("ctx", ctx),
		)
        } else {
            return slog.String("msg", "failed getting runtime")
        }
}

func getLogLevel(level string) slog.Level {

	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}