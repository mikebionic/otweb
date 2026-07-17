package db

import (
	"database/sql"
	"time"
)

// ScheduledSync — запланированная синхронизация (запускается по времени).
type ScheduledSync struct {
	ID          int64         `json:"id"`
	CategoryID  string        `json:"category_id"`
	Label       string        `json:"label"`
	ParamsJSON  string        `json:"-"`
	ScheduledAt int64         `json:"scheduled_at"`
	Status      string        `json:"status"`
	JobID       sql.NullInt64 `json:"-"`
	CreatedAt   int64         `json:"created_at"`
	RunAt       sql.NullInt64 `json:"-"`
}

// JobIDValue — job_id как *int64 (для JSON).
func (s ScheduledSync) JobIDValue() *int64 {
	if s.JobID.Valid {
		v := s.JobID.Int64
		return &v
	}
	return nil
}

// RunAtValue — run_at как *int64 (для JSON).
func (s ScheduledSync) RunAtValue() *int64 {
	if s.RunAt.Valid {
		v := s.RunAt.Int64
		return &v
	}
	return nil
}

func (s *Store) CreateScheduledSync(categoryID, label, paramsJSON string, scheduledAt int64) (int64, error) {
	res, err := s.Hub.Exec(
		`INSERT INTO scheduled_syncs (category_id, label, params_json, scheduled_at, status, created_at)
		 VALUES (?,?,?,?, 'pending', ?)`,
		categoryID, label, paramsJSON, scheduledAt, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanScheduled(rows *sql.Rows) ([]ScheduledSync, error) {
	var out []ScheduledSync
	for rows.Next() {
		var x ScheduledSync
		if err := rows.Scan(&x.ID, &x.CategoryID, &x.Label, &x.ParamsJSON, &x.ScheduledAt,
			&x.Status, &x.JobID, &x.CreatedAt, &x.RunAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetScheduledSyncs — активные (pending/running) + завершённые за последние сутки.
func (s *Store) GetScheduledSyncs() ([]ScheduledSync, error) {
	rows, err := s.Hub.Query(
		`SELECT id, category_id, label, params_json, scheduled_at, status, job_id, created_at, run_at
		 FROM scheduled_syncs
		 WHERE status IN ('pending','running') OR run_at > ?
		 ORDER BY scheduled_at ASC`, time.Now().Unix()-86400)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduled(rows)
}

// ClaimDueScheduledSyncs — атомарно берёт задачи, которым пора (pending и scheduled_at<=now),
// помечает 'running' и возвращает их. Гонки исключены через UPDATE...WHERE status='pending'.
func (s *Store) ClaimDueScheduledSyncs() ([]ScheduledSync, error) {
	rows, err := s.Hub.Query(
		`SELECT id, category_id, label, params_json, scheduled_at, status, job_id, created_at, run_at
		 FROM scheduled_syncs
		 WHERE status='pending' AND scheduled_at <= ?
		 ORDER BY scheduled_at ASC LIMIT 20`, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	due, err := scanScheduled(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	var claimed []ScheduledSync
	for _, x := range due {
		res, err := s.Hub.Exec(`UPDATE scheduled_syncs SET status='running' WHERE id=? AND status='pending'`, x.ID)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 1 {
			claimed = append(claimed, x)
		}
	}
	return claimed, nil
}

// FinishScheduledSync — проставляет финальный статус ('done'/'error') и job_id.
func (s *Store) FinishScheduledSync(id, jobID int64, status string) error {
	_, err := s.Hub.Exec(
		`UPDATE scheduled_syncs SET status=?, job_id=?, run_at=? WHERE id=?`,
		status, jobID, time.Now().Unix(), id)
	return err
}

// CancelScheduledSync — отмена ещё не запущенной задачи.
func (s *Store) CancelScheduledSync(id int64) (bool, error) {
	res, err := s.Hub.Exec(`UPDATE scheduled_syncs SET status='cancelled' WHERE id=? AND status='pending'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
