package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/graph"
	"github.com/nomad-software/delta/internal/settings"
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

// graphColumn draws the graph in the column of the commits, row by row, which is
// what makes it scroll with them and take no time for rows out of sight.
type graphColumn struct {
	qtwidgets.QStyledItemDelegateBase
	// rows are what to draw.
	rows []graph.Row
	// pens and brushes are of each of graphColours.
	pens    []*qtgui.QPen
	brushes []*qtgui.QBrush
}

// graphColours are the colours of the branches, taken in turn: Solarized's
// accents, which read on light and dark backgrounds alike, in an order that puts
// those most alike apart.
var graphColours = []string{
	settings.Blue, settings.Red, settings.Green, settings.Yellow,
	settings.Violet, settings.Cyan, settings.Magenta, settings.Orange,
}

func newGraphColumn(parent qtcore.QObjectLike) *graphColumn {
	g := &graphColumn{}
	qtwidgets.NewQStyledItemDelegateWith(g, parent)
	for _, name := range graphColours {
		brush := qtgui.NewQBrushWithColor(colour(name))
		pen := qtgui.NewQPenWithBrush(brush, graphLine)
		pen.SetCapStyle(qtcore.RoundCap)
		g.brushes = append(g.brushes, brush)
		g.pens = append(g.pens, pen)
	}
	return g
}

// Paint draws a row's cell: the style draws the row's selection, and the cell
// has no text, then the graph is drawn over it.
func (g *graphColumn) Paint(painter *qtgui.QPainter, option *qtwidgets.QStyleOptionViewItem, index *qtcore.QModelIndex) {
	g.Super().Paint(painter, option, index)
	if row := index.Row(); row >= 0 && row < len(g.rows) {
		rect := option.Rect()
		selected := option.State()&qtwidgets.QStyleState_Selected != 0
		g.paintRow(painter, g.rows[row], selected, rect.X, rect.Y, rect.Width, rect.Height)
	}
}

// SizeHint makes the rows taller, giving the lines room to curve between lanes.
func (g *graphColumn) SizeHint(option *qtwidgets.QStyleOptionViewItem, index *qtcore.QModelIndex) qtcore.QSize {
	size := g.Super().SizeHint(option, index)
	size.Height = max(size.Height, graphRow)
	return size
}

// graphWidth returns the width of the column for a number of lanes. A history wider
// than the most is clipped, as the commits' text matters more.
func graphWidth(lanes int) int {
	return min(max(lanes*graphLane+graphMargin, graphLeast), graphMost)
}

// paintRow draws a row's edges, and its node on top of them.
func (g *graphColumn) paintRow(painter *qtgui.QPainter, row graph.Row, selected bool, left, top, width, height int) {
	painter.Save()
	defer painter.Restore()
	painter.SetClipRect(qtcore.QRect{X: left, Y: top, Width: width, Height: height})
	painter.SetRenderHint(qtgui.QPainterAntialiasing)
	painter.SetBrushStyle(qtcore.NoBrush)

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
	role := qtgui.QPaletteBase
	if selected {
		role = qtgui.QPaletteHighlightedText
	}
	painter.SetPenStyle(qtcore.NoPen)
	x := laneX(row.Lane)
	painter.SetBrushColor(qtgui.QGuiApplicationPalette().Color(role))
	drawCircle(painter, x, y1, graphNode+graphNodeRing)
	painter.SetBrush(g.brushes[row.Colour%len(g.brushes)])
	drawCircle(painter, x, y1, graphNode)
}

// paintEdge draws an edge: straight if it stays in its lane, or else an S that
// leaves and arrives upright, so that it runs smoothly on into the next row.
func (g *graphColumn) paintEdge(painter *qtgui.QPainter, e graph.Edge, x1, y1, x2, y2 float64) {
	painter.SetPen(g.pens[e.Colour%len(g.pens)])
	path := qtgui.NewQPainterPath()
	path.MoveTo(qtcore.QPointF{X: x1, Y: y1})
	if e.From == e.To {
		path.LineTo(qtcore.QPointF{X: x2, Y: y2})
	} else {
		middle := (y1 + y2) / 2
		path.CubicTo(qtcore.QPointF{X: x1, Y: middle}, qtcore.QPointF{X: x2, Y: middle}, qtcore.QPointF{X: x2, Y: y2})
	}
	painter.DrawPath(path)
}

// drawCircle draws a circle around a point.
func drawCircle(painter *qtgui.QPainter, x, y, radius float64) {
	painter.DrawEllipseAtF(qtcore.QPointF{X: x, Y: y}, radius, radius)
}
