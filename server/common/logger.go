package common

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// Log is the package-wide sugared logger. Initialized on package import.
var Log *zap.SugaredLogger

func init() {
    InitLogger()
}

// InitLogger configures the global logger.
func InitLogger() {
    cfg := zap.NewProductionConfig()
    cfg.Encoding = "console"
    cfg.EncoderConfig.TimeKey = "ts"
    cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
    
    cfg.DisableCaller = true // Disable caller info for cleaner logs, for debbugging, change to false to see origin of log messages

    logger, err := cfg.Build()
    if err != nil {
        // fallback to a development logger if construction fails
        l, _ := zap.NewDevelopment()
        Log = l.Sugar()
        return
    }
    Log = logger.Sugar()
}

// SyncLogger flushes any buffered log entries.
func SyncLogger() error {
    if Log == nil {
        return nil
    }
    return Log.Sync()
}
