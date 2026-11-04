package graph

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

type commit struct {
	hash    string
	parents []string
}

func layOut(commits []commit) ([]Row, *Layout) {
	l := &Layout{}
	rows := make([]Row, len(commits))
	for i, c := range commits {
		rows[i] = l.Add(c.hash, c.parents)
	}
	return rows, l
}

func TestStraightLine(t *testing.T) {
	rows, l := layOut([]commit{{"c", []string{"b"}}, {"b", []string{"a"}}, {"a", nil}})
	if l.Width() != 1 {
		t.Errorf("width %d", l.Width())
	}
	if len(rows[0].Top) != 0 {
		t.Errorf("a line comes into the tip: %v", rows[0].Top)
	}
	if want := []Edge{{0, 0, 0}}; !slices.Equal(rows[1].Top, want) || !slices.Equal(rows[1].Bottom, want) {
		t.Errorf("middle %+v", rows[1])
	}
	if len(rows[2].Bottom) != 0 {
		t.Errorf("a line leaves the root: %v", rows[2].Bottom)
	}
}

func TestBranchAndMerge(t *testing.T) {
	// m merges s into o, which both come from a.
	rows, l := layOut([]commit{
		{"m", []string{"o", "s"}},
		{"o", []string{"a"}},
		{"s", []string{"a"}},
		{"a", nil},
	})
	if l.Width() != 2 {
		t.Errorf("width %d", l.Width())
	}
	if want := []Edge{{0, 0, 0}, {0, 1, 1}}; !slices.Equal(rows[0].Bottom, want) {
		t.Errorf("merge %v", rows[0].Bottom)
	}
	if rows[1].Lane != 0 || rows[2].Lane != 1 || rows[3].Lane != 0 {
		t.Errorf("lanes %d %d %d", rows[1].Lane, rows[2].Lane, rows[3].Lane)
	}
	// The side branch's last line joins the lane already waiting for a, keeping
	// its own colour.
	if want := []Edge{{0, 0, 0}, {1, 0, 1}}; !slices.Equal(rows[2].Bottom, want) {
		t.Errorf("join %v", rows[2].Bottom)
	}
	if want := []Edge{{0, 0, 0}}; !slices.Equal(rows[3].Top, want) {
		t.Errorf("into the root %v", rows[3].Top)
	}
}

func TestSeparateTipsTakeFreeLanes(t *testing.T) {
	rows, _ := layOut([]commit{
		{"t1", []string{"a"}},
		{"t2", []string{"b"}},
		{"a", nil},
		{"t3", []string{"b"}},
		{"b", nil},
	})
	// Lane 0 is free again by the third tip.
	if rows[1].Lane != 1 || rows[3].Lane != 0 {
		t.Errorf("lanes %d, %d", rows[1].Lane, rows[3].Lane)
	}
	// The third tip keeps to its own lane rather than stepping right to the
	// lane already waiting for b, and the two meet at b, in the leftmost.
	if want := (Edge{0, 0, rows[3].Colour}); !slices.Contains(rows[3].Bottom, want) {
		t.Errorf("from the third tip %v", rows[3].Bottom)
	}
	if rows[4].Lane != 0 || len(rows[4].Top) != 2 {
		t.Errorf("into b in lane %d: %v", rows[4].Lane, rows[4].Top)
	}
}

// TestDoesNotWanderRight checks that a line of merges, each with its side
// branch listed first as git log --topo-order does, stays at the left. The
// main line used to step right to the side branch's lane at every merge,
// making the graph ever wider.
func TestDoesNotWanderRight(t *testing.T) {
	var commits []commit
	for i := range 50 {
		next := fmt.Sprint("m", i+1)
		commits = append(commits,
			commit{fmt.Sprint("m", i), []string{fmt.Sprint("c", i), fmt.Sprint("s", i)}},
			commit{fmt.Sprint("s", i), []string{next}},
			commit{fmt.Sprint("c", i), []string{next}},
		)
	}
	rows, l := layOut(commits)
	if l.Width() != 2 {
		t.Errorf("width %d", l.Width())
	}
	for i := 0; i < len(rows); i += 3 {
		if rows[i].Lane != 0 {
			t.Fatalf("merge %d in lane %d", i/3, rows[i].Lane)
		}
	}
}

