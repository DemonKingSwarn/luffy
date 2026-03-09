package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vtab"
)

// A tiny CSV loader example that:
// - Reads header to declare schema dynamically
// - Supports scanning, and basic INSERT/UPDATE/DELETE via Updater
// - Writes changes back to the CSV file on each change (naive)

type csvModule struct{}
type csvTable struct {
	file      string
	cols      []string
	rows      [][]string
	nextID    int64
	header    bool
	delimiter rune
	quote     rune
}
type csvCursor struct {
	t    *csvTable
	rows [][]string
	pos  int
}

func (m *csvModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	file, delim, header, quote := parseCSVArgs(args[3:])
	if file == "" {
		return nil, fmt.Errorf("csv: require filename=... arg")
	}
	t, err := loadCSV(file, delim, header, quote)
	if err != nil {
		return nil, err
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(%s)", args[2], strings.Join(t.cols, ","))); err != nil {
		return nil, err
	}
	return t, nil
}
func (m *csvModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	return m.Create(ctx, args)
}

func loadCSV(file string, delim rune, header bool, quote rune) (*csvTable, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if quote != 0 && quote != '"' {
		data = bytes.ReplaceAll(data, []byte(string(quote)), []byte("\""))
	}
	r := csv.NewReader(bufio.NewReader(bytes.NewReader(data)))
	r.FieldsPerRecord = -1
	if delim != 0 {
		r.Comma = delim
	}
	var hdr []string
	var rows [][]string
	if header {
		hdr, err = r.Read()
		if err != nil {
			return nil, fmt.Errorf("csv: read header: %w", err)
		}
		rows, err = r.ReadAll()
		if err != nil {
			return nil, err
		}
		for i := range rows {
			if len(rows[i]) < len(hdr) {
				pad := make([]string, len(hdr)-len(rows[i]))
				rows[i] = append(rows[i], pad...)
			} else if len(rows[i]) > len(hdr) {
				rows[i] = rows[i][:len(hdr)]
			}
		}
	} else {
		all, err := r.ReadAll()
		if err != nil {
			return nil, err
		}
		if len(all) == 0 {
			return nil, fmt.Errorf("csv: empty file with header=false")
		}
		n := len(all[0])
		hdr = make([]string, n)
		for i := 0; i < n; i++ {
			hdr[i] = fmt.Sprintf("c%d", i+1)
		}
		rows = all
		for i := range rows {
			if len(rows[i]) < n {
				pad := make([]string, n-len(rows[i]))
				rows[i] = append(rows[i], pad...)
			} else if len(rows[i]) > n {
				rows[i] = rows[i][:n]
			}
		}
	}
	t := &csvTable{file: file, cols: hdr, rows: rows, header: header, delimiter: delim, quote: quote}
	t.nextID = int64(len(rows) + 1)
	return t, nil
}

func (t *csvTable) BestIndex(info *vtab.IndexInfo) error {
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if !c.Usable || c.Op != vtab.OpEQ || c.Column < 0 || c.Column >= len(t.cols) {
			continue
		}
		c.ArgIndex = 0
		c.Omit = true
		info.IdxNum = 1
		info.IdxStr = strconv.Itoa(c.Column)
		return nil
	}
	info.IdxNum = 0
	return nil
}
func (t *csvTable) Open() (vtab.Cursor, error) {
	return &csvCursor{t: t, rows: append(([][]string)(nil), t.rows...), pos: 0}, nil
}
func (t *csvTable) Disconnect() error { return nil }
func (t *csvTable) Destroy() error    { return nil }

// Updater implementation
func (t *csvTable) Insert(cols []vtab.Value, rowid *int64) error {
	rec := valuesToStrings(cols, len(t.cols))
	t.rows = append(t.rows, rec)
	if *rowid == 0 {
		*rowid = t.nextID
	}
	t.nextID++
	return t.flush()
}
func (t *csvTable) Update(oldRowid int64, cols []vtab.Value, newRowid *int64) error {
	idx := int(oldRowid - 1)
	if idx < 0 || idx >= len(t.rows) {
		return fmt.Errorf("csv: rowid %d out of range", oldRowid)
	}
	rec := valuesToStrings(cols, len(t.cols))
	t.rows[idx] = rec
	if newRowid != nil && *newRowid != 0 && *newRowid != oldRowid {
		// naive: swap rows to simulate rowid change
		nidx := int(*newRowid - 1)
		if nidx >= 0 && nidx < len(t.rows) {
			t.rows[idx], t.rows[nidx] = t.rows[nidx], t.rows[idx]
		}
	}
	return t.flush()
}
func (t *csvTable) Delete(oldRowid int64) error {
	idx := int(oldRowid - 1)
	if idx < 0 || idx >= len(t.rows) {
		return fmt.Errorf("csv: rowid %d out of range", oldRowid)
	}
	t.rows = append(t.rows[:idx], t.rows[idx+1:]...)
	return t.flush()
}

