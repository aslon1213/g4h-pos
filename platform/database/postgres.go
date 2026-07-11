package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/configs"

	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// buildDSN returns the PostgreSQL connection string. When config.DB.DSN is set
// (e.g. the Supabase connection-pooler URI) it is used verbatim; otherwise a DSN
// is assembled from the discrete host/port/credentials fields.
func buildDSN(cfg *configs.Config) string {
	if strings.TrimSpace(cfg.DB.DSN) != "" {
		return cfg.DB.DSN
	}
	sslMode := cfg.DB.SSLMode
	if sslMode == "" {
		sslMode = "require" // Supabase requires TLS by default
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.Username, cfg.DB.Password, cfg.DB.Database, sslMode,
	)
}

// NewGormDB opens a GORM connection to PostgreSQL (Supabase) and returns the
// *gorm.DB handle that is fanned out to the ported repositories. It is the
// PostgreSQL counterpart of NewDB (Mongo). It is fatal on failure — use it when
// a Postgres connection is required (migrator, ported-domain tests).
//
// Transactions use GORM's native db.Transaction(func(tx *gorm.DB) error {...}).
// Cross-domain money primitives take the shared *gorm.DB tx so their writes
// enlist in one transaction (replacing the Mongo session-context threading).
func NewGormDB() *gorm.DB {
	cfg, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	db, err := connectGorm(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	return db
}

// MaybeNewGormDB connects to PostgreSQL only when a DSN is configured
// (database.dsn / DATABASE_DSN). During the Mongo→Postgres migration this lets
// app.New() keep working before Postgres is provisioned: it returns nil (with a
// warning) when unconfigured, so unported (Mongo) domains still start. Once a
// DSN is set, a real connection failure is fatal.
func MaybeNewGormDB() *gorm.DB {
	cfg, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	if strings.TrimSpace(cfg.DB.DSN) == "" {
		log.Warn().Msg("PostgreSQL DSN not configured (database.dsn / DATABASE_DSN) — Postgres disabled; ported domains will not work until it is set")
		return nil
	}
	db, err := connectGorm(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	return db
}

func connectGorm(cfg *configs.Config) (*gorm.DB, error) {
	log.Debug().Msg("Connecting to PostgreSQL") // DSN carries credentials — do not log it

	db, err := gorm.Open(postgres.Open(buildDSN(cfg)), &gorm.Config{
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true, // constraints come from SQL migrations, not AutoMigrate
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.DB.MaxConnections > 0 {
		sqlDB.SetMaxOpenConns(int(cfg.DB.MaxConnections))
	}
	if cfg.DB.MinPoolSize > 0 {
		sqlDB.SetMaxIdleConns(int(cfg.DB.MinPoolSize))
	}
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	log.Info().Msg("Successfully connected to PostgreSQL")
	return db, nil
}
