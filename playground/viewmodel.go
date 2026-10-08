// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"fmt"
	"math"
	"math/cmplx"
	"strconv"
	"strings"
	"time"

	"github.com/go-fft/fft"
	"github.com/go-widgets/mvvm"
)

// CurveStyle is how a curve is drawn: joined, or as stems from zero.
type CurveStyle int

const (
	CurveLine CurveStyle = iota
	CurveStem
)

// Curve is one plotted series, in the view-model's own terms: the View turns
// it into a toolkit plot series. The view-model imports no widget.
type Curve struct {
	Label string
	X, Y  []float64
	Style CurveStyle
}

// View names how a complex output is shown.
type View int

const (
	ViewMagnitude View = iota
	ViewDecibel
	ViewPhase
	ViewReIm
)

// ViewNames are the views in View order.
var ViewNames = []string{"Magnitude |y|", "Magnitude (dB)", "Phase (rad)", "Real and imaginary"}

// TypeNames are the DCT/DST types as the picker lists them (index 0 is type I).
var TypeNames = []string{"Type I", "Type II", "Type III", "Type IV"}

// MaxListed is the longest signal whose samples the editor lists. A million
// numbers in a text box helps nobody and costs a second to lay out.
const MaxListed = 4096

// stemLimit is the longest output drawn as stems; past it a stem plot is a
// solid block and a line reads better.
const stemLimit = 128

// ViewModel holds every piece of the playground's state as go-widgets/mvvm
// observables, and derives the plots, the numbers and the code from them. It
// references no widget; scene.go binds the widgets to it.
type ViewModel struct {
	// The signal.
	Preset  *mvvm.Observable[int]
	N       *mvvm.Observable[string]
	Fs      *mvvm.Observable[string]
	F1, F2  *mvvm.Observable[string]
	A1, A2  *mvvm.Observable[string]
	Samples *mvvm.Observable[string]

	// The transform.
	Transform *mvvm.Observable[int]
	Type      *mvvm.Observable[int]
	Window    *mvvm.Observable[int]
	Norm      *mvvm.Observable[int]
	Precision *mvvm.Observable[int]
	View      *mvvm.Observable[int]
	Shift     *mvvm.Observable[bool]

	// Derived: written by recompute alone.
	InputCurves  *mvvm.ObservableList[Curve]
	OutputCurves *mvvm.ObservableList[Curve]
	InputAxis    *mvvm.Observable[string]
	OutputAxis   *mvvm.Observable[string]
	OutputTitle  *mvvm.Observable[string]
	Length       *mvvm.Observable[string] // "N = 1009 (prime) · Rader/Bluestein"
	Accuracy     *mvvm.Observable[string] // the round trip
	Speed        *mvvm.Observable[string] // the timing
	Problem      *mvvm.Observable[string] // why nothing could be computed, or ""
	GoCode       *mvvm.Observable[string]
	NumpyCode    *mvvm.Observable[string]

	// Commands.
	CopyGo    *mvvm.Command
	CopyNumpy *mvvm.Command

	// Clipboard receives the text a Copy command copies; the host installs
	// it (navigator.clipboard in a browser). Nil copies nothing.
	Clipboard func(string)
	// Now is the clock the timing reads (time.Now unless a test swaps it).
	Now func() time.Time
	// Budget is how long the timing keeps repeating a transform.
	Budget time.Duration

	// writing is set while recompute writes Samples or N itself, so that
	// write is not mistaken for the visitor editing them.
	writing bool
	// samples is the signal the last recompute used.
	samples []float64
}

