package storage

import (
	"time"

	"github.com/cnap-oss/app/internal/common"
	gormlogger "gorm.io/gorm/logger"
)

// Config는 GORM 데이터베이스 설정 값을 보관합니다.
// common.DatabaseConfig를 래핑하여 기존 코드와의 호환성을 유지합니다.
type Config struct {
	DSN                  string
	LogLevel             gormlogger.LogLevel
	MaxIdleConns         int
	MaxOpenConns         int
	ConnMaxLifetime      time.Duration
	SkipDefaultTxn       bool
	PrepareStmt          bool
	DisableAutomaticPing bool
}

// ConfigFromEnv는 환경 변수에서 설정을 읽어 Config를 구성합니다.
// common.LoadConfig()를 사용하여 중앙화된 설정을 로드합니다.
func ConfigFromEnv() (Config, error) {
	appConfig, err := common.LoadConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		DSN:                  appConfig.Database.DSN,
		LogLevel:             appConfig.Database.LogLevel,
		MaxIdleConns:         appConfig.Database.MaxIdleConns,
		MaxOpenConns:         appConfig.Database.MaxOpenConns,
		ConnMaxLifetime:      appConfig.Database.ConnMaxLifetime,
		SkipDefaultTxn:       appConfig.Database.SkipDefaultTxn,
		PrepareStmt:          appConfig.Database.PrepareStmt,
		DisableAutomaticPing: appConfig.Database.DisableAutomaticPing,
	}, nil
}
