package operator

import (
	"encoding/csv"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// contactRow is a note a visitor sent the team, with when in the words it is shown in.
type contactRow struct {
	Sent, SentAt, Email, Company, Note string
}

func contactRows(contacts []cloud.Contact, now time.Time) []contactRow {
	rows := make([]contactRow, 0, len(contacts))
	for _, c := range contacts {
		rows = append(rows, contactRow{Sent: ago(c.Sent, now), SentAt: c.Sent.Format("2 Jan 2006 15:04 MST"), Email: c.Email, Company: c.Company, Note: c.Note})
	}
	return rows
}

// contactsCSV is the newest notes visitors sent, for a spreadsheet. A cell a spreadsheet would read as a formula is written as
// text, since the visitor typed it.
func (a *App) contactsCSV(w http.ResponseWriter, r *http.Request) {
	contacts, err := a.Store.Contacts(r.Context())
	if err != nil {
		http.Error(w, "the notes visitors sent could not be read", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="gusto-contacts.csv"`)
	out := csv.NewWriter(w)
	_ = out.Write([]string{"sent", "email", "company", "note"})
	for _, c := range contacts {
		_ = out.Write([]string{when(c.Sent), text(c.Email), text(c.Company), text(c.Note)})
	}
	out.Flush()
}
