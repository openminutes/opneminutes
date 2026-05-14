package cmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerContextKey struct{}

func contextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	if logger == nil {
		logger = zap.NewNop()
	}

	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func loggerFromContext(ctx context.Context) (*zap.Logger, bool) {
	logger, ok := ctx.Value(loggerContextKey{}).(*zap.Logger)
	if !ok || logger == nil {
		return nil, false
	}

	return logger, true
}

func loggerFromCommand(cmd *cobra.Command) *zap.Logger {
	logger, ok := loggerFromContext(cmd.Context())
	if !ok {
		return zap.NewNop()
	}

	return logger
}

func newVerboseLogger(writer io.Writer) *zap.Logger {
	if writer == nil {
		writer = io.Discard
	}

	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(writer),
		zapcore.DebugLevel,
	)

	return zap.New(core)
}
