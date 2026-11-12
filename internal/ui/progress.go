package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/settings"
)

// The progress bar shows only once work has gone on for progressDelay
// milliseconds, so that quick work does not make it flicker. It is
// progressHeight pixels high. While it shows, a segment progressChunk of its
// width sweeps across it every progressSweep milliseconds, moving every
// progressFrame.
const (
	progressDelay  = 300
	progressHeight = 4
	progressChunk  = 0.25
	progressSweep  = 1500
	progressFrame  = 20
)

// progress is the sign that something is being done in the background: a bar
// the width of the window under the toolbar, busy rather than counting, as how
// long git will take is not known. It is painted here, a segment in its colour
// sweeping across a faint track, as a style's own busy bar may be a still block
// of colour. It keeps its room while hidden, so that the panes under it don't
// move as it shows and goes.
type progress struct {
	qtwidgets.QProgressBarBase
	// delay waits for progressDelay before showing it, and frames moves it.
	delay, frames *qtcore.QTimer
	// phase is how far through a sweep the segment is, from 0 to 1.
	phase float64
	// colour is the segment's, as #rrggbb.
	colour string
}

// newProgress returns the bar, its segment in the default colour until another
// is given.
func newProgress() *progress {
	p := &progress{colour: settings.DefaultProgressColour}
	qtwidgets.NewQProgressBarWith(p, nil)
	p.SetRange(0, 0)
	p.SetTextVisible(false)
	p.SetFixedHeight(progressHeight)
	policy := p.SizePolicy()
	policy.SetRetainSizeWhenHidden(true)
	p.SetSizePolicy(policy)
	p.SetVisible(false)

	p.delay = qtcore.NewQTimer(p)
	p.delay.SetSingleShot(true)
	p.delay.OnTimeout(func() {
		p.phase = 0
		p.frames.StartWithInterval(progressFrame)
		p.SetVisible(true)
	})
	p.frames = qtcore.NewQTimer(p)
	p.frames.OnTimeout(func() {
		p.phase += float64(progressFrame) / progressSweep
		if p.phase >= 1 {
			p.phase--
		}
		p.Update()
	})
	return p
}

// start shows the bar once progressDelay has passed, unless stop comes first.
func (p *progress) start() {
	p.delay.StartWithInterval(progressDelay)
}

// stop hides the bar, or keeps it from showing.
func (p *progress) stop() {
	p.delay.Stop()
	p.frames.Stop()
	p.SetVisible(false)
}

// chunk returns where the segment is, from its left to its right, cut to the
// bar: it comes in at the left and goes out at the right.
func (p *progress) chunk() (left, right int) {
	width := float64(p.Width())
	start := (p.phase*(1+progressChunk) - progressChunk) * width
	left, right = int(max(start, 0)), int(min(start+progressChunk*width, width))
	return left, max(left, right)
}

// PaintEvent draws the track, in a colour a little way from the window's towards its
// text, so that it follows the theme, and the segment on it in its colour, by
// default Solarized's blue.
func (p *progress) PaintEvent(*qtgui.QPaintEvent) {
	palette := p.Palette()
	track := mix(palette.Color(qtgui.QPaletteWindowText), palette.Color(qtgui.QPaletteWindow), 15)
	painter := qtgui.NewQPainterOn(p)
	defer painter.End()
	painter.FillRect(qtcore.QRect{Width: p.Width(), Height: p.Height()}, track)
	left, right := p.chunk()
	painter.FillRect(qtcore.QRect{X: left, Width: right - left, Height: p.Height()}, colour(p.colour))
}
