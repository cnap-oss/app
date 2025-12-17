package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	gormlogger "gorm.io/gorm/logger"
)

// Config는 애플리케이션의 모든 설정을 관리합니다.
type Config struct {
	App       AppConfig       `yaml:"app"`
	Database  DatabaseConfig  `yaml:"database"`
	Discord   DiscordConfig   `yaml:"discord"`
	APIKeys   APIKeysConfig   `yaml:"api_keys"`
	Runner    RunnerConfig    `yaml:"runner"`
	Directory DirectoryConfig `yaml:"directory"`
}

// AppConfig는 애플리케이션 기본 설정입니다.
type AppConfig struct {
	// ENV는 실행 환경입니다 (development, production)
	ENV string `yaml:"env"`
	// LogLevel은 애플리케이션 로그 레벨입니다 (debug, info, warn, error)
	LogLevel string `yaml:"log_level"`
}

// DatabaseConfig는 데이터베이스 설정입니다.
type DatabaseConfig struct {
	// DSN은 데이터베이스 연결 문자열입니다
	DSN string `yaml:"dsn"`
	// LogLevel은 GORM 로그 레벨입니다
	LogLevel gormlogger.LogLevel `yaml:"log_level"`
	// MaxIdleConns는 연결 풀의 idle 연결 개수입니다
	MaxIdleConns int `yaml:"max_idle_conns"`
	// MaxOpenConns는 연결 풀의 최대 연결 개수입니다
	MaxOpenConns int `yaml:"max_open_conns"`
	// ConnMaxLifetime은 연결의 최대 수명입니다
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	// SkipDefaultTxn은 기본 트랜잭션을 스킵할지 여부입니다
	SkipDefaultTxn bool `yaml:"skip_default_txn"`
	// PrepareStmt는 prepared statement 캐시를 사용할지 여부입니다
	PrepareStmt bool `yaml:"prepare_stmt"`
	// DisableAutomaticPing은 자동 ping을 비활성화할지 여부입니다
	DisableAutomaticPing bool `yaml:"disable_automatic_ping"`
}

// DiscordConfig는 Discord 봇 설정입니다.
type DiscordConfig struct {
	// Token은 Discord 봇 토큰입니다
	Token string `yaml:"token"`
}

// APIKeysConfig는 외부 API 키 설정입니다.
type APIKeysConfig struct {
	// OpenCode는 OpenCode API 키입니다
	OpenCode string `yaml:"opencode"`
	// Anthropic은 Anthropic API 키입니다
	Anthropic string `yaml:"anthropic"`
	// OpenAI는 OpenAI API 키입니다
	OpenAI string `yaml:"openai"`
}

// RunnerConfig는 Runner 실행 환경 설정입니다.
type RunnerConfig struct {
	// Image는 Docker 이미지 이름입니다
	Image string `yaml:"image"`
	// WorkspaceDir은 워크스페이스 기본 디렉토리입니다
	WorkspaceDir string `yaml:"workspace_dir"`
}

// DirectoryConfig는 디렉토리 경로 설정입니다.
type DirectoryConfig struct {
	// CNAPDir은 기본 데이터 디렉토리입니다 (환경 변수 CNAP_DIR로만 설정 가능, 기본값: $HOME/.cnap)
	CNAPDir string `yaml:"-"`
	// WorkspaceBaseDir은 워크스페이스 기본 디렉토리입니다
	WorkspaceBaseDir string `yaml:"workspace_base_dir"`
	// SQLiteDatabase는 SQLite 데이터베이스 파일 경로입니다
	SQLiteDatabase string `yaml:"sqlite_database"`
}

var (
	instance *Config
	once     sync.Once
	mu       sync.RWMutex
)

// InitConfig는 설정을 초기화합니다.
// configPath가 비어있으면 ${CNAP_DIR}/config.yaml에서 로드를 시도하고, 파일이 없으면 환경 변수에서 로드합니다.
// 파일에서 로드한 후 환경 변수로 오버라이드됩니다.
func InitConfig(configPath string) error {
	var err error
	once.Do(func() {
		if configPath == "" {
			// CNAP_DIR 기본 경로 사용
			configPath = filepath.Join(getCNAPDir(), "config.yaml")
		}

		// 파일이 존재하면 파일에서 로드, 없으면 환경 변수에서 로드
		if _, statErr := os.Stat(configPath); statErr == nil {
			instance, err = LoadConfigFromFile(configPath)
		} else {
			instance, err = LoadConfigFromEnv()
		}
	})
	return err
}

// GetConfig는 싱글톤 Config 인스턴스를 반환합니다.
// InitConfig가 먼저 호출되어야 합니다.
func GetConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		// InitConfig가 호출되지 않은 경우 환경 변수에서 로드 시도
		_ = InitConfig("")
	}
	return instance
}

