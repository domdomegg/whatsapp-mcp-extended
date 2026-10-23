package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"whatsapp-bridge/internal/config"
)

// ContactStore reads whatsmeow's own contact table (whatsmeow_contacts in
// whatsapp.db). whatsmeow owns that file; this is a second, read-only handle on
// it, replacing the Python server's direct sqlite reads.
type ContactStore struct {
	db *sql.DB
}

// OpenContactStore opens whatsapp.db in the store directory for reading.
func OpenContactStore() (*ContactStore, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_journal_mode=WAL&_busy_timeout=5000", filepath.Join(config.StoreDir(), "whatsapp.db"))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open whatsapp.db: %w", err)
	}
	return &ContactStore{db: db}, nil
}

// NewContactStoreFromDB wraps an existing handle (tests).
func NewContactStoreFromDB(db *sql.DB) *ContactStore { return &ContactStore{db: db} }

// Close closes the handle.
func (c *ContactStore) Close() error { return c.db.Close() }

// ContactRow is one whatsmeow_contacts row.
type ContactRow struct {
	JID          string
	FirstName    string
	FullName     string
	PushName     string
	BusinessName string
}

const contactColumns = "their_jid, COALESCE(first_name,''), COALESCE(full_name,''), COALESCE(push_name,''), COALESCE(business_name,'')"

func scanContacts(rows *sql.Rows) ([]ContactRow, error) {
	defer rows.Close()
	var out []ContactRow
	for rows.Next() {
		var r ContactRow
		if err := rows.Scan(&r.JID, &r.FirstName, &r.FullName, &r.PushName, &r.BusinessName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Search matches name fields and the JID, case-insensitively, up to 50 rows.
func (c *ContactStore) Search(query string) ([]ContactRow, error) {
	like := "%" + query + "%"
	rows, err := c.db.Query(`SELECT `+contactColumns+` FROM whatsmeow_contacts
		WHERE LOWER(first_name) LIKE LOWER(?) OR LOWER(full_name) LIKE LOWER(?) OR LOWER(push_name) LIKE LOWER(?) OR their_jid LIKE ?
		LIMIT 50`, like, like, like, like)
	if err != nil {
		return nil, err
	}
	return scanContacts(rows)
}

// Get returns one contact by exact JID, or nil.
func (c *ContactStore) Get(jid string) (*ContactRow, error) {
	rows, err := c.db.Query(`SELECT `+contactColumns+` FROM whatsmeow_contacts WHERE their_jid = ? LIMIT 1`, jid)
	if err != nil {
		return nil, err
	}
	found, err := scanContacts(rows)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

// ListAll returns non-group contacts ordered by their best display name.
func (c *ContactStore) ListAll(limit int) ([]ContactRow, error) {
	rows, err := c.db.Query(`SELECT `+contactColumns+` FROM whatsmeow_contacts
		WHERE their_jid NOT LIKE '%@g.us'
		ORDER BY CASE
			WHEN full_name IS NOT NULL AND full_name != '' THEN full_name
			WHEN push_name IS NOT NULL AND push_name != '' THEN push_name
			WHEN first_name IS NOT NULL AND first_name != '' THEN first_name
			ELSE their_jid END
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanContacts(rows)
}

// PhoneFromJID is the user part of a JID ("123@s.whatsapp.net" -> "123").
func PhoneFromJID(jid string) string {
	if i := strings.Index(jid, "@"); i >= 0 {
		return jid[:i]
	}
	return jid
}
