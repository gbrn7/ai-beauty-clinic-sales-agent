package db

import (
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

func InitDB(databaseURL string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		dialector = postgres.Open(databaseURL)
	} else {
		sqlitePath := strings.TrimPrefix(databaseURL, "sqlite://")
		if sqlitePath == "" {
			sqlitePath = "clinic.db"
		}
		dialector = sqlite.Open(sqlitePath)
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		sqlDB, err := gormDB.DB()
		if err == nil {
			_, _ = sqlDB.Exec("PRAGMA journal_mode=WAL;")
			_, _ = sqlDB.Exec("PRAGMA busy_timeout=5000;")
		}
	}

	err = gormDB.AutoMigrate(
		&model.Treatment{},
		&model.Booking{},
		&model.ClinicHours{},
		&model.DoctorSchedule{},
		&model.SpecialSchedule{},
		&model.ConversationMessage{},
		&model.Contact{},
	)
	if err != nil {
		return nil, err
	}

	return gormDB, nil
}
