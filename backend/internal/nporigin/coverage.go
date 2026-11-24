package nporigin

// Coverage counts what a table encodes (D763).
type Coverage struct {
	Chapters int `json:"chapters"`
	Rows     int `json:"rows"`
	// RowsByCode: every alternative of the row is decided from codes (and
	// values). RowsWithProcess: at least one alternative needs a yes/no
	// answer about the work done. RowsUnread: at least one alternative the
	// parser could not read (treated as a process rule).
	RowsByCode      int `json:"rows_by_code"`
	RowsWithProcess int `json:"rows_with_process"`
	RowsUnread      int `json:"rows_unread"`
	Alternatives    int `json:"alternatives"`
	// ByCode: change, change-from and value alternatives encoded. Process:
	// process rules (answer only). Unread: alternatives not encoded.
	ByCode  int `json:"by_code"`
	Process int `json:"process"`
	Unread  int `json:"unread"`
}

// Unread is one alternative the parser could not read.
type Unread struct {
	Row    string `json:"row"`
	Key    string `json:"key"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// CoverageOf counts the rows and alternatives of t and lists every
// alternative not read.
func CoverageOf(t Table) (Coverage, []Unread) {
	var c Coverage
	var un []Unread
	for _, ch := range t.Chapters {
		c.Chapters++
		for _, r := range ch.Rows {
			c.Rows++
			code, proc, bad := 0, 0, 0
			for _, a := range r.Alts {
				c.Alternatives++
				switch {
				case !a.Encoded:
					bad++
					c.Unread++
					un = append(un, Unread{Row: r.Code, Key: a.Key, Text: a.Text, Reason: a.Reason})
				case a.Kind == KindProcess:
					proc++
					c.Process++
				default:
					code++
					c.ByCode++
				}
			}
			switch {
			case bad > 0:
				c.RowsUnread++
			case proc > 0:
				c.RowsWithProcess++
			default:
				c.RowsByCode++
			}
		}
	}
	return c, un
}
