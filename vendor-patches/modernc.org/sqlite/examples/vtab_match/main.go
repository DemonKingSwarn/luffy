package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vtab"
)

// matchModule demonstrates MATCH pushdown. It exposes a single TEXT column `val`.
// BestIndex recognizes OpMATCH on column 0, sets ArgIndex=0 and Omit=true.
// Filter implements a simple substring match to keep the demo concise.
type matchModule struct{}
type matchTable struct{}
type matchCursor struct {
	rows []string
	pos  int
}

func (m *matchModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("match: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &matchTable{}, nil
}

func (m *matchModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("match: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &matchTable{}, nil
}

func (t *matchTable) BestIndex(info *vtab.IndexInfo) error {
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if !c.Usable || c.Column != 0 {
			continue
		}
		if c.Op == vtab.OpMATCH {
			c.ArgIndex = 0
			c.Omit = true // fully enforce in Filter
			info.IdxNum = 20
			return nil
		}
	}
	info.IdxNum = 0
	return nil
}

func (t *matchTable) Open() (vtab.Cursor, error) {
	return &matchCursor{rows: []string{"alpha", "beta", "alpine", "match", "atlas"}, pos: 0}, nil
}
func (t *matchTable) Disconnect() error { return nil }
func (t *matchTable) Destroy() error    { return nil }

func (c *matchCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	all := []string{"alpha", "beta", "alpine", "match", "atlas"}
	c.rows = all[:]
	if idxNum == 20 && len(vals) == 1 {
		term, _ := vals[0].(string)
		term = strings.ToLower(term)
		filtered := make([]string, 0, len(all))
		for _, s := range all {
			if strings.Contains(strings.ToLower(s), term) {
				filtered = append(filtered, s)
			}
		}
		c.rows = filtered
	}
	c.pos = 0
	return nil
}

func (c *matchCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}
func (c *matchCursor) Eof() bool { return c.pos >= len(c.rows) }
func (c *matchCursor) Column(col int) (vtab.Value, error) {
	if col == 0 && c.pos < len(c.rows) {
		return c.rows[c.pos], nil
	}
	return nil, nil
}
func (c *matchCursor) Rowid() (int64, error) { return int64(c.pos + 1), nil }
func (c *matchCursor) Close() error          { return nil }

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "match", &matchModule{}); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt USING match(val)`); err != nil {
		log.Fatal(err)
	}

	fmt.Println("-- MATCH pushdown")
	dump(db, `SELECT val FROM vt WHERE val MATCH 'al' ORDER BY val`)
}

func dump(db *sql.DB, q string) {
	rows, err := db.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			log.Fatal(err)
		}
		fmt.Println(" ", s)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}
