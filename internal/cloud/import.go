package cloud

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const importName = "files"

// ImportRecord says that the files an earlier release kept were loaded into the database, when, and how many
// records of each kind went in and how many were left out.
type ImportRecord struct {
	At       time.Time      `json:"at"`
	Imported map[string]int `json:"imported"`
	Skipped  map[string]int `json:"skipped,omitempty"`
}

// Summary is the records that went in, as one line, kind by kind.
func (r ImportRecord) Summary() string { return describe(r.Imported) }

func describe(counts map[string]int) string {
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	text := ""
	for _, kind := range kinds {
		if text != "" {
			text += ", "
		}
		text += strconv.Itoa(counts[kind]) + " " + kind
	}
	return text
}

// importOnce loads the old files into a database that has no import record, in one transaction that takes the
// import lock, so that front doors started together import once between them. It never changes the files. It says
// whether this call did the import.
func (s *Store) importOnce(ctx context.Context) (record ImportRecord, ran bool, err error) {
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		ran = false
		if err := s.lock(ctx, tx, "import"); err != nil {
			return err
		}
		prior, found, err := s.importRecord(ctx, tx)
		if err != nil {
			return err
		}
		if found {
			record = prior
			return nil
		}
		if record, err = s.importFiles(ctx, tx); err != nil {
			return err
		}
		ran = true
		return s.saveImportRecord(ctx, tx, record)
	})
	if err == nil && ran {
		s.logImport(record)
	}
	return record, ran, err
}

// logImport says what an import did. A database made on a root that holds no old files has nothing to say.
func (s *Store) logImport(record ImportRecord) {
	if len(record.Skipped) == 0 && record.Total() == 0 {
		return
	}
	slog.Info("gusto cloud loaded the files of the earlier release into the database; the files are left as they were",
		"database", s.db.URLForLog(), "imported", record.Summary(), "left out", describe(record.Skipped))
}

// Total is how many records went in.
func (r ImportRecord) Total() int {
	total := 0
	for _, n := range r.Imported {
		total += n
	}
	return total
}

func (s *Store) importRecord(ctx context.Context, q dbkit.Querier) (ImportRecord, bool, error) {
	var at dbkit.Time
	var counts string
	err := q.QueryRowContext(ctx, `SELECT ran_at, counts FROM imports WHERE name = ?`, importName).Scan(&at, &counts)
	if err != nil {
		return ImportRecord{}, false, ignoreNoRows(err)
	}
	var record ImportRecord
	if err := json.Unmarshal([]byte(counts), &record); err != nil {
		return ImportRecord{}, false, err
	}
	record.At = at.Time
	return record, true, nil
}

func (s *Store) saveImportRecord(ctx context.Context, tx dbkit.Tx, record ImportRecord) error {
	text, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO imports (name, ran_at, counts) VALUES (?, ?, ?) ON CONFLICT (name) DO NOTHING`,
		importName, s.stamp(record.At), string(text))
	return err
}

// importFiles reads the old layout and inserts what it holds. Every insert leaves a row that is there
// already alone, so running it again changes nothing.
func (s *Store) importFiles(ctx context.Context, tx dbkit.Tx) (ImportRecord, error) {
	now := s.now().UTC().Truncate(time.Second)
	old := readLegacy(s.root, now)
	imported, err := s.insertLegacy(ctx, tx, old)
	if err != nil {
		return ImportRecord{}, err
	}
	return ImportRecord{At: now, Imported: imported, Skipped: old.skipped}, nil
}
