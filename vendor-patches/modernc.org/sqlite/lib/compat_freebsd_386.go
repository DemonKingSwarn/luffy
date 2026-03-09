// Copyright 2025 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd && 386

// compat_freebsd_386.go provides type aliases to bridge the naming convention
// difference between the old ccgo-generated sqlite_freebsd_386.go (which lacks
// the T prefix on struct type names) and the rest of the sqlite package which
// expects the new T-prefixed names introduced in ccgo v4.

package sqlite3

// Type aliases mapping new T-prefixed names to the old generated names.
type Tsqlite3_index_constraint = sqlite3_index_constraint
type Tsqlite3_index_orderby = sqlite3_index_orderby
type Tsqlite3_index_constraint_usage = sqlite3_index_constraint_usage
