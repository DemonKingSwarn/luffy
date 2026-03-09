package sqlite

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"

	"modernc.org/sqlite/vtab"
)

// dummyModule is a minimal vtab.Module implementation used to verify that the
// vtab bridge (registration + trampolines) works end-to-end inside this
// repository, without relying on external modules.
type dummyModule struct{}

// dummyTable implements vtab.Table for the dummy module.
type dummyTable struct{}

// dummyCursor implements vtab.Cursor for the dummy module. It returns a small
// fixed result set for testing.
type dummyCursor struct {
	rows []struct {
		rowid int64
		val   string
	}
	pos int
}

// lastIndexInfo captures the most recent IndexInfo seen by dummyTable.BestIndex
// so that tests can assert on constraints and orderings.
var lastIndexInfo *vtab.IndexInfo

// Create implements vtab.Module.Create.
func (m *dummyModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	_ = ctx
	// Declare schema based on args: args[2]=table name, args[3:]=columns.
	if len(args) >= 3 {
		cols := "x"
		if len(args) > 3 {
			cols = strings.Join(args[3:], ",")
		}
		if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(%s)", args[2], cols)); err != nil {
			return nil, err
		}
	}
	return &dummyTable{}, nil
}

// Connect implements vtab.Module.Connect.
func (m *dummyModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	_ = ctx
	// Same schema logic in Connect.
	if len(args) >= 3 {
		cols := "x"
		if len(args) > 3 {
			cols = strings.Join(args[3:], ",")
		}
		if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(%s)", args[2], cols)); err != nil {
			return nil, err
		}
	}
	return &dummyTable{}, nil
}

func (t *dummyTable) BestIndex(info *vtab.IndexInfo) error {
	// Record the last IndexInfo for inspection in tests.
	lastIndexInfo = info
	// Choose a fixed plan ID so we can verify that IdxNum flows through
	// sqlite3_index_info into Cursor.Filter.
	info.IdxNum = 1
	return nil
}

// Open creates a new dummyCursor.
func (t *dummyTable) Open() (vtab.Cursor, error) {
	_ = t
	return &dummyCursor{}, nil
}

// Disconnect is a no-op for the dummy table.
func (t *dummyTable) Disconnect() error { return nil }

// Destroy is a no-op for the dummy table.
func (t *dummyTable) Destroy() error { return nil }

func (c *dummyCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	_ = idxStr
	_ = vals
	// Ensure that the planner-provided idxNum from BestIndex is propagated.
	// If idxNum is not 1, return a different rowset so the test would fail.
	if idxNum == 1 {
		c.rows = []struct {
			rowid int64
			val   string
		}{
			{rowid: 1, val: "alpha"},
			{rowid: 2, val: "beta"},
		}
	} else {
		c.rows = []struct {
			rowid int64
			val   string
		}{
			{rowid: 1, val: "unexpected"},
		}
	}
	c.pos = 0
	return nil
}

// Next advances the cursor.
func (c *dummyCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}

// Eof reports whether the cursor is past the last row.
func (c *dummyCursor) Eof() bool { return c.pos >= len(c.rows) }

// Column returns the string value for column 0 and NULL for others.
func (c *dummyCursor) Column(col int) (vtab.Value, error) {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return nil, nil
	}
	if col == 0 {
		return c.rows[c.pos].val, nil
	}
	return nil, nil
}

// Rowid returns the current rowid.
func (c *dummyCursor) Rowid() (int64, error) {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return 0, nil
	}
	return c.rows[c.pos].rowid, nil
}

// Close clears the cursor state.
func (c *dummyCursor) Close() error {
	c.rows = nil
	c.pos = 0
	return nil
}

// Omit test module types and methods
type omitModuleX struct{ omit bool }
type omitTableX struct{ omit bool }
type omitCursorX struct {
	rows []struct {
		rowid int64
		val   string
	}
	pos int
}

