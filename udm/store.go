package udm

import (
	"database/sql"
	"log/slog"
	"uuid"

	_ "modernc.org/sqlite"
)

type Store interface {
	GetJobs() ([]Job, error)
	GetJob(uuid uuid.UUID) error
	AddJob(j *Job) error
	UpdateJob(j *Job) error
	RemoveJob(j *Job) error
}

func NewSqliteStore(cfg *Config) (Store, error) {
	rv := StoreSqlite{}
	rv.db, rv.Err = sql.Open("sqlite", cfg.DbPath)
	if rv.Err != nil {
		return &rv, rv.Err
	}

	for i := range migv1 {
		_, rv.Err = rv.db.Exec(migv1[i])
		if rv.Err != nil {
			return &rv, rv.Err
		}
	}

	return &rv, nil
}

type StoreSqlite struct {
	db  *sql.DB
	Err error
}

func (s *StoreSqlite) GetJobs() ([]Job, error) {
	return []Job{}, nil
}

func (s *StoreSqlite) GetJob(uuid uuid.UUID) error {
	return nil

}

func (s StoreSqlite) AddJob(j *Job) error {

	_, err := s.db.Exec(`INSERT INTO
		jobs (uuid, out, dir, uri, max_download_limit)
		VALUES (?, ?, ?, ?, ?)`,
		j.Uuid.String(), j.Out, j.Dir, j.Uri, j.MaxDownloadLimit)
	if err != nil {
		return err
	}

	return nil
}

func (s *StoreSqlite) UpdateJob(j *Job) error {
	_, err := s.db.Exec(`UPDATE jobs SET
		max_download_limit = ?,
		uri = ?,
		dir = ?,
		out = ?,
		WHERE uuid = ?`,
		j.MaxDownloadLimit,
		j.Uri,
		j.Dir,
		j.Out,
		j.Uuid.String())
	if err != nil {
		slog.Error("Failed to set max_downlaod_limit", "err", err)
		return err
	}
	return nil
}

func (s *StoreSqlite) RemoveJob(j *Job) error {
	return nil
}

var migv1 = []string{
	`CREATE TABLE IF NOT EXISTS jobs (
		uuid TEXT PRIMARY KEY,
		out TEXT,
		dir TEXT,
		uri TEXT,
		max_download_limit TEXT
	);
	
	`,
}
