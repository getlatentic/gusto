package cloud

import (
	"context"
	"database/sql"
	"errors"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Names of the small settings the front doors share.
const (
	settingDemoLimits  = "demo_limits"
	settingDemosPaused = "demos_paused"
	settingSessionKey  = "session_key"
)

var putSetting = dbkit.Upsert{Table: "settings", Columns: []string{"key", "value"}, Conflict: []string{"key"}, Update: []string{"value"}}

var keepSetting = dbkit.Upsert{Table: "settings", Columns: []string{"key", "value"}, Conflict: []string{"key"}}

func (s *Store) setting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(context.Background(), `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *Store) saveSetting(key, value string) error {
	_, err := s.db.ExecContext(context.Background(), putSetting.SQL(1), key, value)
	return err
}

func (s *Store) dropSetting(key string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM settings WHERE key = ?`, key)
	return err
}