func (m *omitModuleX) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(val)"); err != nil {
		return nil, err
	}
	return &omitTableX{omit: m.omit}, nil
}
func (m *omitModuleX) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(val)"); err != nil {
		return nil, err
	}
	return &omitTableX{omit: m.omit}, nil
}
func (t *omitTableX) BestIndex(info *vtab.IndexInfo) error {
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if c.Usable && c.Op == vtab.OpEQ && c.Column == 0 {
			c.ArgIndex = 0
			c.Omit = t.omit
			break
		}
	}
	return nil
}
func (t *omitTableX) Open() (vtab.Cursor, error) { return &omitCursorX{}, nil }
func (t *omitTableX) Disconnect() error          { return nil }
func (t *omitTableX) Destroy() error             { return nil }
func (c *omitCursorX) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	c.rows = []struct {
		rowid int64
		val   string
	}{{1, "alpha"}, {2, "beta"}}
	c.pos = 0
	return nil
}
func (c *omitCursorX) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}
func (c *omitCursorX) Eof() bool { return c.pos >= len(c.rows) }
func (c *omitCursorX) Column(col int) (vtab.Value, error) {
	if col == 0 {
		return c.rows[c.pos].val, nil
	}
	return nil, nil
}
func (c *omitCursorX) Rowid() (int64, error) { return c.rows[c.pos].rowid, nil }
func (c *omitCursorX) Close() error          { return nil }

// Operator capture module types and methods
type opsModuleX struct{}
type opsTableX struct{}

var seenOpsOps []vtab.ConstraintOp

func (m *opsModuleX) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(c1)"); err != nil {
		return nil, err
	}
	return &opsTableX{}, nil
}
func (m *opsModuleX) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(c1)"); err != nil {
		return nil, err
	}
	return &opsTableX{}, nil
}
func (t *opsTableX) BestIndex(info *vtab.IndexInfo) error {
	seenOpsOps = nil
	for _, c := range info.Constraints {
		if c.Usable {
			seenOpsOps = append(seenOpsOps, c.Op)
		}
	}
	return nil
}
func (t *opsTableX) Open() (vtab.Cursor, error) { return &dummyCursor{}, nil }
func (t *opsTableX) Disconnect() error          { return nil }
func (t *opsTableX) Destroy() error             { return nil }

// matchModuleX exercises MATCH constraint support.
type matchModuleX struct{}
type matchTableX struct{}
type matchCursorX struct {
	rows []struct {
		rowid int64
		val   string
	}
	pos int
}

func (m *matchModuleX) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.EnableConstraintSupport(); err != nil {
		return nil, err
	}
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(val)"); err != nil {
		return nil, err
	}
	return &matchTableX{}, nil
}

func (m *matchModuleX) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if err := ctx.EnableConstraintSupport(); err != nil {
		return nil, err
	}
	if err := ctx.Declare("CREATE TABLE " + args[2] + "(val)"); err != nil {
		return nil, err
	}
	return &matchTableX{}, nil
}

func (t *matchTableX) BestIndex(info *vtab.IndexInfo) error {
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if c.Usable && c.Op == vtab.OpMATCH && c.Column == 0 {
			c.ArgIndex = 0
			c.Omit = true
			info.IdxNum = 1
			return nil
		}
	}
	info.IdxNum = 0
	return nil
}

func (t *matchTableX) Open() (vtab.Cursor, error) { return &matchCursorX{}, nil }
func (t *matchTableX) Disconnect() error          { return nil }
func (t *matchTableX) Destroy() error             { return nil }

func (c *matchCursorX) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	_ = idxStr
	all := []string{"alpha", "alpine", "beta"}
	c.rows = c.rows[:0]
	c.pos = 0
	if idxNum != 1 || len(vals) == 0 {
		return nil
	}
	query, ok := vals[0].(string)
	if !ok {
		return nil
	}
	var rowid int64
	for _, v := range all {
		rowid++
		if strings.Contains(v, query) {
			c.rows = append(c.rows, struct {
				rowid int64
				val   string
			}{rowid: rowid, val: v})
		}
	}
	return nil
}

