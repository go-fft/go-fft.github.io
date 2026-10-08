// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"strings"

	"github.com/go-opentype/fonts/jetbrainsmono"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/mvvmtk"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// baseFontPx is the logical text size; SetupText multiplies it by the device
// pixel ratio.
const baseFontPx = 14

// SetupText installs the toolkit's metric scale and an anti-aliased face at
// that scale. The wasm shell calls it with window.devicePixelRatio BEFORE
// NewScene, so every metric the layout reads is already scaled: one logical
// pixel is one CSS pixel, drawn at device resolution.
func SetupText(scale float64) {
	if scale <= 0 {
		scale = 1
	}
	toolkit.SetMetricScale(scale)
	_ = toolkit.UseOpenTypeTextSize(baseFontPx)
}

// monoTTF is the face of the sample editor and the code panes.
var monoTTF = jetbrainsmono.TTF

// Palette is the handful of colours the page's stylesheet defines; the host
// reads them from the CSS custom properties, so the canvas wears exactly the
// landing's light or dark theme and its brand accent.
type Palette struct {
	Background, Surface, Ink, Line, Border, Accent toolkit.RGBA
}

// LightPalette and DarkPalette are the landing's own tokens (--bg,
// --bg-soft, --ink, --line, --elev-border and --accent from the shared landing
// stylesheet, with go-fft's [params.brand] accent). They are the fallback for
// a host that passes none, and what the native tests draw with.
var (
	LightPalette = Palette{
		Background: toolkit.RGB(0xff, 0xff, 0xff), Surface: toolkit.RGB(0xf7, 0xf8, 0xfb),
		Ink: toolkit.RGB(0x0f, 0x11, 0x15), Line: toolkit.RGB(0xe7, 0xe9, 0xee),
		Border: toolkit.RGB(0xcf, 0xd4, 0xdd), Accent: toolkit.RGB(0x04, 0x78, 0x57),
	}
	DarkPalette = Palette{
		Background: toolkit.RGB(0x0b, 0x0e, 0x14), Surface: toolkit.RGB(0x14, 0x1a, 0x24),
		Ink: toolkit.RGB(0xe6, 0xed, 0xf3), Line: toolkit.RGB(0x23, 0x2a, 0x36),
		Border: toolkit.RGB(0x3a, 0x45, 0x56), Accent: toolkit.RGB(0x34, 0xd3, 0x99),
	}
)

// Theme is the toolkit theme the palette paints with.
func (p Palette) Theme() *toolkit.Theme {
	return &toolkit.Theme{
		Background: p.Background, Surface: p.Surface, SurfaceAlt: p.Line,
		OnBackground: p.Ink, OnSurface: p.Ink, Accent: p.Accent, Border: p.Border,
	}
}

// Scene is the View: the toolkit widget tree, bound to a ViewModel, and the
// small amount of host bookkeeping a canvas application needs (the size, the
// pointer capture, a repaint flag). It implements go-widgets/webcanvas's App,
// Resizer, Scroller and Animator, so the wasm shell is one webcanvas.Run call.
type Scene struct {
	vm    *ViewModel
	w, h  int
	theme *toolkit.Theme

	root *toolkit.PopoverHost
	body *toolkit.HBox

	// The controls.
	preset, transform, typ, window, norm, precision, view *toolkit.DropDown
	n, fs, f1, f2, a1, a2                                 *toolkit.Entry
	shift                                                 *toolkit.Switch
	samples                                               *toolkit.TextView

	// The results.
	inPlot, outPlot                  *toolkit.XYPlot
	inTitle, outTitle                *toolkit.Label
	length, accuracy, speed, problem *toolkit.Label
	goCode, numpyCode                *toolkit.TextView
	copyGo, copyNumpy                *toolkit.Button

	// pressed is the widget a held button is dragging, and its bounds.
	pressed       toolkit.Widget
	pressedBounds toolkit.Rect

	dirty  bool
	unbind []func()
}

