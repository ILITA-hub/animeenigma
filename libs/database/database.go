package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds database configuration
type Config struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	Database string `json:"database" yaml:"database"`
	SSLMode  string `json:"ssl_mode" yaml:"ssl_mode"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Host:    "localhost",
		Port:    5432,
		SSLMode: "disable",
	}
}

// DSN returns the connection string
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// DB wraps GORM database connection
type DB struct {
	*gorm.DB
}

// New creates a new database connection with auto-init
func New(cfg Config) (*DB, error) {
	// First ensure the database exists
	if err := ensureDatabaseExists(cfg); err != nil {
		return nil, fmt.Errorf("ensure database: %w", err)
	}

	// Connect to the target database
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &DB{DB: db}, nil
}

// ensureDatabaseExists creates the database if it doesn't exist
func ensureDatabaseExists(cfg Config) error {
	// Connect to postgres default database
	adminDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.SSLMode,
	)

	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}

	sqlDB, _ := adminDB.DB()
	defer sqlDB.Close()

	// Check if database exists
	var exists bool
	adminDB.Raw("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = ?)", cfg.Database).Scan(&exists)

	if !exists {
		// Create database
		if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.Database)).Error; err != nil {
			return fmt.Errorf("create database: %w", err)
		}
	}

	return nil
}

// AutoMigrate runs GORM auto-migration for the given models
// Creates tables that don't exist and adds missing columns to existing tables
func (db *DB) AutoMigrate(models ...interface{}) error {
	migrator := db.DB.Migrator()

	for _, model := range models {
		if !migrator.HasTable(model) {
			if err := migrator.CreateTable(model); err != nil {
				return fmt.Errorf("create table: %w", err)
			}
			continue
		}

		// Add missing columns to existing tables
		stmt := &gorm.Statement{DB: db.DB}
		if err := stmt.Parse(model); err != nil {
			return fmt.Errorf("parse model: %w", err)
		}
		for _, field := range stmt.Schema.Fields {
			if field.DBName == "" {
				continue
			}
			if !migrator.HasColumn(model, field.DBName) {
				if err := migrator.AddColumn(model, field.DBName); err != nil {
					return fmt.Errorf("add column %s: %w", field.DBName, err)
				}
			}
		}
	}
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Health checks database connectivity
func (db *DB) Health() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// BaseModel provides common fields for all models
type BaseModel struct {
	ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}
