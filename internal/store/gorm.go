package store

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(datasource string, maxOpenConnections int, showSql bool) (*DB, error) {
	sqlDB, err := sql.Open("mysql", datasource)
	if err != nil {
		return nil, err
	}

	if err := pingDatabase(sqlDB); err != nil {
		return nil, err
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(maxOpenConnections)
	sqlDB.SetMaxIdleConns(maxOpenConnections / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)

	var logLevel logger.LogLevel
	if showSql {
		logLevel = logger.Info
	} else {
		logLevel = logger.Silent
	}

	gormCfg := &gorm.Config{
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		Logger:                 newGORMLogger(logLevel),
	}

	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), gormCfg)
	if err != nil {
		return nil, err
	}

	return &DB{
		conn: db,
	}, nil
}

func pingDatabase(db *sql.DB) (err error) {
	for i := 0; i < 5; i++ {
		err = db.Ping()
		if err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	log.Error().Err(err).Msgf("ping database")
	return
}
