// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

const testW, testH = 1400, 860

func newTestScene(t *testing.T) (*Scene, *ViewModel) {
	t.Helper()
	SetupText(1)
	vm := newTestVM(t)
	return NewScene(vm, testW, testH), vm
}

func pixel(buf []byte, w, x, y int) toolkit.RGBA {
	i := 4 * (y*w + x)
	return toolkit.RGBA{R: buf[i], G: buf[i+1], B: buf[i+2], A: buf[i+3]}
}

// count is how many pixels of r are within a small distance of c (a curve
// is anti-aliased, so few of its pixels are the exact colour).
func count(buf []byte, w int, r toolkit.Rect, c toolkit.RGBA) int {
	d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
	n := 0
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if p := pixel(buf, w, x, y); d(p.R, c.R)+d(p.G, c.G)+d(p.B, c.B) <= 60 {
				n++
			}
		}
	}
	return n
}

func render(s *Scene) []byte {
	buf := make([]byte, 4*s.w*s.h)
	s.Draw(buf)
	return buf
}

// center clicks the middle of a control.
func center(r toolkit.Rect) (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

// TestSceneDrawsBothThemes: the canvas wears the palette — its background,
// and the accent on the spectrum — in light and in dark.
func TestSceneDrawsBothThemes(t *testing.T) {
	s, _ := newTestScene(t)
	for _, p := range []Palette{LightPalette, DarkPalette} {
		s.SetPalette(p)
		if !s.AnimationStep(0) {
			t.Error("a palette change does not ask for a repaint")
		}
		buf := render(s)
		if s.AnimationStep(0) {
			t.Error("still dirty after a draw")
		}
		if got := pixel(buf, s.w, 2, 2); got != p.Background {
			t.Errorf("background %v, want %v", got, p.Background)
		}
		if n := count(buf, s.w, s.Rects()["outPlot"], p.Accent); n < 50 {
			t.Errorf("the spectrum has %d accent pixels", n)
		}
		if n := count(buf, s.w, toolkit.Rect{W: 400, H: 300}, p.Ink); n < 50 {
			t.Errorf("the controls have %d ink pixels", n)
		}
		if s.Theme().Accent != p.Accent || p.Theme().Background != p.Background {
			t.Error("Theme does not carry the palette")
		}
	}
}

// TestSceneTypingIntoAField is the sequence a visitor performs: click the
// field at the end of its text, erase, type a prime.
func TestSceneTypingIntoAField(t *testing.T) {
	s, vm := newTestScene(t)
	r := s.Rects()["n"]
	s.Click(r.X+r.W-4, r.Y+r.H/2)
	for range 4 {
		s.KeyDown("Backspace")
	}
	for _, c := range "1009" {
		s.Char(string(c))
	}
	if vm.N.Get() != "1009" || vm.OutputCurves.At(0).Y == nil || len(vm.OutputCurves.At(0).Y) != 505 {
		t.Fatalf("typed N = %q, %d output values", vm.N.Get(), len(vm.OutputCurves.At(0).Y))
	}
	if s.KeyDown("v") {
		t.Error("a shortcut letter was swallowed: the browser would not paste")
	}
	// A pasted line goes to the focused field.
	if !s.Paste("7\nignored") || vm.N.Get() != "10097" {
		t.Errorf("paste into the field: N = %q", vm.N.Get())
	}
	if s.Paste("") {
		t.Error("an empty paste reported a change")
	}
}

func TestSceneSampleEditor(t *testing.T) {
	s, vm := newTestScene(t)
	x, y := center(s.Rects()["samples"])
	s.Click(x, y)
	if !s.editing() {
		t.Fatal("clicking the sample editor does not give it the keyboard")
	}
	s.KeyDown("End")
	s.Char(",")
	s.Char("9")
	if vm.Preset.Get() != int(PresetCustom) || vm.N.Get() != "1001" {
		t.Errorf("typing in the editor: preset %d, N %s, %q", vm.Preset.Get(), vm.N.Get(), vm.Problem.Get())
	}
	if !s.Paste(", 1, 2") || vm.N.Get() != "1003" {
		t.Errorf("pasting in the editor: N %s %q", vm.N.Get(), vm.Problem.Get())
	}
	// A click elsewhere takes the keyboard back.
	s.Click(2, 2)
	if s.editing() {
		t.Error("the editor kept the keyboard")
	}
	if !s.Scroll(x, y, 0, 3) {
		t.Error("scroll")
	}
}

func TestScenePickersAndSwitch(t *testing.T) {
	s, vm := newTestScene(t)
	x, y := center(s.Rects()["transform"])
	s.Click(x, y)
	p, open := s.Rects()["popover"]
	if !open {
		t.Fatal("the transform list did not open")
	}
	row := p.H / len(TransformNames)
	s.Click(p.X+p.W/2, p.Y+row*int(TransformDCT)+row/2)
	if vm.Transform.Get() != int(TransformDCT) {
		t.Fatalf("picked %d, want DCT", vm.Transform.Get())
	}
	if _, open := s.Rects()["popover"]; open {
		t.Error("the list stayed open")
	}
	s.Click(center(s.Rects()["shift"]))
	if !vm.Shift.Get() {
		t.Error("the fftshift switch did not toggle")
	}
	// The copy button runs its command.
	var got string
	vm.Clipboard = func(c string) { got = c }
	s.Click(center(s.Rects()["copyNumpy"]))
	if got != vm.NumpyCode.Get() || got == "" {
		t.Errorf("Copy numpy copied %q", got)
	}
}

func TestScenePointer(t *testing.T) {
	s, _ := newTestScene(t)
	pr := s.Rects()["outPlot"]
	// Hover (no button): the plot's readout follows the pointer.
	if !s.Move(pr.X+pr.W/2, pr.Y+pr.H/2) || !s.outPlot.Hover().Get() {
		t.Error("hovering the spectrum shows no readout")
	}
	// A press captures; moves become drags; the release ends it.
	if s.Release(1, 1) {
		t.Error("a release with nothing pressed changed something")
	}
	s.Click(pr.X+10, pr.Y+10)
	if !s.Move(pr.X+20, pr.Y+10) {
		t.Error("drag")
	}
	if !s.Release(pr.X+20, pr.Y+10) || s.pressed != nil {
		t.Error("release")
	}
	if s.Context(1, 1) {
		t.Error("context menu")
	}
	// A press between controls captures the container under it.
	w, _ := deepest(s.body, s.body.Bounds().X+1, s.body.Bounds().Y+s.body.Bounds().H-1)
	if w == nil {
		t.Error("deepest found nothing")
	}
}

func TestSceneResize(t *testing.T) {
	s, _ := newTestScene(t)
	if w, h := s.Resize(800, 600); w != 800 || h != 600 {
		t.Errorf("Resize = %d×%d", w, h)
	}
	if w, h := s.Size(); w != 800 || h != 600 || s.Rects()["outPlot"].X+s.Rects()["outPlot"].W > 800 {
		t.Errorf("Size = %d×%d", w, h)
	}
	render(s)
	SetupText(2)
	defer SetupText(1)
	if w, h := s.Resize(800, 600); w != 1600 || h != 1200 {
		t.Errorf("Resize at 2x = %d×%d", w, h)
	}
	if w, h := s.Resize(0, 0); w != 1 || h != 1 {
		t.Errorf("Resize to nothing = %d×%d", w, h)
	}
	SetupText(0) // a non-positive ratio means 1
	if toolkit.MetricScale() != 1 {
		t.Errorf("SetupText(0) scale = %v", toolkit.MetricScale())
	}
}

func TestSceneWithoutTheMonoFace(t *testing.T) {
	saved := monoTTF
	monoTTF = []byte("not a font")
	defer func() { monoTTF = saved }()
	s, _ := newTestScene(t)
	if s.goCode.Font != nil {
		t.Error("a broken face was installed")
	}
	render(s)
}

func TestSceneUnbind(t *testing.T) {
	s, vm := newTestScene(t)
	before := len(s.inPlot.Series().At(0).Y)
	for _, u := range s.unbind {
		u()
	}
	vm.N.Set("32")
	if got := len(s.inPlot.Series().At(0).Y); got != before || got == 32 {
		t.Errorf("after unbind the input plot still follows the view-model: %d points", got)
	}
}
