package cloud

import (
	"context"
	"database/sql"
	"errors"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Names of the small settings the gusto cloud servers share.
const (
	settingDemoLimits  = "demo_limits"
	settingDemosPaused = "demos_paused"
	settingSessionKey  = "session_key"
)

var putSetting = dbkit.Upsert{Table: "settings", Columns: []string{"key", "value"}, Conflict: []string{"key"}, Update: []string{"value"}}

var keepSetting = dbkit.Upsert{Table: "settings", Columns: []string{"key", "value"}, Conflict: []string{"key"}}

func (s *Store) setting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *Store) saveSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, putSetting.SQL(1), key, value)
	return err
}

func (s *Store) dropSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}
