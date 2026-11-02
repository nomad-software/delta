// Package graph lays out the history of commits as lanes, for drawing as a tree
// beside a list of them. It knows nothing of the UI.
//
// Each commit is a row, cut in two at its centre, where its node is. An edge in
// the top half runs from a lane at the top of the row to a lane at the centre,
// and one in the bottom half from a lane at the centre to a lane at the bottom.
// The lanes at the bottom of a row are those at the top of the next.
package graph

// Edge is a line across half of a row, between two lanes. Colour numbers the
// branch it is part of, and is not limited: the UI picks from its own colours.
type Edge struct {
	From, To int
	Colour   int
}

// Row is how one commit is drawn.
type Row struct {
	// Lane is where the node is, in the colour numbered Colour.
	Lane   int
	Colour int
	// Top and Bottom are the edges of each half of the row.
	Top, Bottom []Edge
}

// lane is a lane going down from a row: the commit it is waiting to reach, or
// nothing if it is free, and its colour.
type lane struct {
	hash   string
	colour int
}

// Layout works out the rows of commits, given newest first and with every commit
// before its parents, as git log --topo-order has them. It can be given them a
// page at a time.
type Layout struct {
	lanes   []lane
	colours int
	// width is the most lanes any row has needed.
	width int
}

// Width returns the number of lanes needed to draw every row so far.
func (l *Layout) Width() int {
	return l.width
}

// Add returns the row of the next commit.
func (l *Layout) Add(hash string, parents []string) Row {
	column := l.find(hash)
	isTip := column < 0
	if isTip {
		column = l.free(0)
		l.lanes[column] = lane{hash: hash, colour: l.newColour()}
	}
	row := Row{Lane: column, Colour: l.lanes[column].colour}

	// Whatever was waiting for this commit comes in to the node, and the rest
	// pass by. A branch tip has nothing coming in.
	for j, in := range l.lanes {
		switch {
		case in.hash == "":
		case in.hash != hash:
			row.Top = append(row.Top, Edge{j, j, in.colour})
		case j != column || !isTip:
			row.Top = append(row.Top, Edge{j, column, in.colour})
			l.lanes[j].hash = ""
		default:
			l.lanes[j].hash = ""
		}
	}
	for j, passing := range l.lanes {
		if passing.hash != "" {
			row.Bottom = append(row.Bottom, Edge{j, j, passing.colour})
		}
	}

	// The first parent carries on down this lane if nothing is already waiting
	// for it, and the others take a lane of their own, each as a new branch.
	for i, parent := range parents {
		target := l.find(parent)
		switch {
		case target >= 0:
		case i == 0:
			target = column
			l.lanes[column] = lane{hash: parent, colour: row.Colour}
		default:
			target = l.free(column)
			l.lanes[target] = lane{hash: parent, colour: l.newColour()}
		}
		// A branch keeps its own colour up to where it joins another. What
		// a merge brings in is in the colour of what it came from.
		colour := l.lanes[target].colour
		if i == 0 {
			colour = row.Colour
		}
		row.Bottom = append(row.Bottom, Edge{column, target, colour})
	}

	l.width = max(l.width, column+1, len(l.lanes))
	for len(l.lanes) > 0 && l.lanes[len(l.lanes)-1].hash == "" {
		l.lanes = l.lanes[:len(l.lanes)-1]
	}
	return row
}

// find returns the lane waiting for the commit, or -1.
func (l *Layout) find(hash string) int {
	for j, waiting := range l.lanes {
		if waiting.hash == hash {
			return j
		}
	}
	return -1
}

// free returns the first free lane from the one given, making one if need be.
func (l *Layout) free(from int) int {
	for len(l.lanes) <= from {
		l.lanes = append(l.lanes, lane{})
	}
	for j := from; j < len(l.lanes); j++ {
		if l.lanes[j].hash == "" {
			return j
		}
	}
	l.lanes = append(l.lanes, lane{})
	return len(l.lanes) - 1
}

// newColour returns the number of the next colour.
func (l *Layout) newColour() int {
	l.colours++
	return l.colours - 1
}
