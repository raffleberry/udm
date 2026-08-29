package udm

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func NewDb(cfg *Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite", cfg.DbPath)
	if err != nil {
		return nil, err
	}

	for i := range migv1 {

	}

	return db, nil
}

type SJob struct {
}

func GetJobs(db *sql.DB) error {
	return nil
}

func GetJob(db *sql.DB) error {
	return nil

}

func AddJob(db *sql.DB) error {
	return nil
}

var migv1 = []string{}