// randomHistory returns commits newest first, each with parents among those
// older than it.
func randomHistory(random *rand.Rand, size int) []commit {
	commits := make([]commit, size)
	for i := range commits {
		c := commit{hash: fmt.Sprint(i)}
		for range random.Intn(4) {
			if i+1 < size {
				parent := fmt.Sprint(i + 1 + random.Intn(min(size-i-1, 8)))
				if !slices.Contains(c.parents, parent) {
					c.parents = append(c.parents, parent)
				}
			}
		}
		commits[i] = c
	}
	return commits
}

// TestRowsJoinUp checks on random histories that nothing is drawn with a gap:
// what leaves the bottom of a row is what enters the top of the next, colours
// included, and every edge touches the node's lane or passes straight.
func TestRowsJoinUp(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	for range 200 {
		commits := randomHistory(random, 40)
		rows, l := layOut(commits)
		for i, row := range rows {
			if row.Lane >= l.Width() {
				t.Fatalf("row %d: lane %d of %d", i, row.Lane, l.Width())
			}
			for _, e := range row.Top {
				if e.From != e.To && e.To != row.Lane {
					t.Fatalf("row %d: stray edge %v", i, e)
				}
			}
			for _, e := range row.Bottom {
				if e.From != e.To && e.From != row.Lane {
					t.Fatalf("row %d: stray edge %v", i, e)
				}
			}
			if i == 0 {
				continue
			}
			// A line joining a lane that goes on keeps its own colour.
			arriving := map[int]int{}
			for _, e := range rows[i-1].Bottom {
				if _, ok := arriving[e.To]; !ok || e.From == e.To {
					arriving[e.To] = e.Colour
				}
			}
			leaving := map[int]int{}
			for _, e := range row.Top {
				leaving[e.From] = e.Colour
			}
			if fmt.Sprint(arriving) != fmt.Sprint(leaving) {
				t.Fatalf("row %d: above %v, below %v\n%+v", i, arriving, leaving, commits)
			}
		}
	}
}

// TestPagesMatchAllAtOnce checks that a layout carries on across pages, as a
// layout made afresh for each page would not.
func TestPagesMatchAllAtOnce(t *testing.T) {
	commits := randomHistory(rand.New(rand.NewSource(2)), 60)
	whole, _ := layOut(commits)
	first, l := layOut(commits[:30])
	rows := first
	for _, c := range commits[30:] {
		rows = append(rows, l.Add(c.hash, c.parents))
	}
	if fmt.Sprint(rows) != fmt.Sprint(whole) {
		t.Errorf("got %+v\nwant %+v", rows, whole)
	}
}

// A clone goes on as the layout would, from where it had got to, and leaves the
// layout itself as it was, as the history lays out its next page in the
// background in a clone while the layout is in use.
func TestClone(t *testing.T) {
	commits := []commit{
		{"e", []string{"d", "c"}}, {"d", []string{"b"}}, {"c", []string{"b"}}, {"b", []string{"a"}}, {"a", nil},
	}
	whole, _ := layOut(commits)
	l := &Layout{}
	for _, c := range commits[:2] {
		l.Add(c.hash, c.parents)
	}
	clone := l.Clone()
	for i, c := range commits[2:] {
		if row := clone.Add(c.hash, c.parents); !rowsEqual(row, whole[i+2]) {
			t.Errorf("row %d of the clone %+v, not %+v", i+2, row, whole[i+2])
		}
	}
	// The layout itself goes on from where the clone started.
	for i, c := range commits[2:] {
		if row := l.Add(c.hash, c.parents); !rowsEqual(row, whole[i+2]) {
			t.Errorf("row %d of the layout %+v, not %+v", i+2, row, whole[i+2])
		}
	}
}

func rowsEqual(a, b Row) bool {
	return a.Lane == b.Lane && a.Colour == b.Colour && slices.Equal(a.Top, b.Top) && slices.Equal(a.Bottom, b.Bottom)
}
