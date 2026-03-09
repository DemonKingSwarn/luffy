package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vtab"
)

// echoModule implements a tiny read-only vtab with a single TEXT column `val`.
// It demonstrates ctx.Declare, BestIndex with ArgIndex/Omit, and Filter.
type echoModule struct{}
type echoTable struct{}
type echoCursor struct {
	rows []string
	pos  int
}

func (m *echoModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	// args: [moduleName, dbName, tableName, ...module args]
	if len(args) < 3 {
		return nil, fmt.Errorf("echo: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &echoTable{}, nil
}

func (m *echoModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("echo: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &echoTable{}, nil
}

func (t *echoTable) BestIndex(info *vtab.IndexInfo) error {
	// Recognize usable constraints on column 0 for EQ and LIKE.
	// Assign 0-based ArgIndex so Filter receives RHS in vals[0].
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if !c.Usable || c.Column != 0 {
			continue
		}
		switch c.Op {
		case vtab.OpEQ:
			c.ArgIndex = 0
			c.Omit = true // We will fully enforce equality in Filter.
			info.IdxNum = 1
			return nil
		case vtab.OpLIKE:
			c.ArgIndex = 0
			// Leave Omit=false to allow SQLite to re-check if needed.
			info.IdxNum = 2
			return nil
		}
	}
	info.IdxNum = 0 // full scan
	return nil
}

func (t *echoTable) Open() (vtab.Cursor, error) {
	return &echoCursor{rows: []string{"alpha", "beta", "alpine"}, pos: 0}, nil
}

func (t *echoTable) Disconnect() error { return nil }
func (t *echoTable) Destroy() error    { return nil }

func (c *echoCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	// Base data set
	all := []string{"alpha", "beta", "alpine"}
	c.rows = all[:]

	switch idxNum {
	case 1: // EQ
		if len(vals) == 1 {
			want, _ := vals[0].(string)
			filtered := make([]string, 0, 1)
			for _, s := range all {
				if s == want {
					filtered = append(filtered, s)
				}
			}
			c.rows = filtered
		}
	case 2: // LIKE (prefix only: 'prefix%')
		if len(vals) == 1 {
			pat, _ := vals[0].(string)
			prefix := pat
			if n := len(pat); n > 0 && pat[n-1] == '%' {
				prefix = pat[:n-1]
			}
			filtered := make([]string, 0, len(all))
			for _, s := range all {
				if len(prefix) == 0 || (len(s) >= len(prefix) && s[:len(prefix)] == prefix) {
					filtered = append(filtered, s)
				}
			}
			c.rows = filtered
		}
	}
	c.pos = 0
	return nil
}

func (c *echoCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}

func (c *echoCursor) Eof() bool { return c.pos >= len(c.rows) }

func (c *echoCursor) Column(col int) (vtab.Value, error) {
	if col == 0 && c.pos < len(c.rows) {
		return c.rows[c.pos], nil
	}
	return nil, nil
}

func (c *echoCursor) Rowid() (int64, error) { return int64(c.pos + 1), nil }
func (c *echoCursor) Close() error          { return nil }

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "echo", &echoModule{}); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt USING echo(val)`); err != nil {
		log.Fatal(err)
	}

	fmt.Println("-- full scan")
	dump(db, `SELECT val FROM vt ORDER BY val`)

	fmt.Println("-- equality pushdown")
	dump(db, `SELECT val FROM vt WHERE val = 'alpha'`)

	fmt.Println("-- LIKE prefix pushdown")
	dump(db, `SELECT val FROM vt WHERE val LIKE 'al%' ORDER BY val`)
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
