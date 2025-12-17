package common

import (
	"go.uber.org/zap"
)

// NewLogger creates a new zap logger with the given name.
// The logger is configured based on the centralized Config.
func NewLogger(name string) (*zap.Logger, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	return NewLoggerWithConfig(name, cfg)
}

// NewLoggerWithConfig creates a new zap logger with the given name and config.
func NewLoggerWithConfig(name string, cfg *Config) (*zap.Logger, error) {
	var config zap.Config
	if cfg.App.ENV == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	// LOG_LEVEL 환경변수가 설정되어 있으면 적용
	if cfg.App.LogLevel != "" {
		level, err := zap.ParseAtomicLevel(cfg.App.LogLevel)
		if err == nil {
			config.Level = level
		}
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	// name이 제공되면 Named logger 반환
	if name != "" {
		return logger.Named(name), nil
	}

	return logger, nil
}

// MustNewLogger creates a new logger and panics if it fails.
// Use this when logger creation failure should be fatal.
func MustNewLogger(name string) *zap.Logger {
	logger, err := NewLogger(name)
	if err != nil {
		panic(err)
	}
	return logger
}
