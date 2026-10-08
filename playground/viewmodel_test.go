// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"math"
	"strings"
	"testing"
	"time"
)

// newTestVM is a view-model whose timing does not wait on a real clock.
func newTestVM(t *testing.T) *ViewModel {
	t.Helper()
	vm := NewViewModel()
	vm.Budget = 0
	return vm
}

func TestViewModelOpening(t *testing.T) {
	vm := newTestVM(t)
	if vm.Problem.Get() != "" {
		t.Fatalf("problem at start: %s", vm.Problem.Get())
	}
	out := vm.OutputCurves.Slice()
	if len(out) != 1 || len(out[0].Y) != 501 || out[0].Style != CurveLine {
		t.Fatalf("opening output: %+v", out)
	}
	// The spectrum peaks at the two tones: bins 50 and 120 of 1 Hz each.
	y := out[0].Y
	if y[50] < 100*y[80] || y[120] < 50*y[80] {
		t.Errorf("no peaks at 50 and 120 Hz: %v %v %v", y[50], y[120], y[80])
	}
	if in := vm.InputCurves.Slice(); len(in) != 2 || in[0].Label != "x" {
		t.Errorf("input curves: %d", len(in))
	}
	for name, s := range map[string]string{"length": vm.Length.Get(), "accuracy": vm.Accuracy.Get(), "speed": vm.Speed.Get(), "go": vm.GoCode.Get(), "numpy": vm.NumpyCode.Get()} {
		if s == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if !strings.Contains(vm.Length.Get(), "N = 1000 = 2^3·5^3 · Stockham") {
		t.Errorf("length: %s", vm.Length.Get())
	}
	if !strings.HasPrefix(vm.Samples.Get(), "0, ") {
		t.Errorf("samples not listed: %.40s", vm.Samples.Get())
	}
}

func TestViewModelEveryTransformAndView(t *testing.T) {
	vm := newTestVM(t)
	vm.N.Set("64")
	for tr := range TransformNames {
		vm.Transform.Set(tr)
		for v := range ViewNames {
			vm.View.Set(v)
			if vm.Problem.Get() != "" {
				t.Fatalf("%s/%s: %s", TransformNames[tr], ViewNames[v], vm.Problem.Get())
			}
			out := vm.OutputCurves.Slice()
			if len(out) == 0 || vm.OutputTitle.Get() == "" {
				t.Fatalf("%s/%s: no output", TransformNames[tr], ViewNames[v])
			}
			if v == int(ViewReIm) && Transform(tr).ComplexOut() && len(out) != 2 {
				t.Errorf("%s re/im: %d curves", TransformNames[tr], len(out))
			}
		}
	}
	// Short outputs are stems; a typed transform names its type.
	vm.Transform.Set(int(TransformDST))
	vm.Type.Set(3)
	vm.View.Set(int(ViewMagnitude))
	if !strings.HasPrefix(vm.OutputTitle.Get(), "DST-IV") || vm.OutputCurves.At(0).Style != CurveStem {
		t.Errorf("DST-IV: %q %v", vm.OutputTitle.Get(), vm.OutputCurves.At(0).Style)
	}
}

func TestViewModelInputAxes(t *testing.T) {
	vm := newTestVM(t)
	vm.N.Set("16")
	vm.Window.Set(int(WindowNone))
	if in := vm.InputCurves.Slice(); len(in) != 1 || in[0].X[1] != 1.0/1000 {
		t.Errorf("forward input: %+v", in)
	}
	vm.Transform.Set(int(TransformIFFT))
	if !strings.HasPrefix(vm.InputAxis.Get(), "index n") || vm.InputCurves.At(0).X[3] != 3 {
		t.Errorf("inverse input axis %q", vm.InputAxis.Get())
	}
	vm.Transform.Set(int(TransformIRFFT))
	if in := vm.InputCurves.At(0); len(in.Y) != 9 || !strings.HasPrefix(vm.InputAxis.Get(), "bin k") {
		t.Errorf("IRFFT input: %d values, %q", len(in.Y), vm.InputAxis.Get())
	}
}

func TestViewModelProblems(t *testing.T) {
	vm := newTestVM(t)
	for _, tc := range []struct {
		o    func(string)
		v    string
		want string
	}{
		{vm.N.Set, "zero", "N:"},
		{vm.N.Set, "0", "N:"},
		{vm.Fs.Set, "-1", "fs:"},
		{vm.Fs.Set, "x", "fs:"},
		{vm.F1.Set, "Inf", "f1:"},
		{vm.A2.Set, "", "a2:"},
	} {
		tc.o(tc.v)
		if !strings.HasPrefix(vm.Problem.Get(), tc.want) || vm.OutputCurves.Len() != 0 {
			t.Errorf("%q: problem %q, %d curves", tc.v, vm.Problem.Get(), vm.OutputCurves.Len())
		}
		vm.N.Set("1000")
		vm.Fs.Set("1000")
		vm.F1.Set("50")
		vm.A2.Set("0.5")
	}
	if vm.Problem.Get() != "" {
		t.Fatalf("not recovered: %s", vm.Problem.Get())
	}
	// A transform that cannot run on the signal: the DCT-I of one sample.
	vm.N.Set("1")
	vm.Transform.Set(int(TransformDCT))
	vm.Type.Set(0)
	if !strings.Contains(vm.Problem.Get(), "DCT-I") {
		t.Errorf("DCT-I of one sample: %q", vm.Problem.Get())
	}
	// A bad sample rate seen by a transform change, with the signal valid.
	vm.N.Set("8")
	vm.Preset.Set(int(PresetCustom))
	vm.Fs.Set("0")
	if !strings.HasPrefix(vm.Problem.Get(), "fs:") {
		t.Errorf("custom with fs = 0: %q", vm.Problem.Get())
	}
}

func TestViewModelCustomSamples(t *testing.T) {
	vm := newTestVM(t)
	vm.Samples.Set("1, 2, 3, 4, 5")
	if vm.Preset.Get() != int(PresetCustom) || vm.N.Get() != "5" || vm.Problem.Get() != "" {
		t.Fatalf("custom: preset %d N %s problem %q", vm.Preset.Get(), vm.N.Get(), vm.Problem.Get())
	}
	if !strings.Contains(vm.Length.Get(), "N = 5 = prime · Stockham · NextFastLen = 6") {
		t.Errorf("length: %s", vm.Length.Get())
	}
	vm.Transform.Set(int(TransformFFT))
	if strings.Contains(vm.Length.Get(), "NextFastLen") {
		t.Errorf("5 is a fast complex length: %s", vm.Length.Get())
	}
	vm.Samples.Set("1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17")
	if !strings.Contains(vm.Length.Get(), "N = 17 = prime · Rader or Bluestein · NextFastLen = 18") {
		t.Errorf("complex next fast length: %s", vm.Length.Get())
	}
	vm.Samples.Set("1, 2, 3, 4, 5")
	// Editing a parameter while Custom keeps the visitor's samples.
	vm.F1.Set("7")
	if vm.N.Get() != "5" {
		t.Errorf("a parameter edit replaced the custom samples: N = %s", vm.N.Get())
	}
	vm.Samples.Set("1, x")
	if !strings.HasPrefix(vm.Problem.Get(), "samples:") {
		t.Errorf("bad samples: %q", vm.Problem.Get())
	}
	// Back to a preset regenerates.
	vm.Preset.Set(int(PresetImpulse))
	if vm.Problem.Get() != "" || !strings.HasPrefix(vm.Samples.Get(), "1, 0, 0") {
		t.Errorf("impulse: %q %.20s", vm.Problem.Get(), vm.Samples.Get())
	}
	// A long signal is not listed.
	vm.N.Set("5000")
	if !strings.HasPrefix(vm.Samples.Get(), "# 5000 samples") || vm.Preset.Get() != int(PresetImpulse) {
		t.Errorf("long signal listed: %.30s (preset %d)", vm.Samples.Get(), vm.Preset.Get())
	}
}

func TestViewModelCopy(t *testing.T) {
	vm := newTestVM(t)
	vm.CopyGo.Execute() // no clipboard: nothing happens
	var got string
	vm.Clipboard = func(s string) { got = s }
	vm.CopyGo.Execute()
	if got != vm.GoCode.Get() {
		t.Error("Copy Go")
	}
	vm.CopyNumpy.Execute()
	if got != vm.NumpyCode.Get() {
		t.Error("Copy numpy")
	}
}

func TestViewModelShiftAndPrecision(t *testing.T) {
	vm := newTestVM(t)
	vm.Transform.Set(int(TransformFFT))
	vm.Shift.Set(true)
	if x := vm.OutputCurves.At(0).X; x[0] != -500 {
		t.Errorf("shifted axis starts at %v", x[0])
	}
	vm.Precision.Set(int(Float32))
	if !strings.Contains(vm.GoCode.Get(), "FFT32") || vm.Problem.Get() != "" {
		t.Errorf("float32 code: %s", vm.GoCode.Get())
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		500: "500 ns", 12_340: "12.3 µs", 5_670_000: "5.67 ms", 2_500_000_000: "2.5 s",
	} {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%d) = %q, want %q", d, got, want)
		}
	}
}

func TestDecibelsAndPhase(t *testing.T) {
	db := decibels([]float64{10, 0, 1}, nil)
	if db[0] != 20 || db[1] != -180 || db[2] != 0 {
		t.Errorf("decibels: %v", db)
	}
	if z := decibels([]float64{0, 0}, []float64{0, 0}); !math.IsInf(z[0], -1) {
		t.Errorf("all-zero decibels: %v", z)
	}
	p := phase([]float64{0, 1, 1e-12}, []float64{1, 0, 0})
	if p[0] != math.Pi/2 || p[1] != 0 || !math.IsNaN(p[2]) {
		t.Errorf("phase: %v", p)
	}
}
