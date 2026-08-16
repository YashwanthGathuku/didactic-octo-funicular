package main

import "strings"

// SQLite connection settings.
//
// The gateway opened `sql.Open("sqlite", cfg.DatabaseURL)` with no pragmas at
// all. Under concurrency that produces `database is locked` errors the
// application has no way to handle, and the failure surfaced while building the
// evidence ledger: twenty-four concurrent appends failed outright rather than
// queueing.
//
// Three settings fix it, and each addresses a different cause:
//
//   - busy_timeout makes a writer wait for the write lock instead of failing
//     immediately. Without it, any contention at all is an error.
//
//   - journal_mode=WAL lets readers proceed while a writer holds the lock. In
//     the default rollback journal a single writer blocks every reader, so a
//     validation worker stalls every API request.
//
// A fourth setting was tried and deliberately rejected: `_txlock=immediate`,
// which begins every transaction by taking the write lock.
//
// It fixes the ledger's specific problem. A deferred transaction that reads and
// then writes must upgrade its lock, and SQLite refuses to *wait* on an
// upgrade -- it returns SQLITE_BUSY immediately, because waiting could deadlock
// two transactions each holding a read lock. The ledger append is exactly that
// shape, and immediate mode removes the upgrade.
//
// It also breaks the worker pool. A job handler's transaction stays open for
// the whole handler, so in immediate mode one long-running handler holds the
// only write lock and every other worker blocks at BEGIN -- including workers
// that would have served a different tenant. The per-tenant concurrency test
// caught it: the quiet tenant's job never ran at all.
//
// The ledger's bounded retry with backoff handles the upgrade case instead,
// which is the narrower fix for the narrower problem.
//
// This is a SQLite deployment concern. PostgreSQL needs none of it.
const sqliteConnectionSettings = "_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

// sqliteDSN appends the connection settings to a database path.
//
// A path that already carries query parameters keeps them; the settings are
// added rather than replacing what an operator wrote, and a setting the
// operator already specified wins because SQLite applies the first occurrence.
func sqliteDSN(path string) string {
	if path == "" {
		return path
	}
	// ":memory:" is left alone. It is not a supported production configuration
	// and adding WAL to it is meaningless.
	if strings.HasPrefix(path, ":memory:") {
		return path
	}
	if strings.Contains(path, "?") {
		return path + "&" + sqliteConnectionSettings
	}
	return path + "?" + sqliteConnectionSettings
}