// LoadConfig는 YAML 파일에서 설정을 로드합니다.
// 레거시 호환성을 위해 유지하되, GetConfig 사용을 권장합니다.
func LoadConfig() (*Config, error) {
	return GetConfig(), nil
}

// LoadConfigFromFile은 YAML 파일에서 설정을 로드합니다.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 파일 읽기 실패: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("설정 파일 파싱 실패: %w", err)
	}

	// YAML에서 로드한 후 환경 변수로 오버라이드
	cfg = mergeWithEnv(cfg)

	return cfg, nil
}

// LoadConfigFromEnv는 환경 변수에서 설정을 로드합니다.
func LoadConfigFromEnv() (*Config, error) {
	cfg := &Config{
		App:       loadAppConfig(),
		Database:  loadDatabaseConfig(),
		Discord:   loadDiscordConfig(),
		APIKeys:   loadAPIKeysConfig(),
		Runner:    loadRunnerConfig(),
		Directory: loadDirectoryConfig(),
	}

	return cfg, nil
}

// mergeWithEnv는 YAML 설정을 환경 변수로 오버라이드합니다.
func mergeWithEnv(cfg *Config) *Config {
	// App
	if env := os.Getenv("CNAP_ENV"); env != "" {
		cfg.App.ENV = env
	}
	if logLevel := os.Getenv("CNAP_LOG_LEVEL"); logLevel != "" {
		cfg.App.LogLevel = logLevel
	}

	// Database
	if dsn := os.Getenv("CNAP_DB_DSN"); dsn != "" {
		cfg.Database.DSN = dsn
	}
	if logLevel := os.Getenv("CNAP_DB_LOG_LEVEL"); logLevel != "" {
		cfg.Database.LogLevel = parseLogLevel(logLevel)
	}
	if maxIdle := os.Getenv("CNAP_DB_MAX_IDLE"); maxIdle != "" {
		cfg.Database.MaxIdleConns = parseIntWithDefault(maxIdle, cfg.Database.MaxIdleConns)
	}
	if maxOpen := os.Getenv("CNAP_DB_MAX_OPEN"); maxOpen != "" {
		cfg.Database.MaxOpenConns = parseIntWithDefault(maxOpen, cfg.Database.MaxOpenConns)
	}
	if lifetime := os.Getenv("CNAP_DB_CONN_LIFETIME"); lifetime != "" {
		cfg.Database.ConnMaxLifetime = parseDurationWithDefault(lifetime, cfg.Database.ConnMaxLifetime)
	}
	if skipTxn := os.Getenv("CNAP_DB_SKIP_DEFAULT_TXN"); skipTxn != "" {
		cfg.Database.SkipDefaultTxn = parseBoolWithDefault(skipTxn, cfg.Database.SkipDefaultTxn)
	}
	if prepStmt := os.Getenv("CNAP_DB_PREPARE_STMT"); prepStmt != "" {
		cfg.Database.PrepareStmt = parseBoolWithDefault(prepStmt, cfg.Database.PrepareStmt)
	}

	// Discord
	if token := os.Getenv("CNAP_DISCORD_TOKEN"); token != "" {
		cfg.Discord.Token = token
	}

	// API Keys
	if apiKey := os.Getenv("CNAP_OPENCODE_API_KEY"); apiKey != "" {
		cfg.APIKeys.OpenCode = apiKey
	}
	if apiKey := os.Getenv("CNAP_ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.APIKeys.Anthropic = apiKey
	}
	if apiKey := os.Getenv("CNAP_OPENAI_API_KEY"); apiKey != "" {
		cfg.APIKeys.OpenAI = apiKey
	}

	// Runner
	if image := os.Getenv("CNAP_RUNNER_IMAGE"); image != "" {
		cfg.Runner.Image = image
	}
	if workspaceDir := os.Getenv("CNAP_RUNNER_WORKSPACE_DIR"); workspaceDir != "" {
		cfg.Runner.WorkspaceDir = workspaceDir
	}

	// Directory
	if cnapDir := os.Getenv("CNAP_DIR"); cnapDir != "" {
		cfg.Directory.CNAPDir = cnapDir
	}
	if workspaceBaseDir := os.Getenv("CNAP_WORKSPACE_BASE_DIR"); workspaceBaseDir != "" {
		cfg.Directory.WorkspaceBaseDir = workspaceBaseDir
	}
	if sqliteDB := os.Getenv("CNAP_SQLITE_DATABASE"); sqliteDB != "" {
		cfg.Directory.SQLiteDatabase = sqliteDB
	}

	return cfg
}

func loadAppConfig() AppConfig {
	return AppConfig{
		ENV:      getEnvOrDefault("CNAP_ENV", "production"),
		LogLevel: getEnvOrDefault("CNAP_LOG_LEVEL", "info"),
	}
}

