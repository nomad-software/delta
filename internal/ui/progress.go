package ui

import (
	"runtime"

	qt "github.com/mappu/miqt/qt6"
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
// long git will take is not known. It is painted here, a segment in its
// colour sweeping across a faint track, as a style's own busy bar may
// be a still block of colour. It keeps its room while hidden, so that the panes
// under it don't move as it shows and goes.
type progress struct {
	*qt.QProgressBar
	// delay waits for progressDelay before showing it, and frames moves it.
	delay, frames *qt.QTimer
	// phase is how far through a sweep the segment is, from 0 to 1.
	phase float64
	// colour is the segment's, as #rrggbb.
	colour string
}

// newProgress returns the bar, its segment in the colour.
func newProgress(colour string) *progress {
	p := &progress{QProgressBar: qt.NewQProgressBar2(), colour: colour}
	p.SetRange(0, 0)
	p.SetTextVisible(false)
	p.SetFixedHeight(progressHeight)
	policy := p.SizePolicy()
	policy.SetRetainSizeWhenHidden(true)
	p.SetSizePolicy(*policy)
	p.SetVisible(false)
	p.OnPaintEvent(func(func(*qt.QPaintEvent), *qt.QPaintEvent) { p.paint() })

	p.delay = qt.NewQTimer2(p.QObject)
	p.delay.SetSingleShot(true)
	p.delay.OnTimeout(func() {
		p.phase = 0
		p.frames.Start(progressFrame)
		p.SetVisible(true)
	})
	p.frames = qt.NewQTimer2(p.QObject)
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
	p.delay.Start(progressDelay)
}

// stop hides the bar, or keeps it from showing.
func (p *progress) stop() {
	p.delay.Stop()
	p.frames.Stop()
	p.SetVisible(false)
}

// moving says whether the segment is sweeping, on show.
func (p *progress) moving() bool {
	return p.frames.IsActive()
}

// chunk returns where the segment is, from its left to its right, cut to the
// bar: it comes in at the left and goes out at the right.
func (p *progress) chunk() (left, right int) {
	width := float64(p.Width())
	start := (p.phase*(1+progressChunk) - progressChunk) * width
	left, right = int(max(start, 0)), int(min(start+progressChunk*width, width))
	return left, max(left, right)
}

// paint draws the track, in a colour a little way from the window's towards its
// text, so that it follows the theme, and the segment on it in its colour, by
// default Solarized's blue.
func (p *progress) paint() {
	// The palette is a copy, whose colours are its own, kept until they have
	// been used.
	palette := p.Palette()
	defer runtime.KeepAlive(palette)
	track := mix(palette.ColorWithCr(qt.QPalette__WindowText), palette.ColorWithCr(qt.QPalette__Window), 15)
	defer track.Delete()
	painter := qt.NewQPainter2(p.QPaintDevice)
	defer painter.Delete()
	painter.FillRect5(0, 0, p.Width(), p.Height(), track)
	left, right := p.chunk()
	segment := qt.NewQColor6(p.colour)
	defer segment.Delete()
	painter.FillRect5(left, 0, right-left, p.Height(), segment)
	painter.End()
}
