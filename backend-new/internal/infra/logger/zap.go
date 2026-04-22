// Package logger provides the project-wide zap logger factory.
//
// Two modes:
//   - New(true)  → development: colored console, human-readable, caller info
//   - New(false) → production: JSON to stdout, ISO8601 timestamps, no color
//
// All layers (app / infra / transport) receive a *zap.Logger via dependency
// injection from cmd/server/main.go — they never construct their own.
//
// Package logger 提供项目级 zap logger 工厂。
//
// 两种模式：
//   - New(true)  → 开发模式：彩色控制台、人类可读、带调用位置
//   - New(false) → 生产模式：JSON 输出到 stdout、ISO8601 时间戳、无彩色
//
// 所有层（app / infra / transport）通过 DI 从 cmd/server/main.go
// 接收 *zap.Logger 实例，不应自己构造。
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap logger. dev=true selects the colored console encoder
// suitable for local development; dev=false selects production JSON.
//
// New 构建 zap logger。dev=true 使用适合本地开发的彩色控制台编码器；
// dev=false 使用生产环境的 JSON 格式。
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