// row is the height of one control row, in logical pixels.
const row = 30

// NewScene builds the widget tree for a w×h device-pixel surface and binds it
// to vm. Call SetupText first.
func NewScene(vm *ViewModel, w, h int) *Scene {
	s := &Scene{vm: vm, w: w, h: h, theme: LightPalette.Theme()}
	inv := s.invalidate
	sc := toolkit.Scaled

	dd := func(options []string, o *mvvm.Observable[int]) *toolkit.DropDown {
		d := toolkit.NewDropDown(options, o.Get())
		s.unbind = append(s.unbind, mvvmtk.BindSelectedIndex(d, o, inv))
		return d
	}
	entry := func(o *mvvm.Observable[string]) *toolkit.Entry {
		e := toolkit.NewEntry(o.Get())
		s.unbind = append(s.unbind, mvvmtk.BindEntryText(e, o, inv))
		return e
	}
	label := func(o *mvvm.Observable[string]) *toolkit.Label {
		l := toolkit.NewLabel(o.Get())
		l.Ellipsis = true
		s.unbind = append(s.unbind, mvvmtk.BindLabel(l, o, inv))
		return l
	}
	caption := func(text string) *toolkit.Label {
		l := toolkit.NewLabel(text)
		l.Ellipsis = true
		return l
	}
	heading := func(text string) *toolkit.Label {
		return toolkit.NewLabel(text).SetFontSize(sc(baseFontPx + 3))
	}
	mono := func(o *mvvm.Observable[string]) *toolkit.TextView {
		tv := toolkit.NewTextView(o.Get())
		if f, err := toolkit.NewTrueTypeFont(monoTTF, sc(baseFontPx-1)); err == nil {
			tv.SetFont(f)
		}
		s.unbind = append(s.unbind, mvvmtk.BindTextView(tv, o, inv))
		return tv
	}

	// --- the controls -----------------------------------------------------
	s.preset = dd(PresetNames, vm.Preset)
	s.n, s.fs = entry(vm.N), entry(vm.Fs)
	s.f1, s.f2 = entry(vm.F1), entry(vm.F2)
	s.a1, s.a2 = entry(vm.A1), entry(vm.A2)
	s.samples = mono(vm.Samples)
	s.transform = dd(TransformNames, vm.Transform)
	s.typ = dd(TypeNames, vm.Type)
	s.window = dd(WindowNames, vm.Window)
	s.norm = dd(normLabels, vm.Norm)
	s.precision = dd(PrecisionNames, vm.Precision)
	s.view = dd(ViewNames, vm.View)
	s.view.OpenUp = true
	s.precision.OpenUp = true
	s.shift = toolkit.NewSwitch(vm.Shift.Get())
	s.unbind = append(s.unbind, mvvmtk.BindSwitch(s.shift, vm.Shift, inv))

	// pair is "label [entry]  label [entry]" on one row.
	pair := func(l1 string, e1 *toolkit.Entry, l2 string, e2 *toolkit.Entry) *toolkit.HBox {
		h := toolkit.NewHBox()
		h.Spacing = sc(6)
		h.AddFixed(caption(l1), sc(56))
		h.AddFlex(e1, 1)
		h.AddFixed(caption(l2), sc(56))
		h.AddFlex(e2, 1)
		return h
	}
	// labelled is "label [widget]" on one row.
	labelled := func(text string, w toolkit.Widget) *toolkit.HBox {
		h := toolkit.NewHBox()
		h.Spacing = sc(6)
		h.AddFixed(caption(text), sc(84))
		h.AddFlex(w, 1)
		return h
	}
	shiftRow := toolkit.NewHBox()
	shiftRow.Spacing = sc(6)
	shiftRow.AddFixed(caption("fftshift"), sc(84))
	shiftRow.AddFixed(s.shift, sc(44))
	shiftRow.AddFlex(caption("(FFT: zero frequency in the middle)"), 1)

	left := toolkit.NewVBox()
	left.Spacing = sc(6)
	left.AddFixed(heading("Signal"), sc(row))
	left.AddFixed(labelled("preset", s.preset), sc(row))
	left.AddFixed(pair("N", s.n, "fs (Hz)", s.fs), sc(row))
	left.AddFixed(pair("f1 (Hz)", s.f1, "f2 (Hz)", s.f2), sc(row))
	left.AddFixed(pair("a1", s.a1, "a2", s.a2), sc(row))
	left.AddFixed(caption("samples — type or paste your own (commas, spaces, new lines):"), sc(row-6))
	left.AddFlex(s.samples, 1)
	left.AddFixed(heading("Transform"), sc(row))
	left.AddFixed(labelled("transform", s.transform), sc(row))
	left.AddFixed(labelled("DCT/DST", s.typ), sc(row))
	left.AddFixed(labelled("window", s.window), sc(row))
	left.AddFixed(labelled("norm", s.norm), sc(row))
	left.AddFixed(labelled("precision", s.precision), sc(row))
	left.AddFixed(labelled("show", s.view), sc(row))
	left.AddFixed(shiftRow, sc(row))

	// --- the results ------------------------------------------------------
	s.inPlot, s.outPlot = &toolkit.XYPlot{}, &toolkit.XYPlot{}
	s.unbind = append(s.unbind,
		bindCurves(s.inPlot, vm.InputCurves, inv),
		bindCurves(s.outPlot, vm.OutputCurves, inv),
	)
	s.inTitle = label(prefixed(vm.InputAxis, "Input — x axis: "))
	s.outTitle = label(vm.OutputTitle)
	outAxis := label(prefixed(vm.OutputAxis, "x axis: "))
	s.length, s.accuracy, s.speed = label(vm.Length), label(vm.Accuracy), label(vm.Speed)
	s.problem = label(vm.Problem)
	s.problem.Ink = toolkit.RGB(0xd9, 0x30, 0x25)
	s.goCode, s.numpyCode = mono(vm.GoCode), mono(vm.NumpyCode)
	s.copyGo = toolkit.NewButton("Copy Go", nil)
	s.copyNumpy = toolkit.NewButton("Copy numpy", nil)
	s.unbind = append(s.unbind,
		mvvmtk.BindCommand(s.copyGo, vm.CopyGo, inv),
		mvvmtk.BindCommand(s.copyNumpy, vm.CopyNumpy, inv),
	)

	outHead := toolkit.NewHBox()
	outHead.Spacing = sc(12)
	outHead.AddFlex(s.outTitle, 3)
	outHead.AddFlex(outAxis, 2)

	codeHead := toolkit.NewHBox()
	codeHead.Spacing = sc(8)
	codeHead.AddFlex(heading("The same thing in Go, and in numpy"), 1)
	codeHead.AddFixed(s.copyGo, s.copyGo.PreferredWidth()+sc(8))
	codeHead.AddFixed(s.copyNumpy, s.copyNumpy.PreferredWidth()+sc(8))

	code := toolkit.NewHBox()
	code.Spacing = sc(8)
	code.AddFlex(s.goCode, 3)
	code.AddFlex(s.numpyCode, 2)

	right := toolkit.NewVBox()
	right.Spacing = sc(4)
	right.AddFixed(s.inTitle, sc(row-6))
	right.AddFlex(s.inPlot, 2)
	right.AddFixed(outHead, sc(row-6))
	right.AddFlex(s.outPlot, 3)
	right.AddFixed(s.length, sc(row-8))
	right.AddFixed(s.accuracy, sc(row-8))
	right.AddFixed(s.speed, sc(row-8))
	right.AddFixed(s.problem, sc(row-8))
	right.AddFixed(codeHead, sc(row+2))
	right.AddFixed(code, sc(200))

	s.body = toolkit.NewHBox()
	s.body.Spacing = sc(18)
	s.body.AddFixed(left, sc(380))
	s.body.AddFlex(right, 1)
	s.root = toolkit.NewPopoverHost(s.body)
	s.layout()
	return s
}

