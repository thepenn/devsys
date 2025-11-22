package store

import (
	"context"
	"runtime/debug"

	"gorm.io/gorm"
)

type DB struct {
	conn *gorm.DB
}

// Scanner 扫描器接口
type Scanner interface {
	Scan(dest ...interface{}) error
}

// GetDB 获取原始的GORM数据库连接
func (db *DB) GetDB() *gorm.DB {
	return db.conn
}

// View 执行只读操作
func (db *DB) View(fn func(*gorm.DB) error) error {
	return fn(db.conn)
}

// Transaction 执行事务操作
func (db *DB) Transaction(fn func(*gorm.DB) error) (err error) {
	return db.conn.Transaction(func(tx *gorm.DB) error {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				panic(p) // 重新抛出panic，让GORM处理回滚
			}
		}()
		return fn(tx)
	})
}

// WithContext 使用上下文
func (db *DB) WithContext(ctx context.Context) *gorm.DB {
	return db.conn.WithContext(ctx)
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	sqlDB, err := db.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