func (c *matchCursorX) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}

func (c *matchCursorX) Eof() bool { return c.pos >= len(c.rows) }

func (c *matchCursorX) Column(col int) (vtab.Value, error) {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return nil, nil
	}
	if col == 0 {
		return c.rows[c.pos].val, nil
	}
	return nil, nil
}

func (c *matchCursorX) Rowid() (int64, error) {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return 0, nil
	}
	return c.rows[c.pos].rowid, nil
}

func (c *matchCursorX) Close() error {
	c.rows = nil
	c.pos = 0
	return nil
}

// TestDummyModuleVtab verifies that a simple vtab module implemented in Go
// can be registered and queried through the modernc.org/sqlite driver.
func TestDummyModuleVtab(t *testing.T) {
	// Open an in-memory database using this driver.
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	// Register the dummy module.
	if err := vtab.RegisterModule(db, "dummy", &dummyModule{}); err != nil {
		t.Fatalf("vtab.RegisterModule failed: %v", err)
	}

	// Create a virtual table using the dummy module.
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt USING dummy(value)`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE vt USING dummy failed: %v", err)
	}

	// Query the virtual table with a simple equality constraint.
	rows, err := db.Query(`SELECT rowid, value FROM vt WHERE value = 'alpha' ORDER BY rowid`)
	if err != nil {
		t.Fatalf("SELECT from vt failed: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var rowid int64
		var value string
		if err := rows.Scan(&rowid, &value); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		got = append(got, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d (%v)", len(got), got)
	}
	if got[0] != "alpha" {
		t.Fatalf("unexpected value from vt: %v (want [alpha])", got)
	}

	// Verify that BestIndex saw a usable equality constraint on column 0.
	if lastIndexInfo == nil {
		t.Fatalf("expected BestIndex to be called and lastIndexInfo to be set")
	}
	found := false
	for _, c := range lastIndexInfo.Constraints {
		if c.Column == 0 && c.Op == vtab.OpEQ && c.Usable {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("BestIndex did not observe a usable EQ constraint on column 0; got %+v", lastIndexInfo.Constraints)
	}

	// Verify ColUsed indicates column 0 is referenced.
	if lastIndexInfo.ColUsed == 0 || (lastIndexInfo.ColUsed&1) == 0 {
		t.Fatalf("expected ColUsed to include column 0; got %b", lastIndexInfo.ColUsed)
	}
}

// argIndexModule exercises Constraint.ArgIndex and ensures that the arguments
// passed to Cursor.Filter arrive in the expected order.
type argIndexModule struct{}

type argIndexTable struct{}

type argIndexCursor struct {
	rows []int64
	pos  int
}

var (
	argIndexFilterVals  []vtab.Value
	argIndexFilterCalls int
)

func (m *argIndexModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	// Declare a simple schema using provided column names.
	if len(args) >= 3 {
		cols := "c1,c2"
		if len(args) > 3 {
			cols = strings.Join(args[3:], ",")
		}
		if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(%s)", args[2], cols)); err != nil {
			return nil, err
		}
	}
	return &argIndexTable{}, nil
}

func (m *argIndexModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) >= 3 {
		cols := "c1,c2"
		if len(args) > 3 {
			cols = strings.Join(args[3:], ",")
		}
		if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(%s)", args[2], cols)); err != nil {
			return nil, err
		}
	}
	return &argIndexTable{}, nil
}

func (t *argIndexTable) BestIndex(info *vtab.IndexInfo) error {
	// Assign ArgIndex sequentially (0-based) for all usable EQ constraints so
	// that SQLite passes their RHS values to Filter in argv[] in that order.
	next := 0
	for i := range info.Constraints {
		c := &info.Constraints[i]
		if !c.Usable || c.Op != vtab.OpEQ {
			continue
		}
		c.ArgIndex = next
		next++
	}
	return nil
}

func (t *argIndexTable) Open() (vtab.Cursor, error) {
	_ = t
	return &argIndexCursor{}, nil
}

func (t *argIndexTable) Disconnect() error { return nil }

func (t *argIndexTable) Destroy() error { return nil }

func (c *argIndexCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	_ = idxNum
	_ = idxStr
	// Capture the values passed from SQLite so the test can assert on them.
	argIndexFilterCalls++
	argIndexFilterVals = append([]vtab.Value(nil), vals...)
	// Expose a single dummy row so the query returns one result.
	c.rows = []int64{1}
	c.pos = 0
	return nil
}

func (c *argIndexCursor) Next() error {
	if c.pos < len(c.rows) {
		c.pos++
	}
	return nil
}

func (c *argIndexCursor) Eof() bool { return c.pos >= len(c.rows) }

func (c *argIndexCursor) Column(col int) (vtab.Value, error) {
	_ = col
	// We only select rowid in the test, so Column is unused.
	return nil, nil
}

func (c *argIndexCursor) Rowid() (int64, error) {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return 0, nil
	}
	return c.rows[c.pos], nil
}

func (c *argIndexCursor) Close() error { return nil }

// TestVtabConstraintArgIndex verifies that setting Constraint.ArgIndex in
// BestIndex causes Cursor.Filter to receive the corresponding argument values
// in argv[] in the expected order.
func TestVtabConstraintArgIndex(t *testing.T) {
	argIndexFilterVals = nil
	argIndexFilterCalls = 0

	if err := vtab.RegisterModule(nil, "argtest", &argIndexModule{}); err != nil {
		t.Fatalf("vtab.RegisterModule(argtest) failed: %v", err)
	}
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE VIRTUAL TABLE at USING argtest(c1, c2)`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE at USING argtest failed: %v", err)
	}

	rows, err := db.Query(`SELECT rowid FROM at WHERE c1 = ? AND c2 = ?`, 10, 20)
	if err != nil {
		t.Fatalf("SELECT from at failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rowid int64
		if err := rows.Scan(&rowid); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if argIndexFilterCalls == 0 {
		t.Fatalf("expected Filter to be called at least once")
	}
	if len(argIndexFilterVals) != 2 {
		t.Fatalf("expected 2 argv values in Filter, got %d (%v)", len(argIndexFilterVals), argIndexFilterVals)
	}
	v1, ok1 := argIndexFilterVals[0].(int64)
	v2, ok2 := argIndexFilterVals[1].(int64)
	if !ok1 || !ok2 {
		t.Fatalf("unexpected argv types: %T, %T", argIndexFilterVals[0], argIndexFilterVals[1])
	}
	if v1 != 10 || v2 != 20 {
		t.Fatalf("unexpected argv values in Filter: got (%v, %v), want (10, 20)", v1, v2)
	}
}

// TestVtabOmitConstraintEffect verifies that setting Constraint.Omit causes
// SQLite to not re-evaluate the parent constraint and relies on the vtab to
// enforce it.
func TestVtabOmitConstraintEffect(t *testing.T) {
	if err := vtab.RegisterModule(nil, "omit_off", &omitModuleX{omit: false}); err != nil {
		t.Fatalf("RegisterModule omit_off: %v", err)
	}
	if err := vtab.RegisterModule(nil, "omit_on", &omitModuleX{omit: true}); err != nil {
		t.Fatalf("RegisterModule omit_on: %v", err)
	}
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt_off USING omit_off(val)`); err != nil {
		t.Fatalf("create vt_off: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE vt_on USING omit_on(val)`); err != nil {
		t.Fatalf("create vt_on: %v", err)
	}

	// omit=false: SQLite should re-check WHERE and filter down to 1 row.
	rows, err := db.Query(`SELECT val FROM vt_off WHERE val = 'alpha'`)
	if err != nil {
		t.Fatalf("query vt_off: %v", err)
	}
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	rows.Close()
	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("omit=false expected [alpha], got %v", got)
	}

	// omit=true: SQLite should not re-check WHERE; both rows would pass unless the vtab filters.
	rows, err = db.Query(`SELECT val FROM vt_on WHERE val = 'alpha'`)
	if err != nil {
		t.Fatalf("query vt_on: %v", err)
	}
	got = got[:0]
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	rows.Close()
	if len(got) != 2 {
		t.Fatalf("omit=true expected 2 rows (no re-check), got %d %v", len(got), got)
	}
}

