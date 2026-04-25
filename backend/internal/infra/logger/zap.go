// Package logger provides the project-wide zap logger factory.
//
// Package logger 提供项目级 zap logger 工厂。
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap logger. dev=true: colored console (for local dev).
// dev=false: JSON to stdout with ISO8601 timestamps (for prod).
//
// New 构造 zap logger。dev=true：彩色控制台（本地开发）。
// dev=false：JSON 输出到 stdout 带 ISO8601 时间戳（生产）。
func New(dev bool) (*zap.Logger, error) {
	var cfg zap.Config
	if dev {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "time"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}
	return logger, nil
}
