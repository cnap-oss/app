package storage

import (
	"os"
	"strconv"
	"time"

	"github.com/cnap-oss/app/internal/common"
	gormlogger "gorm.io/gorm/logger"
)

// Config는 GORM 데이터베이스 설정 값을 보관합니다.
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
// DATABASE_URL이 설정되지 않은 경우, 로컬 개발을 위해 SQLite를 기본으로 사용합니다.
func ConfigFromEnv() (Config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// DATABASE_URL이 없으면 SQLite 기본값 사용 (로컬 개발용)
		dsn = getDefaultSQLiteDSN()
	}

	cfg := Config{
		DSN:             dsn,
		LogLevel:        parseLogLevel(os.Getenv("DB_LOG_LEVEL")),
		MaxIdleConns:    parseIntWithDefault(os.Getenv("DB_MAX_IDLE"), 5),
		MaxOpenConns:    parseIntWithDefault(os.Getenv("DB_MAX_OPEN"), 20),
		ConnMaxLifetime: parseDurationWithDefault(os.Getenv("DB_CONN_LIFETIME"), 30*time.Minute),
		SkipDefaultTxn:  parseBoolWithDefault(os.Getenv("DB_SKIP_DEFAULT_TXN"), true),
		PrepareStmt:     parseBoolWithDefault(os.Getenv("DB_PREPARE_STMT"), false),
	}

	if v, ok := lookupEnvBool("DB_DISABLE_AUTO_PING"); ok {
		cfg.DisableAutomaticPing = v
	}

	return cfg, nil
}

// getDefaultSQLiteDSN은 로컬 개발용 기본 SQLite DSN을 반환합니다.
func getDefaultSQLiteDSN() string {
	// common.GetDatabasePath()는 SQLITE_DATABASE 환경변수와 기본 경로를 처리합니다
	return common.GetDatabasePath()
}

func parseLogLevel(value string) gormlogger.LogLevel {
	switch value {
	case "silent", "SILENT":
		return gormlogger.Silent
	case "error", "ERROR":
		return gormlogger.Error
	case "warn", "WARN":
		return gormlogger.Warn
	case "info", "INFO":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}

func parseIntWithDefault(value string, def int) int {
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return parsed
}

func parseDurationWithDefault(value string, def time.Duration) time.Duration {
	if value == "" {
		return def
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return def
	}
	return d
}

func parseBoolWithDefault(value string, def bool) bool {
	if value == "" {
		return def
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return def
	}
	return parsed
}

func lookupEnvBool(key string) (bool, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}