// TestVtabMatchConstraint ensures MATCH constraints work when enabled.
func TestVtabMatchConstraint(t *testing.T) {
	if err := vtab.RegisterModule(nil, "matchx", &matchModuleX{}); err != nil {
		t.Fatalf("vtab.RegisterModule(matchx) failed: %v", err)
	}
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE VIRTUAL TABLE mt USING matchx(val)`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE mt USING matchx failed: %v", err)
	}

	rows, err := db.Query(`SELECT val FROM mt WHERE val MATCH 'al' ORDER BY val`)
	if err != nil {
		t.Fatalf("SELECT from mt failed: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "alpine" {
		t.Fatalf("unexpected MATCH results: %v", got)
	}
}

// TestVtabConstraintOperators verifies that at least one non-EQ operator is
// faithfully mapped (IS NULL) to the Go ConstraintOp.
func TestVtabConstraintOperators(t *testing.T) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "ops", &opsModuleX{}); err != nil {
		t.Fatalf("register ops: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE ovt USING ops(c1)`); err != nil {
		t.Fatalf("create ovt: %v", err)
	}

	rows, err := db.Query(`SELECT rowid FROM ovt WHERE c1 IS NULL`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows.Close()
	// Expect to see an ISNULL op recorded.
	found := false
	for _, op := range seenOpsOps {
		if op == vtab.OpISNULL {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to see OpISNULL in constraints, got %v", seenOpsOps)
	}

	// Also verify LIKE maps through when present.
	rows, err = db.Query(`SELECT rowid FROM ovt WHERE c1 LIKE 'a%'`)
	if err != nil {
		t.Fatalf("query like: %v", err)
	}
	rows.Close()
	found = false
	for _, op := range seenOpsOps {
		if op == vtab.OpLIKE {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to see OpLIKE in constraints, got %v", seenOpsOps)
	}

	// And verify GLOB maps through when present.
	rows, err = db.Query(`SELECT rowid FROM ovt WHERE c1 GLOB 'a*'`)
	if err != nil {
		t.Fatalf("query glob: %v", err)
	}
	rows.Close()
	found = false
	for _, op := range seenOpsOps {
		if op == vtab.OpGLOB {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to see OpGLOB in constraints, got %v", seenOpsOps)
	}
}

// overflowIdxModule sets an out-of-range IdxNum to verify the driver rejects
// values that do not fit into SQLite's int32 idxNum.
type overflowIdxModule struct{}
type overflowIdxTable struct{}
type overflowIdxCursor struct{}

func (m *overflowIdxModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("overflowIdx: missing table name")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &overflowIdxTable{}, nil
}
func (m *overflowIdxModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	return m.Create(ctx, args)
}
func (t *overflowIdxTable) BestIndex(info *vtab.IndexInfo) error {
	// Force IdxNum to exceed int32 to trigger trampoline guard.
	info.IdxNum = int64(math.MaxInt32) + 1
	return nil
}
func (t *overflowIdxTable) Open() (vtab.Cursor, error) { return &overflowIdxCursor{}, nil }
func (t *overflowIdxTable) Disconnect() error          { return nil }
func (t *overflowIdxTable) Destroy() error             { return nil }
func (c *overflowIdxCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	return nil
}
func (c *overflowIdxCursor) Next() error                    { return nil }
func (c *overflowIdxCursor) Eof() bool                      { return true }
func (c *overflowIdxCursor) Column(int) (vtab.Value, error) { return nil, nil }
func (c *overflowIdxCursor) Rowid() (int64, error)          { return 0, nil }
func (c *overflowIdxCursor) Close() error                   { return nil }

// badcolModule returns an unsupported type from Column to ensure the error
// text from functionReturnValue is propagated via sqlite3_result_error.
type badcolModule struct{}
type badcolTable struct{}
type badcolCursor struct{ pos int }

func (m *badcolModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("badcol: missing table name")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &badcolTable{}, nil
}
func (m *badcolModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	return m.Create(ctx, args)
}
func (t *badcolTable) BestIndex(info *vtab.IndexInfo) error { return nil }
func (t *badcolTable) Open() (vtab.Cursor, error)           { return &badcolCursor{pos: 0}, nil }
func (t *badcolTable) Disconnect() error                    { return nil }
func (t *badcolTable) Destroy() error                       { return nil }
func (c *badcolCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	c.pos = 0
	return nil
}
func (c *badcolCursor) Next() error {
	if c.pos < 1 {
		c.pos++
	}
	return nil
}
func (c *badcolCursor) Eof() bool { return c.pos >= 1 }
func (c *badcolCursor) Column(col int) (vtab.Value, error) {
	// Return a value of an unsupported type to provoke functionReturnValue error.
	type unsupported struct{}
	return unsupported{}, nil
}
func (c *badcolCursor) Rowid() (int64, error) { return 1, nil }
func (c *badcolCursor) Close() error          { return nil }

func TestVtabIdxNumOverflowError(t *testing.T) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "overflow_idx", &overflowIdxModule{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE ovt USING overflow_idx(val)`); err != nil {
		t.Fatalf("create vt: %v", err)
	}

	// Any SELECT should invoke BestIndex and fail due to IdxNum overflow.
	_, err = db.Query(`SELECT val FROM ovt`)
	if err == nil {
		t.Fatalf("expected SELECT to fail due to IdxNum overflow")
	}
	if msg := err.Error(); !strings.Contains(msg, "IdxNum") || !strings.Contains(msg, "int32") {
		t.Fatalf("unexpected error: %v", msg)
	}
}

func TestVtabColumnUnsupportedValueErrorMessage(t *testing.T) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "badcol", &badcolModule{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE bc USING badcol(val)`); err != nil {
		t.Fatalf("create vt: %v", err)
	}

	// Run a query and ensure it fails with a descriptive message from xColumn.
	rows, err := db.Query(`SELECT val FROM bc`)
	if err != nil {
		// Prepare-time error would also be acceptable, but we expect run-time here.
		// Ensure message mentions unsupported driver.Value.
		if !strings.Contains(err.Error(), "did not return a valid driver.Value") {
			t.Fatalf("unexpected error from Query: %v", err)
		}
		return
	}
	defer rows.Close()

	// Iterate to trigger stepping/column retrieval; expect rows.Err() to contain our message.
	for rows.Next() {
		var v any
		_ = rows.Scan(&v)
	}
	if err := rows.Err(); err == nil {
		t.Fatalf("expected rows.Err to report unsupported value")
	} else if !strings.Contains(err.Error(), "did not return a valid driver.Value") {
		t.Fatalf("unexpected rows.Err: %v", err)
	}
}

// Updater demo: in-memory table with (name, email) columns and rowid.
type updRow struct {
	id  int64
	val string
}

type updaterModuleX struct{}
type updaterTableX struct {
	rows   []updRow
	nextID int64
}
type updaterCursorX struct {
	t   *updaterTableX
	pos int
}

func (m *updaterModuleX) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("upd: missing table name")
	}
	if err := ctx.Declare(fmt.Sprintf("CREATE TABLE %s(val)", args[2])); err != nil {
		return nil, err
	}
	return &updaterTableX{rows: nil, nextID: 1}, nil
}
func (m *updaterModuleX) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	return m.Create(ctx, args)
}
func (t *updaterTableX) BestIndex(info *vtab.IndexInfo) error { return nil }
func (t *updaterTableX) Open() (vtab.Cursor, error)           { return &updaterCursorX{t: t, pos: 0}, nil }
func (t *updaterTableX) Disconnect() error                    { return nil }
func (t *updaterTableX) Destroy() error                       { return nil }