func (t *csvTable) flush() error {
	// Write header + rows back to file, respecting delimiter and quote.
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if t.delimiter != 0 {
		w.Comma = t.delimiter
	}
	if t.header {
		if err := w.Write(t.cols); err != nil {
			return err
		}
	}
	if err := w.WriteAll(t.rows); err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	out := buf.Bytes()
	if t.quote != 0 && t.quote != '"' {
		out = bytes.ReplaceAll(out, []byte("\""), []byte(string(t.quote)))
	}
	tmp := t.file + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, t.file)
}

func (c *csvCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	c.pos = 0
	c.rows = append(c.rows[:0], c.t.rows...)
	if idxNum == 1 {
		col, err := strconv.Atoi(idxStr)
		if err != nil || col < 0 || col >= len(c.t.cols) {
			return nil
		}
		if len(vals) == 0 {
			return nil
		}
		target := fmt.Sprint(vals[0])
		filtered := make([][]string, 0, len(c.rows))
		for _, r := range c.rows {
			if col < len(r) && r[col] == target {
				filtered = append(filtered, r)
			}
		}
		c.rows = filtered
	}
	return nil
}
func (c *csvCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}
func (c *csvCursor) Eof() bool { return c.pos >= len(c.rows) }
func (c *csvCursor) Column(col int) (vtab.Value, error) {
	if c.pos >= len(c.rows) || col >= len(c.t.cols) {
		return nil, nil
	}
	return c.rows[c.pos][col], nil
}
func (c *csvCursor) Rowid() (int64, error) { return int64(c.pos + 1), nil }
func (c *csvCursor) Close() error          { return nil }

func parseCSVArgs(args []string) (file string, delim rune, header bool, quote rune) {
	// Defaults
	delim = ','
	header = true
	quote = '"'
	for _, a := range args {
		kv := strings.SplitN(a, "=", 2)
		k := kv[0]
		v := ""
		if len(kv) == 2 {
			v = unquote(kv[1])
		}
		switch k {
		case "filename":
			file = v
		case "delimiter":
			if v == "\\t" || v == "\t" {
				delim = '\t'
			} else if v != "" {
				r, _ := utf8.DecodeRuneInString(v)
				if r != utf8.RuneError {
					delim = r
				}
			}
		case "header":
			lv := strings.ToLower(v)
			if lv == "false" || lv == "0" || lv == "no" {
				header = false
			} else if lv == "true" || lv == "1" || lv == "yes" {
				header = true
			}
		case "quote":
			if v != "" {
				r, _ := utf8.DecodeRuneInString(v)
				if r != utf8.RuneError {
					quote = r
				}
			}
		}
	}
	return
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func valuesToStrings(vals []vtab.Value, n int) []string {
	out := make([]string, n)
	for i := 0; i < n && i < len(vals); i++ {
		if vals[i] == nil {
			continue
		}
		out[i] = fmt.Sprint(vals[i])
	}
	return out
}

func main() {
	// Create a temp CSV file with a header and a couple rows.
	dir, err := os.MkdirTemp("", "csvdemo-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(file, []byte("name,email\nAlice,alice@example.com\nBob,bob@example.com\n"), 0644); err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "csv", &csvModule{}); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE VIRTUAL TABLE people USING csv(filename=%q, delimiter=",", header=true, quote='"')`, file)); err != nil {
		log.Fatal(err)
	}

	fmt.Println("-- initial")
	dump(db, `SELECT rowid, name, email FROM people ORDER BY rowid`)

	fmt.Println("-- insert Carol")
	if _, err := db.Exec(`INSERT INTO people(name, email) VALUES(?, ?)`, "Carol", "carol@example.com"); err != nil {
		log.Fatal(err)
	}
	dump(db, `SELECT rowid, name, email FROM people ORDER BY rowid`)

	fmt.Println("-- update Bob -> Robert")
	if _, err := db.Exec(`UPDATE people SET name = ? WHERE rowid = ?`, "Robert", 2); err != nil {
		log.Fatal(err)
	}
	dump(db, `SELECT rowid, name, email FROM people ORDER BY rowid`)

	fmt.Println("-- delete Alice")
	if _, err := db.Exec(`DELETE FROM people WHERE rowid = ?`, 1); err != nil {
		log.Fatal(err)
	}
	dump(db, `SELECT rowid, name, email FROM people ORDER BY rowid`)
}

func dump(db *sql.DB, q string) {
	rows, err := db.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			log.Fatal(err)
		}
		fmt.Println(" ", id, name, email)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}
