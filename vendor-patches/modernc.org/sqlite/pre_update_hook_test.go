package sqlite_test

import (
	"database/sql"
	"slices"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestPreUpdateHook(t *testing.T) {
	connStr := ":memory:"
	driverName := "sqlite_pre_update_hook_test"

	var (
		insertCount     int
		insertNewValues []any
		updateCount     int
		updateOldValues []any
		updateNewValues []any
		deleteCount     int
		deleteOldValues []any
		commitCount     int
		rollbackCount   int
	)
	var testDriver sqlite.Driver
	testDriver.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
		if hooker, ok := conn.(sqlite.HookRegisterer); ok {
			hooker.RegisterPreUpdateHook(func(data sqlite.SQLitePreUpdateData) {
				switch data.Op {
				case sqlite3.SQLITE_INSERT:
					insertCount++
					insertNewValues = make([]any, data.Count())
					err := data.New(insertNewValues...)
					if err != nil {
						t.Fatal(err)
					}
				case sqlite3.SQLITE_UPDATE:
					updateCount++
					updateOldValues = make([]any, data.Count())
					err := data.Old(updateOldValues...)
					if err != nil {
						t.Fatal(err)
					}
					updateNewValues = make([]any, data.Count())
					err = data.New(updateNewValues...)
					if err != nil {
						t.Fatal(err)
					}
				case sqlite3.SQLITE_DELETE:
					deleteCount++
					deleteOldValues = make([]any, data.Count())
					err := data.Old(deleteOldValues...)
					if err != nil {
						t.Fatal(err)
					}
				}
			})
			hooker.RegisterCommitHook(func() int32 {
				commitCount++
				return 0
			})
			hooker.RegisterRollbackHook(func() {
				rollbackCount++
			})
		}
		return nil
	})

	sql.Register(driverName, &testDriver)

	db, err := sql.Open(driverName, connStr)
	if err != nil {
		t.Fatal(err)
	}

	expectInsertValues := []any{int64(42), 3.1415, "Test", "will be nil"}
	expectUpdateValues := []any{int64(43), 1.5, "Test update", nil}
	_, err = db.Exec(`
	CREATE TABLE in_memory_test(id INTEGER PRIMARY KEY, f FLOAT, t TEXT, x ANY);
	INSERT INTO in_memory_test VALUES(42, 3.1415, 'Test', 'will be nil');
	UPDATE in_memory_test SET id = 43, f = 1.5, t = 'Test update', x = null;
	DELETE FROM in_memory_test;
	SELECT 1;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if insertCount != 1 {
		t.Errorf("pre update hook: expect %d inserts call, got %d", 1, insertCount)
	}
	if !slices.Equal(insertNewValues, expectInsertValues) {
		t.Errorf("pre update hook: expect %v as inserted new values, got %v", expectInsertValues, insertNewValues)
	}
	if updateCount != 1 {
		t.Errorf("pre update hook: expect %d updates call, got %d", 1, updateCount)
	}
	if !slices.Equal(updateOldValues, expectInsertValues) {
		t.Errorf("pre update hook: expect %v as updated old values, got %v", expectInsertValues, updateOldValues)
	}
	if !slices.Equal(updateNewValues, expectUpdateValues) {
		t.Errorf("pre update hook: expect %v as updated new values, got %v", expectUpdateValues, updateNewValues)
	}
	if deleteCount != 1 {
		t.Errorf("pre update hook: expect %d deletes call, got %d", 1, deleteCount)
	}
	if !slices.Equal(deleteOldValues, expectUpdateValues) {
		t.Errorf("pre update hook: expect %v as deleted old values, got %v", expectUpdateValues, deleteOldValues)
	}
	if commitCount != 4 {
		t.Errorf("commit hook: expect %d, got %d", 4, commitCount)
	}
	if rollbackCount != 0 {
		t.Errorf("rollback hook: expect %d, got %d", 0, rollbackCount)
	}

}