// Updater methods
func (t *updaterTableX) Insert(cols []vtab.Value, rowid *int64) error {
	val, _ := cols[0].(string)
	id := *rowid
	if id == 0 {
		id = t.nextID
	}
	t.nextID = id + 1
	t.rows = append(t.rows, updRow{id: id, val: val})
	*rowid = id
	return nil
}
func (t *updaterTableX) Update(oldRowid int64, cols []vtab.Value, newRowid *int64) error {
	for i := range t.rows {
		if t.rows[i].id == oldRowid {
			val, _ := cols[0].(string)
			t.rows[i].val = val
			if newRowid != nil && *newRowid != 0 && *newRowid != oldRowid {
				t.rows[i].id = *newRowid
			}
			return nil
		}
	}
	return fmt.Errorf("row %d not found", oldRowid)
}
func (t *updaterTableX) Delete(oldRowid int64) error {
	for i := range t.rows {
		if t.rows[i].id == oldRowid {
			t.rows = append(t.rows[:i], t.rows[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("row %d not found", oldRowid)
}

func (c *updaterCursorX) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	c.pos = 0
	return nil
}
func (c *updaterCursorX) Next() error {
	if c.pos < len(c.t.rows) {
		c.pos++
	}
	return nil
}
func (c *updaterCursorX) Eof() bool { return c.pos >= len(c.t.rows) }
func (c *updaterCursorX) Column(col int) (vtab.Value, error) {
	if c.pos >= len(c.t.rows) {
		return nil, nil
	}
	if col == 0 {
		return c.t.rows[c.pos].val, nil
	}
	return nil, nil
}
func (c *updaterCursorX) Rowid() (int64, error) { return c.t.rows[c.pos].id, nil }
func (c *updaterCursorX) Close() error          { return nil }

func TestVtabUpdaterInsertUpdateDelete(t *testing.T) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := vtab.RegisterModule(db, "updemo", &updaterModuleX{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE ut USING updemo(name,email)`); err != nil {
		t.Fatalf("create vt: %v", err)
	}

	// Insert Alice and Bob (auto rowid)
	if _, err := db.Exec(`INSERT INTO ut(val) VALUES(?)`, "Alice"); err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ut(val) VALUES(?)`, "Bob"); err != nil {
		t.Fatalf("insert bob: %v", err)
	}

	// Insert Carol (auto rowid)
	if _, err := db.Exec(`INSERT INTO ut(val) VALUES(?)`, "Carol"); err != nil {
		t.Fatalf("insert carol: %v", err)
	}

	// Verify rows
	assertRows := func(want []int64) {
		rows, err := db.Query(`SELECT rowid FROM ut ORDER BY rowid`)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		defer rows.Close()
		got := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows.Err: %v", err)
		}
		if len(got) != len(want) {
			t.Fatalf("got %d rows, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ids mismatch got %v want %v", got, want)
			}
		}
	}

	assertRows([]int64{1, 2, 3})

	// Update Bob's email (rowid=2)
	if _, err := db.Exec(`UPDATE ut SET val = ? WHERE rowid = ?`, "Bobby", 2); err != nil {
		t.Fatalf("update: %v", err)
	}
	// Rowids remain unchanged after value update
	assertRows([]int64{1, 2, 3})

	// Delete Bob (rowid=2)
	if _, err := db.Exec(`DELETE FROM ut WHERE rowid = ?`, 2); err != nil {
		t.Fatalf("delete: %v", err)
	}

	assertRows([]int64{1, 3})
}