// NewViewModel builds the view-model with the opening configuration — two
// sines in 1 000 samples at 1 kHz, an RFFT, a Hann window — and computes it.
func NewViewModel() *ViewModel {
	vm := &ViewModel{
		Preset:  mvvm.NewObservable(int(PresetSines)),
		N:       mvvm.NewObservable("1000"),
		Fs:      mvvm.NewObservable("1000"),
		F1:      mvvm.NewObservable("50"),
		F2:      mvvm.NewObservable("120"),
		A1:      mvvm.NewObservable("1"),
		A2:      mvvm.NewObservable("0.5"),
		Samples: mvvm.NewObservable(""),

		Transform: mvvm.NewObservable(int(TransformRFFT)),
		Type:      mvvm.NewObservable(1),
		Window:    mvvm.NewObservable(int(WindowHann)),
		Norm:      mvvm.NewObservable(int(fft.NormBackward)),
		Precision: mvvm.NewObservable(int(Float64)),
		View:      mvvm.NewObservable(int(ViewMagnitude)),
		Shift:     mvvm.NewObservable(false),

		InputCurves:  mvvm.NewObservableList[Curve](),
		OutputCurves: mvvm.NewObservableList[Curve](),
		InputAxis:    mvvm.NewObservable(""),
		OutputAxis:   mvvm.NewObservable(""),
		OutputTitle:  mvvm.NewObservable(""),
		Length:       mvvm.NewObservable(""),
		Accuracy:     mvvm.NewObservable(""),
		Speed:        mvvm.NewObservable(""),
		Problem:      mvvm.NewObservable(""),
		GoCode:       mvvm.NewObservable(""),
		NumpyCode:    mvvm.NewObservable(""),

		Now:    time.Now,
		Budget: 20 * time.Millisecond,
	}
	vm.CopyGo = mvvm.NewCommand(func() { vm.copy(vm.GoCode.Get()) }, nil)
	vm.CopyNumpy = mvvm.NewCommand(func() { vm.copy(vm.NumpyCode.Get()) }, nil)

	for _, o := range []*mvvm.Observable[string]{vm.N, vm.Fs, vm.F1, vm.F2, vm.A1, vm.A2} {
		o.SubscribeChanged(vm.signalEdited)
	}
	vm.Preset.SubscribeChanged(vm.signalEdited)
	vm.Samples.SubscribeChanged(vm.samplesEdited)
	for _, o := range []*mvvm.Observable[int]{vm.Transform, vm.Type, vm.Window, vm.Norm, vm.Precision, vm.View} {
		o.SubscribeChanged(vm.recompute)
	}
	vm.Shift.SubscribeChanged(vm.recompute)
	vm.signalEdited()
	return vm
}

func (vm *ViewModel) copy(s string) {
	if vm.Clipboard != nil {
		vm.Clipboard(s)
	}
}

// set writes an observable on the view-model's own behalf (see writing).
func (vm *ViewModel) set(o *mvvm.Observable[string], v string) {
	vm.writing = true
	o.Set(v)
	vm.writing = false
}

// samplesEdited is the visitor typing or pasting in the sample editor: the
// text becomes the signal, and the preset becomes Custom.
func (vm *ViewModel) samplesEdited() {
	if vm.writing {
		return
	}
	vm.writing = true
	vm.Preset.Set(int(PresetCustom))
	vm.writing = false
	vm.recompute()
}

// signalEdited is a change to the preset or its parameters: regenerate the
// signal, list it in the editor, and recompute.
func (vm *ViewModel) signalEdited() {
	if vm.writing {
		return
	}
	if Preset(vm.Preset.Get()) == PresetCustom {
		vm.recompute()
		return
	}
	p, err := vm.params()
	if err != nil {
		vm.fail(err.Error())
		return
	}
	x := Generate(p)
	if len(x) <= MaxListed {
		vm.set(vm.Samples, FormatSamples(x))
	} else {
		vm.set(vm.Samples, fmt.Sprintf("# %d samples: too many to list here.\n# Type or paste your own to replace them.", len(x)))
	}
	vm.samples = x
	vm.recompute()
}

// field parses one numeric parameter.
func field(name, s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%s: %q is not a number", name, s)
	}
	return v, nil
}

// params reads the preset's parameters from the text fields.
func (vm *ViewModel) params() (SignalParams, error) {
	p := SignalParams{Preset: Preset(vm.Preset.Get()), Seed: 1}
	n, err := strconv.Atoi(strings.TrimSpace(vm.N.Get()))
	if err != nil || n < 1 || n > MaxN {
		return p, fmt.Errorf("N: %q is not a length from 1 to %d", vm.N.Get(), MaxN)
	}
	p.N = n
	fs, err := vm.fs()
	if err != nil {
		return p, err
	}
	p.Fs = fs
	for _, f := range []struct {
		name string
		o    *mvvm.Observable[string]
		dst  *float64
	}{{"f1", vm.F1, &p.F1}, {"f2", vm.F2, &p.F2}, {"a1", vm.A1, &p.A1}, {"a2", vm.A2, &p.A2}} {
		if *f.dst, err = field(f.name, f.o.Get()); err != nil {
			return p, err
		}
	}
	return p, nil
}

