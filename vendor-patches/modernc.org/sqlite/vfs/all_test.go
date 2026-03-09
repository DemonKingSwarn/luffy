// Copyright 2022 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vfs

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"modernc.org/sqlite"
)

func E(err error) string {
	if err == nil {
		return ""
	}
	if err, ok := err.(*sqlite.Error); ok {
		return sqlite.ErrorCodeString[err.Code()]
	}
	return err.Error()
}

func TestVFS(t *testing.T) {
	const dbname = "canary.db"

	tmpdbdir := t.TempDir()

	func() {
		db, err := sql.Open("sqlite", "file:"+filepath.Join(tmpdbdir, dbname))
		if err != nil {
			t.Fatalf("unexpected failure to open database, %s", err.Error())
		}
		defer db.Close()

		_, err = db.Exec("create table 'test' ('name' varchar(32) not null, primary key('name') )")
		if err != nil {
			t.Fatalf("unexpected create table error, %s", E(err))
		}

		_, err = db.Exec("insert into 'test' (name) values ('foobar')")
		if err != nil {
			t.Fatalf("unexpected insert error, %s", E(err))
		}
	}()

	vfsid, fs, err := New(os.DirFS(tmpdbdir))
	if err != nil {
		t.Fatalf("unexpected failure to register new vfs, %s", err.Error())
	}
	defer fs.Close()

	db, err := sql.Open("sqlite", "file:"+dbname+"?vfs="+vfsid)
	if err != nil {
		t.Fatalf("unexpected failure to open database, %s", err.Error())
	}
	defer db.Close()

	rows, err := db.Query("select * from 'test'")
	if err != nil {
		t.Fatalf("unexpected select error, %s", E(err))
	}
	defer rows.Close()
	var got string
	if !rows.Next() {
		t.Fatalf("unexpected empty select result")
	}
	err = rows.Scan(&got)
	if err != nil {
		t.Fatalf("unexpected scan error, %s", E(err))
	}
	want := "foobar"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
