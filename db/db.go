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
	log.Println("[db] wabrum_mv connected")

	return &Store{Hub: hub, Mirror: mirror}, nil
}

func (s *Store) Close() {
	if s.Hub != nil {
		s.Hub.Close()
	}
	if s.Mirror != nil {
		s.Mirror.Close()
	}
}
