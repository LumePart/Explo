package debug

import (
	"log"
	"runtime"
)

var debugMode bool

func Init(mode bool) {
	debugMode = mode
}

func Debug(ctx string) {
	if debugMode {
		_, file, line, ok := runtime.Caller(1)
        if ok {
            log.Printf("DEBUG: %s:%d %s", file, line, ctx)
        } else {
            log.Printf("DEBUG: %s", ctx)
        }
	}
}