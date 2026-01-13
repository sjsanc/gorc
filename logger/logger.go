package logger

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.SugaredLogger

func init() {
	config := zap.NewDevelopmentConfig()
	config.DisableCaller = true
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = dimTimeEncoder
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.ConsoleSeparator = " "

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}
	Log = logger.Sugar()
}

func dimTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("\033[2m%s\033[0m", t.Format("15:04:05")))
}