// normLabels are numpy's norm names with their go-fft constants.
var normLabels = []string{`"backward" (fft.NormBackward)`, `"ortho" (fft.NormOrtho)`, `"forward" (fft.NormForward)`}

// prefixed is a one-way derived observable: prefix + o.
func prefixed(o *mvvm.Observable[string], prefix string) *mvvm.Observable[string] {
	d := mvvm.NewObservable(prefix + o.Get())
	o.Subscribe(func(v string) { d.Set(prefix + v) })
	return d
}

// bindCurves makes a plot show a view-model curve list: every change to the
// list replaces the plot's series.
func bindCurves(p *toolkit.XYPlot, l *mvvm.ObservableList[Curve], invalidate func()) func() {
	push := func() {
		curves := l.Slice()
		series := make([]toolkit.PlotSeries, len(curves))
		for i, c := range curves {
			series[i] = toolkit.PlotSeries{Label: c.Label, X: c.X, Y: c.Y}
			if c.Style == CurveStem {
				series[i].Style = toolkit.PlotStem
			}
		}
		p.SetSeries(series...)
		invalidate()
	}
	push()
	return l.SubscribeChanged(push)
}

func (s *Scene) invalidate() { s.dirty = true }

// layout fits the tree to the surface, with a margin.
func (s *Scene) layout() {
	m := toolkit.Scaled(14)
	s.root.SetBounds(toolkit.Rect{W: s.w, H: s.h})
	s.body.SetBounds(toolkit.Rect{X: m, Y: m, W: s.w - 2*m, H: s.h - 2*m})
}

