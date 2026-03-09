package main

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"regexp"

	_ "modernc.org/sqlite"
	sqlite "modernc.org/sqlite"
	"modernc.org/sqlite/vtab"
)

// Register a minimal REGEXP(pattern, value) UDF so SQLite can evaluate
// expressions when the vtab does not Omit the constraint.
func init() {
	err := sqlite.RegisterFunction("regexp", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) != 2 {
				return int64(0), nil
			}
			pat, _ := args[0].(string)
			val, _ := args[1].(string)
			// Empty pattern never matches.
			if pat == "" {
				return int64(0), nil
			}
			matched, err := regexp.MatchString(pat, val)
			if err != nil {
				return nil, err
			}
			if matched {
				return int64(1), nil
			}
			return int64(0), nil
		},
	})
	if err != nil {
		log.Fatalf("register regexp UDF: %v", err)
	}
}

// reModule demonstrates REGEXP pushdown. It exposes a single TEXT column `val`.
// If created with USING re(omit=true), BestIndex sets Omit=true and Filter fully
// enforces the constraint. Otherwise, Omit=false and SQLite re-checks with the
// REGEXP UDF.
type reModule struct{}
type reTable struct{ omit bool }
type reCursor struct {
	rows []string
	pos  int
}

func (m *reModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("re: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &reTable{omit: hasOmit(args[3:])}, nil
}

func (m *reModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("re: missing table name in args")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &reTable{omit: hasOmit(args[3:])}, nil
}

func hasOmit(args []string) bool {
	for _, a := range args {
		if a == "omit=true" {
			return true
		}
	}
	return false
}

func (t *reTable) BestIndex(info *vtab.IndexInfo) error {
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if !c.Usable || c.Column != 0 {
			continue
		}
		if c.Op == vtab.OpREGEXP {
			c.ArgIndex = 0
			c.Omit = t.omit
			info.IdxNum = 10 // REGEXP plan
			break
		}
	}
	return nil
}

func (t *reTable) Open() (vtab.Cursor, error) {
	// Small fixed dataset for demo purposes.
	return &reCursor{rows: []string{"alpha", "beta", "alpine", "regex"}, pos: 0}, nil
}

func (t *reTable) Disconnect() error { return nil }
func (t *reTable) Destroy() error    { return nil }

func (c *reCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	all := []string{"alpha", "beta", "alpine", "regex"}
	c.rows = all[:]
	if idxNum == 10 && len(vals) == 1 {
		pat, _ := vals[0].(string)
		re, err := regexp.Compile(pat)
		if err != nil {
			// Leave rows empty on invalid pattern; SQLite will still re-check if Omit=false.
			c.rows = nil
		} else {
			filtered := make([]string, 0, len(all))
			for _, s := range all {
				if re.MatchString(s) {
					filtered = append(filtered, s)
				}
			}
			c.rows = filtered
		}
	}
	c.pos = 0
	return nil
}

func (c *reCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}
func (c *reCursor) Eof() bool { return c.pos >= len(c.rows) }
func (c *reCursor) Column(col int) (vtab.Value, error) {
	if col == 0 && c.pos < len(c.rows) {
		return c.rows[c.pos], nil
	}
	return nil, nil
}
func (c *reCursor) Rowid() (int64, error) { return int64(c.pos + 1), nil }
func (c *reCursor) Close() error          { return nil }

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "re", &reModule{}); err != nil {
		log.Fatal(err)
	}

	// Table with Omit=false: SQLite re-checks REGEXP using the UDF.
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt_re USING re(val)`); err != nil {
		log.Fatal(err)
	}
	// Table with Omit=true: vtab fully enforces REGEXP.
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt_re_omit USING re(val, omit=true)`); err != nil {
		log.Fatal(err)
	}

	fmt.Println("-- REGEXP with Omit=false (SQLite re-checks)")
	dump(db, `SELECT val FROM vt_re WHERE val REGEXP '^al' ORDER BY val`)

	fmt.Println("-- REGEXP with Omit=true (vtab enforces)")
	dump(db, `SELECT val FROM vt_re_omit WHERE val REGEXP '^al' ORDER BY val`)
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
