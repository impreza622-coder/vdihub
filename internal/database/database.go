package database

import (
	"fmt"
	"log"

	"shoukehuduan/internal/config"
	"shoukehuduan/pkg/types"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := cfg.DSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库实例失败: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("数据库连接成功")

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&types.User{},
		&types.UserVM{},
		&types.Session{},
	)
}

func InitDBConnection(cfg *config.DatabaseConfig) error {
	var err error
	DB, err = InitDB(cfg)
	if err != nil {
		return err
	}

	if err := AutoMigrate(DB); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	return nil
}
