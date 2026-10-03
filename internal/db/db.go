package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	once     sync.Once
	instance *sql.DB
)

func Open(dataDir string) (*sql.DB, error) {
	var err error
	once.Do(func() {
		dbPath := filepath.Join(dataDir, "agent-hub.db")
		instance, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
		if err != nil {
			return
		}
		instance.SetMaxOpenConns(1)

		if err = Migrate(instance); err != nil {
			instance.Close()
			instance = nil
		}
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return instance, nil
}
