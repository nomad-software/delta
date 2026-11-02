package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/graph"
)

// The size of the graph, in pixels.
const (
	graphRow      = 24
	graphLane     = 14
	graphMargin   = 8
	graphLeast    = 40
	graphMost     = 320
	graphLine     = 2.0
	graphNode     = 4.5
	graphNodeRing = 1.5
)

// graphColours are the colours of the branches, which read on light and dark
// backgrounds alike, and are told apart at a glance.
var graphColours = []string{
	"#3b82f6", "#ef4444", "#22a55b", "#f59e0b",
	"#a855f7", "#06b6d4", "#ec4899", "#84a60f",
}

// graphColumn draws the graph in the column of the commits, row by row, which is
// what makes it scroll with them and take no time for rows out of sight.
type graphColumn struct {
	*qt.QStyledItemDelegate
	// rows are what to draw.
	rows []graph.Row
	// pens and brushes are of each of graphColours.
	pens    []*qt.QPen
	brushes []*qt.QBrush
}

func newGraphColumn(parent *qt.QObject) *graphColumn {
	g := &graphColumn{QStyledItemDelegate: qt.NewQStyledItemDelegate2(parent)}
	for _, name := range graphColours {
		colour := qt.NewQColor6(name)
		brush := qt.NewQBrush3(colour)
		colour.Delete()
		pen := qt.NewQPen4(brush, graphLine)
		pen.SetCapStyle(qt.RoundCap)
		g.brushes = append(g.brushes, brush)
		g.pens = append(g.pens, pen)
	}
	g.OnPaint(func(super func(*qt.QPainter, *qt.QStyleOptionViewItem, *qt.QModelIndex), painter *qt.QPainter, option *qt.QStyleOptionViewItem, index *qt.QModelIndex) {
		// The style draws the row's selection, and the cell has no text.
		super(painter, option, index)
		if row := index.Row(); row >= 0 && row < len(g.rows) {
			rect := option.Rect()
			selected := option.State()&qt.QStyle__State_Selected != 0
			g.paintRow(painter, g.rows[row], selected, rect.Left(), rect.Top(), rect.Width(), rect.Height())
		}
	})
	// Taller rows give the lines room to curve between lanes.
	g.OnSizeHint(func(super func(*qt.QStyleOptionViewItem, *qt.QModelIndex) *qt.QSize, option *qt.QStyleOptionViewItem, index *qt.QModelIndex) *qt.QSize {
		size := super(option, index)
		size.SetHeight(max(size.Height(), graphRow))
		return size
	})
	return g
}

// graphWidth returns the width of the column for a number of lanes. A history wider
// than the most is clipped, as the commits' text matters more.
func graphWidth(lanes int) int {
	return min(max(lanes*graphLane+graphMargin, graphLeast), graphMost)
}

// paintRow draws a row's edges, and its node on top of them.
func (g *graphColumn) paintRow(painter *qt.QPainter, row graph.Row, selected bool, left, top, width, height int) {
	painter.Save()
	defer painter.Restore()
	painter.SetClipRect2(left, top, width, height)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	painter.SetBrushWithStyle(qt.NoBrush)

	laneX := func(lane int) float64 {
		return float64(left) + graphMargin + float64(lane)*graphLane
	}
	y0, y1, y2 := float64(top), float64(top)+float64(height)/2, float64(top+height)
	for _, e := range row.Top {
		g.paintEdge(painter, e, laneX(e.From), y0, laneX(e.To), y1)
	}
	for _, e := range row.Bottom {
		g.paintEdge(painter, e, laneX(e.From), y1, laneX(e.To), y2)
	}

	// A ring sets the node off from the lines it joins: in the background, or
	// where that is the selection, which a node can be the colour of, in the
	// selected text's colour.
	role := qt.QPalette__Base
	if selected {
		role = qt.QPalette__HighlightedText
	}
	ring := qt.NewQBrush3(qt.QGuiApplication_Palette().ColorWithCr(role))
	defer ring.Delete()
	painter.SetPenWithStyle(qt.NoPen)
	x := laneX(row.Lane)
	painter.SetBrush(ring)
	drawCircle(painter, x, y1, graphNode+graphNodeRing)
	painter.SetBrush(g.brushes[row.Colour%len(g.brushes)])
	drawCircle(painter, x, y1, graphNode)
}

// paintEdge draws an edge: straight if it stays in its lane, or else an S that
// leaves and arrives upright, so that it runs smoothly on into the next row.
func (g *graphColumn) paintEdge(painter *qt.QPainter, e graph.Edge, x1, y1, x2, y2 float64) {
	painter.SetPenWithPen(g.pens[e.Colour%len(g.pens)])
	path := qt.NewQPainterPath()
	defer path.Delete()
	path.MoveTo2(x1, y1)
	if e.From == e.To {
		path.LineTo2(x2, y2)
	} else {
		middle := (y1 + y2) / 2
		path.CubicTo2(x1, middle, x2, middle, x2, y2)
	}
	painter.DrawPath(path)
}

// drawCircle draws a circle around a point.
func drawCircle(painter *qt.QPainter, x, y, radius float64) {
	centre := qt.NewQPointF3(x, y)
	defer centre.Delete()
	painter.DrawEllipse3(centre, radius, radius)
}
