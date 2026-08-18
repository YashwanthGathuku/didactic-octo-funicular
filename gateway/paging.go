package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Server-side paging for the operations UI.
//
// Keyset paging, not OFFSET. OFFSET is the obvious choice and it is wrong for
// these lists: rows arrive at the head of every one of them while an operator
// is reading, so page two computed by OFFSET repeats rows page one already
// showed, and a row that moved across the boundary is skipped entirely. For an
// evidence timeline that is not a cosmetic defect -- it is a reader who was
// shown a complete-looking list with a gap in it.
//
// The cursor is therefore a position in the ordering, not a count of rows
// skipped. Everything paged here is ordered by a monotonic primary key
// descending, so the cursor is the last id the client received and the next
// page is "strictly older than that".

const (
	// defaultPageLimit is what a client that names no limit receives.
	defaultPageLimit = 50

	// maxPageLimit is the ceiling, and it is enforced rather than negotiated: a
	// client asking for 100000 rows gets 200. Trusting the client's limit is
	// how a list endpoint becomes a way to exhaust the server's memory with a
	// single authenticated request, and how a browser tab ends up holding an
	// entire tenant's history.
	maxPageLimit = 200
)

// cursorPrefix versions the cursor encoding.
//
// The cursor is opaque by convention -- base64 of a versioned string -- so that
// what it encodes can change without breaking clients that stored one. It is
// not opaque as a *control*: anyone can decode it, so it is validated on the
// way back in and it is never trusted to carry authorization. A cursor names a
// position; the tenant filter still comes from the verified principal.
const cursorPrefix = "sfp1:"

type pageRequest struct {
	Limit int
	// After is exclusive: the next page begins strictly beyond it. Zero means
	// the beginning of the list.
	After int64

	// AfterSort is the leading component for lists that are not ordered by id.
	//
	// The SLA board is ordered by when a file is due, which is not unique --
	// two feeds can share a deadline to the second. A cursor of the deadline
	// alone would either skip the second feed or repeat the first, so the
	// cursor carries the deadline *and* the row id, and the comparison is
	// lexicographic over the pair. Ordering by a non-unique key alone is the
	// most common way keyset paging is got wrong.
	AfterSort int64
	// Composite says the cursor carried both components, so a zero AfterSort
	// (the Unix epoch) is distinguishable from "no cursor".
	Composite bool
}

// parsePage reads limit and cursor from the query string.
//
// A malformed cursor is an error rather than a silent reset to the first page.
// Silently restarting would show an operator the head of the list again while
// they believed they were advancing through it, which is the same class of
// defect as a skipped row.
func parsePage(r *http.Request) (pageRequest, error) {
	p := pageRequest{Limit: defaultPageLimit}

	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return p, fmt.Errorf("limit must be a positive integer")
		}
		p.Limit = min(n, maxPageLimit)
	}

	raw := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if raw == "" {
		return p, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return p, fmt.Errorf("cursor is not a cursor this server issued")
	}
	s, ok := strings.CutPrefix(string(decoded), cursorPrefix)
	if !ok {
		return p, fmt.Errorf("cursor is not a cursor this server issued")
	}
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 1:
		after, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || after < 0 {
			return p, fmt.Errorf("cursor is not a cursor this server issued")
		}
		p.After = after
	case 2:
		sortKey, err1 := strconv.ParseInt(parts[0], 10, 64)
		id, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil || id < 0 {
			return p, fmt.Errorf("cursor is not a cursor this server issued")
		}
		p.AfterSort, p.After, p.Composite = sortKey, id, true
	default:
		return p, fmt.Errorf("cursor is not a cursor this server issued")
	}
	return p, nil
}

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(cursorPrefix + strconv.FormatInt(id, 10)))
}

func encodeCompositeCursor(sortKey, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix +
		strconv.FormatInt(sortKey, 10) + "," + strconv.FormatInt(id, 10)))
}

// page is the envelope every paged list returns.
//
// hasMore is derived by asking for one row more than the limit and discarding
// it, not by counting the table. A COUNT(*) on every page is both a second scan
// and a lie by the time it is rendered.
type page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
	Limit      int    `json:"limit"`

	// Partial says the server could not assemble every field of every row. The
	// UI renders that as a distinct state: a list with a row missing its
	// findings is not the same thing as a list with no findings, and the
	// operator is the one who has to know which they are looking at.
	Partial       bool   `json:"partial,omitempty"`
	PartialReason string `json:"partialReason,omitempty"`
}

// finish trims the sentinel row and sets the cursor.
//
// Callers query limit+1 rows and pass the whole slice here with the id of the
// last *kept* row supplied by idOf.
func finish[T any](items []T, req pageRequest, idOf func(T) int64) page[T] {
	out := page[T]{Limit: req.Limit, Items: items}
	if len(items) > req.Limit {
		out.Items = items[:req.Limit]
		out.HasMore = true
	}
	if out.Items == nil {
		out.Items = []T{}
	}
	if out.HasMore && len(out.Items) > 0 {
		out.NextCursor = encodeCursor(idOf(out.Items[len(out.Items)-1]))
	}
	return out
}

// finishComposite is finish for a list ordered by a non-unique sort key paired
// with the row id.
func finishComposite[T any](items []T, req pageRequest, keyOf func(T) (int64, int64)) page[T] {
	out := page[T]{Limit: req.Limit, Items: items}
	if len(items) > req.Limit {
		out.Items = items[:req.Limit]
		out.HasMore = true
	}
	if out.Items == nil {
		out.Items = []T{}
	}
	if out.HasMore && len(out.Items) > 0 {
		out.NextCursor = encodeCompositeCursor(keyOf(out.Items[len(out.Items)-1]))
	}
	return out
}