// fs reads the sample rate.
func (vm *ViewModel) fs() (float64, error) {
	fs, err := field("fs", vm.Fs.Get())
	if err == nil && fs <= 0 {
		err = fmt.Errorf("fs: the sample rate must be positive")
	}
	return fs, err
}

// Config is the transform configuration the pickers describe.
func (vm *ViewModel) Config() Config {
	fs, _ := vm.fs()
	return Config{
		Transform: Transform(vm.Transform.Get()),
		Type:      vm.Type.Get() + 1,
		Norm:      fft.Norm(vm.Norm.Get()),
		Window:    Window(vm.Window.Get()),
		Precision: Precision(vm.Precision.Get()),
		Shift:     vm.Shift.Get(),
		Fs:        fs,
	}
}

// fail clears every derived output and says why.
func (vm *ViewModel) fail(why string) {
	vm.Problem.Set(why)
	vm.InputCurves.Clear()
	vm.OutputCurves.Clear()
	vm.Accuracy.Set("")
	vm.Speed.Set("")
}

// recompute derives everything shown from the current state: it is the only
// writer of the derived observables.
func (vm *ViewModel) recompute() {
	if Preset(vm.Preset.Get()) == PresetCustom {
		x, err := ParseSamples(vm.Samples.Get())
		if err != nil {
			vm.fail("samples: " + err.Error())
			return
		}
		vm.samples = x
		vm.set(vm.N, strconv.Itoa(len(x)))
	}
	x := vm.samples
	c := vm.Config()
	if _, err := vm.fs(); err != nil {
		vm.fail(err.Error())
		return
	}
	res, err := Run(x, c)
	if err != nil {
		vm.fail(err.Error())
		return
	}
	vm.Problem.Set("")
	n := len(x)

	vm.GoCode.Set(GoCode(c, n))
	vm.NumpyCode.Set(NumpyCode(c, n))

	engine := "Stockham"
	if !Smooth(n) {
		engine = "Rader or Bluestein"
	}
	if c.Transform == TransformRFFT || c.Transform == TransformIRFFT {
		if fast := fft.NextFastLen(n, true); fast != n {
			engine += fmt.Sprintf(" · NextFastLen = %d", fast)
		}
	} else if fast := fft.NextFastLen(n, false); fast != n {
		engine += fmt.Sprintf(" · NextFastLen = %d", fast)
	}
	vm.Length.Set(fmt.Sprintf("N = %d = %s · %s", n, Factor(n), engine))
	vm.Accuracy.Set(fmt.Sprintf("round trip: max |x − inverse(forward(x))| = %.2g", res.RoundTrip))
	// Run accepted this configuration, and Time validates the same way.
	t, _ := Time(x, c, vm.Budget, vm.Now)
	vm.Speed.Set(fmt.Sprintf("%s per call, in this browser (mean of %d runs)", formatDuration(t.PerCall), t.Runs))

	vm.setInput(x, res, c)
	vm.setOutput(res, c)
}

// formatDuration writes a duration with three significant digits in the unit
// that suits it.
func formatDuration(d time.Duration) string {
	s := d.Seconds()
	switch {
	case s < 1e-6:
		return fmt.Sprintf("%.3g ns", s*1e9)
	case s < 1e-3:
		return fmt.Sprintf("%.3g µs", s*1e6)
	case s < 1:
		return fmt.Sprintf("%.3g ms", s*1e3)
	}
	return fmt.Sprintf("%.3g s", s)
}

// style draws a short series as stems and a long one as a line.
func style(n int) CurveStyle {
	if n <= stemLimit {
		return CurveStem
	}
	return CurveLine
}

