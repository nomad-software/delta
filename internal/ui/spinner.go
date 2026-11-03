package ui

import (
	"math"

	qt "github.com/mappu/miqt/qt6"
)

// The spinner shows only once work has gone on for spinnerDelay milliseconds, so
// that quick work does not make it flicker, and turns spinnerStep degrees every
// spinnerFrame milliseconds while it shows. Its arc is spinnerArc degrees of a
// circle.
const (
	spinnerDelay = 300
	spinnerFrame = 50
	spinnerStep  = 30
	spinnerArc   = 270
)

// spinner is the toolbar's sign that something is being done in the
// background: a turning arc the size of a button's icon, in the style of the
// icons, with Working under it, as a button has its name. It is not a button.
type spinner struct {
	*qt.QWidget
	icon, text *qt.QLabel
	// delay waits for spinnerDelay before showing it, and frames turns it.
	delay, frames *qt.QTimer
	// angle is where the arc starts, in degrees.
	angle int
	// shown is told whether the spinner should be on show, as the toolbar's
	// action holding it is what shows or hides it.
	shown func(bool)
}

func newSpinner() *spinner {
	s := &spinner{QWidget: qt.NewQWidget2()}
	s.SetMinimumWidth(toolButtonWidth)
	s.icon = qt.NewQLabel2()
	s.icon.SetAlignment(qt.AlignCenter)
	s.icon.SetFixedHeight(toolIconSize)
	s.text = qt.NewQLabel3("Working")
	s.text.SetAlignment(qt.AlignCenter)
	layout := qt.NewQVBoxLayout(s.QWidget)
	layout.SetContentsMargins(4, 3, 4, 3)
	layout.SetSpacing(2)
	layout.AddWidget(s.icon.QWidget)
	layout.AddWidget(s.text.QWidget)

	s.delay = qt.NewQTimer2(s.QObject)
	s.delay.SetSingleShot(true)
	s.delay.OnTimeout(func() {
		s.frames.Start(spinnerFrame)
		s.paint()
		s.shown(true)
	})
	s.frames = qt.NewQTimer2(s.QObject)
	s.frames.OnTimeout(func() {
		s.angle = (s.angle + spinnerStep) % 360
		s.paint()
	})
	return s
}

// start shows the spinner once spinnerDelay has passed, unless stop comes first.
func (s *spinner) start() {
	s.delay.Start(spinnerDelay)
}

// stop hides the spinner, or keeps it from showing.
func (s *spinner) stop() {
	s.delay.Stop()
	s.frames.Stop()
	s.shown(false)
}

// spinning says whether the spinner is turning, on show.
func (s *spinner) spinning() bool {
	return s.frames.IsActive()
}

// paint draws the arc where it has turned to, in the colour of the text, at the
// screen's own resolution.
func (s *spinner) paint() {
	ratio := s.DevicePixelRatioF()
	side := int(math.Ceil(toolIconSize * ratio))
	image := qt.NewQImage3(side, side, qt.QImage__Format_ARGB32_Premultiplied)
	defer image.Delete()
	image.Fill2(qt.Transparent)
	p := qt.NewQPainter2(image.QPaintDevice)
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.Scale(float64(side)/iconGrid, float64(side)/iconGrid)
	colour := iconColour(qt.QIcon__Normal)
	brush := qt.NewQBrush3(colour)
	pen := qt.NewQPen8(brush, iconStroke, qt.SolidLine, qt.RoundCap, qt.RoundJoin)
	p.SetPenWithPen(pen)
	// Qt's angles are in sixteenths of a degree, anticlockwise, so it turns
	// clockwise as the angle grows.
	p.DrawArc3(4, 4, 16, 16, -s.angle*16, spinnerArc*16)
	p.End()
	p.Delete()
	pen.Delete()
	brush.Delete()
	colour.Delete()
	pixmap := qt.QPixmap_FromImage(image)
	pixmap.SetDevicePixelRatio(ratio)
	s.icon.SetPixmap(pixmap)
}