// SetPalette recolours the scene: the host calls it at start and whenever the
// page's theme flips.
func (s *Scene) SetPalette(p Palette) {
	s.theme = p.Theme()
	s.dirty = true
}

// Theme is the toolkit theme the scene is painted with.
func (s *Scene) Theme() *toolkit.Theme { return s.theme }

// Size is the surface size in device pixels.
func (s *Scene) Size() (int, int) { return s.w, s.h }

// Resize relays the scene out for a canvas of cw×ch CSS pixels, returning the
// device-pixel size it renders at (the CSS size times the metric scale).
func (s *Scene) Resize(cw, ch int) (int, int) {
	scale := toolkit.MetricScale()
	s.w, s.h = max(int(float64(cw)*scale+0.5), 1), max(int(float64(ch)*scale+0.5), 1)
	s.layout()
	return s.w, s.h
}

// Draw paints the whole surface into buf (RGBA, row-major).
func (s *Scene) Draw(buf []byte) {
	bg := s.theme.Background
	for i := 0; i+3 < len(buf); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = bg.R, bg.G, bg.B, 0xff
	}
	s.root.Draw(painter.NewPixelPainter(buf, s.w, s.h), s.theme)
	s.dirty = false
}

// AnimationStep repaints when something outside an input event changed the
// scene — the theme, a paste, a binding — and costs nothing otherwise.
func (s *Scene) AnimationStep(float64) bool { return s.dirty }

// editing is the sample editor while it holds the keyboard. A TextView keeps
// its own focus flag (it is not in the toolkit's focus walk), so the scene
// routes keys to it first.
func (s *Scene) editing() bool { return s.samples.Focused().Get() }

// Click is a primary-button press at device pixel (x, y).
func (s *Scene) Click(x, y int) bool {
	inSamples := s.samples.Bounds().Contains(x, y)
	s.samples.Focused().Set(inSamples)
	s.root.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: x, Y: y})
	s.pressed, s.pressedBounds = deepest(s.body, x, y)
	return true
}