// setInput publishes the input plot: the signal, and the windowed signal
// when a window is on; for IRFFT, the half spectrum it reads.
func (vm *ViewModel) setInput(x []float64, res Result, c Config) {
	vm.InputCurves.Clear()
	if c.Transform == TransformIRFFT {
		k := make([]float64, len(res.Input))
		for i := range k {
			k[i] = float64(i)
		}
		vm.InputAxis.Set("bin k (the first N/2+1 values, read as a half spectrum)")
		vm.InputCurves.Append(Curve{Label: "X[k]", X: k, Y: res.Input, Style: style(len(k))})
		return
	}
	t := make([]float64, len(x))
	name := "time (s)"
	for i := range t {
		t[i] = float64(i) / c.Fs
		if c.Transform.Inverse() {
			t[i] = float64(i)
		}
	}
	if c.Transform.Inverse() {
		name = "index n (the input is read as a spectrum)"
	}
	vm.InputAxis.Set(name)
	if c.Window == WindowNone {
		vm.InputCurves.Append(Curve{Label: "x", X: t, Y: x, Style: style(len(x))})
		return
	}
	vm.InputCurves.Append(
		Curve{Label: "x", X: t, Y: x, Style: CurveLine},
		Curve{Label: "x · " + WindowNames[c.Window] + " (transformed)", X: t, Y: res.Input, Style: style(len(x))},
	)
}

// setOutput publishes the result plot in the chosen view.
func (vm *ViewModel) setOutput(res Result, c Config) {
	vm.OutputCurves.Clear()
	vm.OutputAxis.Set(res.AxisName)
	name := TransformNames[c.Transform]
	if c.Typed() {
		name += "-" + []string{"I", "II", "III", "IV"}[c.Type-1]
	}
	st := style(len(res.Re))
	view := View(vm.View.Get())
	if res.Im == nil { // a real-valued result
		if view == ViewDecibel {
			vm.OutputTitle.Set(name + ": 20·log10|y|")
			vm.OutputCurves.Append(Curve{Label: "dB", X: res.Axis, Y: decibels(res.Re, nil), Style: CurveLine})
			return
		}
		vm.OutputTitle.Set(name + ": y (real)")
		vm.OutputCurves.Append(Curve{Label: "y", X: res.Axis, Y: res.Re, Style: st})
		return
	}
	switch view {
	case ViewMagnitude:
		vm.OutputTitle.Set(name + ": |y|")
		vm.OutputCurves.Append(Curve{Label: "|y|", X: res.Axis, Y: magnitude(res.Re, res.Im), Style: st})
	case ViewDecibel:
		vm.OutputTitle.Set(name + ": 20·log10|y|")
		vm.OutputCurves.Append(Curve{Label: "dB", X: res.Axis, Y: decibels(res.Re, res.Im), Style: CurveLine})
	case ViewPhase:
		vm.OutputTitle.Set(name + ": arg y (radians; blank where |y| is negligible)")
		vm.OutputCurves.Append(Curve{Label: "arg y", X: res.Axis, Y: phase(res.Re, res.Im), Style: st})
	default:
		vm.OutputTitle.Set(name + ": real and imaginary parts")
		vm.OutputCurves.Append(
			Curve{Label: "Re y", X: res.Axis, Y: res.Re, Style: CurveLine},
			Curve{Label: "Im y", X: res.Axis, Y: res.Im, Style: CurveLine},
		)
	}
}

func magnitude(re, im []float64) []float64 {
	m := make([]float64, len(re))
	for i := range re {
		m[i] = math.Hypot(re[i], im[i])
	}
	return m
}

// decibels is 20·log10|y|, floored 200 dB under the peak so an exact zero
// does not stretch the axis to minus infinity. im may be nil.
func decibels(re, im []float64) []float64 {
	if im == nil {
		im = make([]float64, len(re))
	}
	m := magnitude(re, im)
	peak := 0.0
	for _, v := range m {
		peak = math.Max(peak, v)
	}
	floor := math.Inf(-1)
	if peak > 0 {
		floor = 20*math.Log10(peak) - 200
	}
	for i, v := range m {
		m[i] = math.Max(20*math.Log10(v), floor)
	}
	return m
}

// phase is arg y, NaN (a gap in the plot) where |y| is below a billionth of
// the peak: the angle of rounding noise is noise.
func phase(re, im []float64) []float64 {
	m := magnitude(re, im)
	peak := 0.0
	for _, v := range m {
		peak = math.Max(peak, v)
	}
	p := make([]float64, len(re))
	for i := range re {
		p[i] = math.NaN()
		if m[i] > peak*1e-9 {
			p[i] = cmplx.Phase(complex(re[i], im[i]))
		}
	}
	return p
}