func loadDatabaseConfig() DatabaseConfig {
	dsn := os.Getenv("CNAP_DATABASE_URL")
	if dsn == "" {
		// CNAP_DATABASE_URL이 없으면 SQLite 기본값 사용 (로컬 개발용)
		// GetDatabasePath() 호출 대신 직접 계산 (순환 참조 방지)
		sqliteDB := os.Getenv("CNAP_SQLITE_DATABASE")
		if sqliteDB == "" {
			cnapDir := getCNAPDir()
			sqliteDB = filepath.Join(cnapDir, "cnap.db")
		}
		dsn = sqliteDB
	}

	cfg := DatabaseConfig{
		DSN:             dsn,
		LogLevel:        parseLogLevel(os.Getenv("CNAP_DB_LOG_LEVEL")),
		MaxIdleConns:    parseIntWithDefault(os.Getenv("CNAP_DB_MAX_IDLE"), 5),
		MaxOpenConns:    parseIntWithDefault(os.Getenv("CNAP_DB_MAX_OPEN"), 20),
		ConnMaxLifetime: parseDurationWithDefault(os.Getenv("CNAP_DB_CONN_LIFETIME"), 30*time.Minute),
		SkipDefaultTxn:  parseBoolWithDefault(os.Getenv("CNAP_DB_SKIP_DEFAULT_TXN"), true),
		PrepareStmt:     parseBoolWithDefault(os.Getenv("CNAP_DB_PREPARE_STMT"), false),
	}

	if v, ok := lookupEnvBool("CNAP_DB_DISABLE_AUTO_PING"); ok {
		cfg.DisableAutomaticPing = v
	}

	return cfg
}

func loadDiscordConfig() DiscordConfig {
	return DiscordConfig{
		Token: os.Getenv("CNAP_DISCORD_TOKEN"),
	}
}

func loadAPIKeysConfig() APIKeysConfig {
	return APIKeysConfig{
		OpenCode:  os.Getenv("CNAP_OPENCODE_API_KEY"),
		Anthropic: os.Getenv("CNAP_ANTHROPIC_API_KEY"),
		OpenAI:    os.Getenv("CNAP_OPENAI_API_KEY"),
	}
}

func loadRunnerConfig() RunnerConfig {
	cfg := RunnerConfig{
		Image:        os.Getenv("CNAP_RUNNER_IMAGE"),
		WorkspaceDir: os.Getenv("CNAP_RUNNER_WORKSPACE_DIR"),
	}

	// CNAP_RUNNER_IMAGE가 설정되지 않은 경우 CNAP_ENV에 따라 기본값 설정
	if cfg.Image == "" {
		env := getEnvOrDefault("CNAP_ENV", "production")
		if env == "development" {
			cfg.Image = "cnap-runner:latest"
		} else {
			cfg.Image = "ghcr.io/cnap-oss/cnap-runner:latest"
		}
	}

	return cfg
}

func loadDirectoryConfig() DirectoryConfig {
	return DirectoryConfig{
		CNAPDir:          getCNAPDir(),
		WorkspaceBaseDir: os.Getenv("CNAP_WORKSPACE_BASE_DIR"),
		SQLiteDatabase:   os.Getenv("CNAP_SQLITE_DATABASE"),
	}
}

// getCNAPDir은 CNAP_DIR 환경 변수를 반환하거나 기본값을 계산합니다.
func getCNAPDir() string {
	cnapDir := os.Getenv("CNAP_DIR")
	if cnapDir != "" {
		return cnapDir
	}

	// CNAP_DIR이 없으면 $HOME/.cnap 사용
	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return filepath.Join(homeDir, ".cnap")
	}

	// Fallback: ./data
	return "./data"
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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

// Validate는 필수 설정 값들을 검증합니다.
func (c *Config) Validate() error {
	if c.Discord.Token == "" {
		return fmt.Errorf("CNAP_DISCORD_TOKEN is required")
	}
	if c.APIKeys.OpenCode == "" {
		return fmt.Errorf("CNAP_OPENCODE_API_KEY is required")
	}
	return nil
}

// GetAPIKeyEnvVars는 Runner에 전달할 API 키 환경 변수 목록을 반환합니다.
func (c *Config) GetAPIKeyEnvVars() []string {
	var env []string
	if c.APIKeys.OpenCode != "" {
		env = append(env, fmt.Sprintf("CNAP_OPENCODE_API_KEY=%s", c.APIKeys.OpenCode))
	}
	if c.APIKeys.Anthropic != "" {
		env = append(env, fmt.Sprintf("CNAP_ANTHROPIC_API_KEY=%s", c.APIKeys.Anthropic))
	}
	if c.APIKeys.OpenAI != "" {
		env = append(env, fmt.Sprintf("CNAP_OPENAI_API_KEY=%s", c.APIKeys.OpenAI))
	}
	return env
}
