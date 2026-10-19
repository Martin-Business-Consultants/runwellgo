package core

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Index lists sort by their column headers and come a page at a time, as the
// Rails core's did (TableSorting, paginate):
//
//	?sort=due&direction=desc   a column the list names, ascending unless desc
//	?page=2&per=25             a page; without per, the whole list (what
//	                           agents and the CLI read, as Rails' JSON did)
//
// Sorting works on the list's own answer: each sortable column is a path
// into an item ("client.display_name"), so a derived value sorts the same
// as a stored one. Empty values sort last whichever way, then by id, so
// pages never shuffle. The answer says where it is: page, per, total, pages,
// sort, direction.

// sortPaths are a list's sortable columns: key → where in an item its value
// is, and, for a column whose order isn't its value's (a status), the
// values in order.
type sortPaths map[string]sortPath

type sortPath struct {
	path  []string
	order []string
}

// by is a column read at path ("client", "display_name").
func by(path ...string) sortPath { return sortPath{path: path} }

// dig is the value at path in an item; nil when it isn't there.
func dig(v any, path ...string) any {
	for _, k := range path {
		switch o := v.(type) {
		case Obj:
			v = o.Get(k)
		case map[string]any:
			v = o[k]
		default:
			return nil
		}
	}
	return v
}

// sortList sorts list by the column in ?sort (?direction asc or desc) when
// it's one of cols; it answers the column and direction applied, "" when
// the list keeps its own order.
func (c *Ctx) sortList(list []Obj, cols sortPaths) (string, string) {
	key := c.Query("sort")
	col, ok := cols[key]
	if !ok {
		return "", ""
	}
	dir := "asc"
	if c.Query("direction") == "desc" {
		dir = "desc"
	}
	value := func(o Obj) any {
		v := dig(o, col.path...)
		if col.order != nil {
			if i := slices.Index(col.order, str(v)); i >= 0 {
				return float64(i)
			}
			return nil
		}
		return v
	}
	slices.SortStableFunc(list, func(a, b Obj) int {
		va, vb := value(a), value(b)
		ea, eb := empty(va), empty(vb)
		switch {
		case ea && eb:
		case ea:
			return 1
		case eb:
			return -1
		default:
			if n := compareValues(va, vb); n != 0 {
				if dir == "desc" {
					return -n
				}
				return n
			}
		}
		return compareValues(a.Get("id"), b.Get("id"))
	})
	return key, dir
}

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}

// compareValues orders two values of a column: numbers as numbers, times
// and dates as their text (ISO, so in order), words without case.
func compareValues(a, b any) int {
	if fa, ok := number(a); ok {
		if fb, ok := number(b); ok {
			switch {
			case fa < fb:
				return -1
			case fa > fb:
				return 1
			}
			return 0
		}
	}
	if ba, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			switch {
			case ba == bb:
				return 0
			case !ba:
				return -1
			}
			return 1
		}
	}
	sa, sb := textOf(a), textOf(b)
	return strings.Compare(strings.ToLower(sa), strings.ToLower(sb))
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if t, ok := v.(interface{ MarshalJSON() ([]byte, error) }); ok {
		raw, _ := t.MarshalJSON()
		return strings.Trim(string(raw), `"`)
	}
	return fmt.Sprint(v)
}

// pageOf cuts one page out of list when the request asks for pages (?per),
// and adds where it is to out: page, per, total, pages. Without ?per it's
// the whole list on one page.
func (c *Ctx) pageOf(list []Obj) ([]Obj, Obj) {
	total := len(list)
	per, err := strconv.Atoi(c.Query("per"))
	if err != nil || per <= 0 {
		return list, O("page", 1, "per", total, "total", total, "pages", 1)
	}
	per = min(per, 100)
	pages := max(1, int(math.Ceil(float64(total)/float64(per))))
	page, _ := strconv.Atoi(c.Query("page"))
	page = min(max(page, 1), pages)
	from := (page - 1) * per
	to := min(from+per, total)
	return list[from:to], O("page", page, "per", per, "total", total, "pages", pages)
}

// searchedKeys are where an item's words are, for ?q: its names and titles,
// and the names of what it belongs to.
var searchedKeys = [][]string{{"display_name"}, {"name"}, {"title"}, {"subject"}, {"description"}, {"ref"}, {"email"},
	{"sender_name"}, {"sender_email"}, {"owner_name"}, {"client", "display_name"}, {"engagement", "display_name"}, {"owner", "name"}}

// searched keeps the items matching ?q: every word in it, in any of an
// item's names (searchedKeys), without case.
func (c *Ctx) searched(list []Obj) []Obj {
	words := strings.Fields(strings.ToLower(c.Query("q")))
	if len(words) == 0 {
		return list
	}
	out := []Obj{}
	for _, o := range list {
		var text strings.Builder
		for _, k := range searchedKeys {
			if v, ok := dig(o, k...).(string); ok {
				text.WriteString(strings.ToLower(v))
				text.WriteByte(' ')
			}
		}
		all := true
		for _, w := range words {
			if !strings.Contains(text.String(), w) {
				all = false
				break
			}
		}
		if all {
			out = append(out, o)
		}
	}
	return out
}

// listAnswer searches (?q), sorts and pages list, then answers out with it
// under key and where the list is (pagination: page, per, total, pages;
// sort, direction).
func (c *Ctx) listAnswer(out Obj, key string, list []Obj, cols sortPaths) error {
	list = c.searched(list)
	sortKey, dir := c.sortList(list, cols)
	shown, where := c.pageOf(list)
	if shown == nil {
		shown = []Obj{}
	}
	where = where.Set("sort", nullable(sortKey)).Set("direction", nullable(dir))
	return c.JSON(200, out.Set(key, shown).Set("pagination", where))
}

// pagedAnswer searches (?q) and pages a list the database already sorted (?sort among
// sorted's keys), and answers out with it under key and where it is.
func (c *Ctx) pagedAnswer(out Obj, key string, list []Obj, sorted map[string]string) error {
	shown, where := c.pageOf(c.searched(list))
	if shown == nil {
		shown = []Obj{}
	}
	sortKey, dir := "", ""
	if _, ok := sorted[c.Query("sort")]; ok {
		sortKey, dir = c.Query("sort"), "asc"
		if c.Query("direction") == "desc" {
			dir = "desc"
		}
	}
	return c.JSON(200, out.Set(key, shown).Set("pagination", where.Set("sort", nullable(sortKey)).Set("direction", nullable(dir))))
}
