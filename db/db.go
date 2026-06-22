package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	Hub    *sql.DB
	Mirror *sql.DB
}

func New(hubDSN, mirrorDSN string) (*Store, error) {
	hub, err := sql.Open("mysql", hubDSN)
	if err != nil {
		return nil, fmt.Errorf("hub db open: %w", err)
	}
	if err := hub.Ping(); err != nil {
		return nil, fmt.Errorf("hub db ping: %w", err)
	}
	hub.SetMaxOpenConns(25)
	hub.SetMaxIdleConns(10)
	log.Println("[db] otapi_hub connected")

	mirror, err := sql.Open("mysql", mirrorDSN)
	if err != nil {
		return nil, fmt.Errorf("mirror db open: %w", err)
	}
	if err := mirror.Ping(); err != nil {
		return nil, fmt.Errorf("mirror db ping: %w", err)
	}
	mirror.SetMaxOpenConns(10)
	mirror.SetMaxIdleConns(5)
	log.Println("[db] mirror DB connected")

	s := &Store{Hub: hub, Mirror: mirror}
	if err := s.autoMigrate(); err != nil {
		log.Printf("[db] autoMigrate warning: %v", err)
	}
	return s, nil
}

func (s *Store) autoMigrate() error {
	_, err := s.Hub.Exec(`CREATE TABLE IF NOT EXISTS attr_translations (
		pid              VARCHAR(128)  NOT NULL,
		vid              VARCHAR(128)  NOT NULL,
		property_name_zh VARCHAR(512)  NOT NULL DEFAULT '',
		value_zh         VARCHAR(1024) NOT NULL DEFAULT '',
		property_name_ru VARCHAR(512)  NOT NULL DEFAULT '',
		value_ru         VARCHAR(1024) NOT NULL DEFAULT '',
		translated_at    BIGINT        DEFAULT NULL,
		PRIMARY KEY (pid, vid)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *Store) Close() {
	if s.Hub != nil {
		s.Hub.Close()
	}
	if s.Mirror != nil {
		s.Mirror.Close()
	}
}
