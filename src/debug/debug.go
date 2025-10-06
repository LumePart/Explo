package debug

import (
	//"log"
	"runtime"
	"log/slog"
)


func Init(level string) {
	slog.SetLogLoggerLevel(getLogLevel(level))
}

func Debug(ctx string) slog.Attr {
		_, file, line, ok := runtime.Caller(1)
        if ok {
			return slog.Group("runtime",
            	slog.String("file", file), 
				slog.Int("line",line), 
				slog.String("msg", ctx),
		)
        } else {
            return slog.String("msg", ctx)
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