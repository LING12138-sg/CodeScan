package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"codescan/internal/config"
)

var DB *gorm.DB

func InitDB(cfg *config.DBConfig) error {
	var err error
	DB, err = OpenMySQL(cfg, true)
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}

	// Connection pool settings
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("error getting underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Auto migrate schemas
	if _, err := EnsureSchema(DB); err != nil {
		return err
	}

	return nil
}