// deepest is the leaf widget under (x, y), for drag capture.
func deepest(w toolkit.Widget, x, y int) (toolkit.Widget, toolkit.Rect) {
	type parent interface{ Children() []toolkit.Widget }
	for {
		p, ok := w.(parent)
		if !ok {
			return w, w.Bounds()
		}
		next := toolkit.Widget(nil)
		for _, c := range p.Children() {
			if c.Bounds().Contains(x, y) {
				next = c
				break
			}
		}
		if next == nil {
			return w, w.Bounds()
		}
		w = next
	}
}

// Move is pointer motion: a drag to the pressed widget, else hover.
func (s *Scene) Move(x, y int) bool {
	if s.pressed != nil {
		r := s.pressedBounds
		s.pressed.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: x - r.X, Y: y - r.Y})
		return true
	}
	s.root.OnEvent(toolkit.Event{Kind: toolkit.EventMouseMove, X: x, Y: y})
	return true
}

// Release ends a press.
func (s *Scene) Release(x, y int) bool {
	if s.pressed == nil {
		return false
	}
	r := s.pressedBounds
	s.pressed.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: x - r.X, Y: y - r.Y})
	s.pressed = nil
	return true
}

// Context is a secondary click; the playground has no context menu.
func (s *Scene) Context(int, int) bool { return false }

// key delivers a keyboard event to whatever holds the keyboard.
func (s *Scene) key(ev toolkit.Event) {
	if s.editing() {
		s.samples.OnEvent(ev)
		return
	}
	s.root.OnEvent(ev)
}

// Char is typed text.
func (s *Scene) Char(ch string) bool {
	s.key(toolkit.Event{Kind: toolkit.EventChar, Code: ch})
	return true
}

// KeyDown is a named key. A single character here is a shortcut (the shell
// sends ⌘V as "v"): it is left to the browser, which turns copy and paste
// into the events the host forwards to Paste.
func (s *Scene) KeyDown(code string) bool {
	if len([]rune(code)) == 1 {
		return false
	}
	s.key(toolkit.Event{Kind: toolkit.EventKeyDown, Code: code})
	return true
}

// Scroll is the wheel, in rows.
func (s *Scene) Scroll(x, y, _, dy int) bool {
	s.root.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, X: x, Y: y, Delta: dy})
	return true
}

// Paste inserts text from the system clipboard where the keyboard is: the
// sample editor takes it whole (a pasted column of numbers), a field takes
// its first line.
func (s *Scene) Paste(text string) bool {
	if s.editing() {
		s.samples.Paste(text)
		s.dirty = true
		return true
	}
	line, _, _ := strings.Cut(text, "\n")
	for _, r := range line {
		s.root.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
	}
	s.dirty = true
	return line != ""
}

// Rects names the on-screen rectangle of each control, in device pixels, for
// a test harness that drives the page with synthetic events.
func (s *Scene) Rects() map[string]toolkit.Rect {
	r := map[string]toolkit.Rect{
		"preset": s.preset.Bounds(), "transform": s.transform.Bounds(), "type": s.typ.Bounds(),
		"window": s.window.Bounds(), "norm": s.norm.Bounds(), "precision": s.precision.Bounds(),
		"view": s.view.Bounds(), "shift": s.shift.Bounds(), "n": s.n.Bounds(), "fs": s.fs.Bounds(),
		"f1": s.f1.Bounds(), "f2": s.f2.Bounds(), "a1": s.a1.Bounds(), "a2": s.a2.Bounds(),
		"samples": s.samples.Bounds(), "inPlot": s.inPlot.Bounds(), "outPlot": s.outPlot.Bounds(),
		"copyGo": s.copyGo.Bounds(), "copyNumpy": s.copyNumpy.Bounds(),
		"goCode": s.goCode.Bounds(), "numpyCode": s.numpyCode.Bounds(),
	}
	// An open option list, whose rows share its height evenly.
	for _, p := range toolkit.PopoverOwners(s.root) {
		r["popover"] = p.PopoverBounds()
	}
	return r
}
