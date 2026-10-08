// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"math"
	"math/cmplx"
	"strings"
	"testing"
	"time"

	"github.com/go-fft/fft"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestGenerate(t *testing.T) {
	base := SignalParams{N: 8, Fs: 8, F1: 1, F2: 2, A1: 2, A2: 1, Seed: 1}
	p := base
	p.Preset = PresetSines
	x := Generate(p)
	// t = 2/8: sin(π/2)·2 + sin(π)·1 = 2.
	if len(x) != 8 || !near(x[2], 2, 1e-12) || x[0] != 0 {
		t.Errorf("sines: %v", x)
	}
	p.Preset = PresetChirp
	if x := Generate(p); x[0] != 0 || len(x) != 8 {
		t.Errorf("chirp: %v", x)
	}
	p.Preset = PresetSquare
	x = Generate(p)
	if x[1] != 2 || x[5] != -2 || x[0] != 2 {
		t.Errorf("square: %v", x)
	}
	p.Preset = PresetNoise
	a, b := Generate(p), Generate(p)
	if a[3] != b[3] || a[3] == 0 {
		t.Errorf("noise is not seeded: %v %v", a[3], b[3])
	}
	p.Preset = PresetImpulse
	x = Generate(p)
	if x[0] != 2 || x[1] != 0 {
		t.Errorf("impulse: %v", x)
	}
	p.Preset = PresetCustom
	if Generate(p) != nil {
		t.Error("custom generated samples")
	}
	p.Preset, p.N = PresetSines, 0
	if Generate(p) != nil {
		t.Error("N = 0 generated samples")
	}
}

func TestParseSamples(t *testing.T) {
	for _, in := range []string{
		"1, 2, 3",
		"[1. 2. 3.]",
		"[]float64{1, 2, 3}",
		"# a comment\n1\n2 ; 3 # trailing",
		"(1,\t2,\r\n3)",
	} {
		x, err := ParseSamples(in)
		if err != nil || len(x) != 3 || x[2] != 3 {
			t.Errorf("ParseSamples(%q) = %v, %v", in, x, err)
		}
	}
	if _, err := ParseSamples("1, two"); err == nil || !strings.Contains(err.Error(), `"two"`) {
		t.Errorf("a word parsed: %v", err)
	}
	if _, err := ParseSamples("# nothing"); err == nil {
		t.Error("no samples parsed")
	}
	if _, err := ParseSamples(strings.Repeat("0 ", MaxN+1)); err == nil {
		t.Error("more than MaxN samples parsed")
	}
	x := []float64{0.1, -2, 3e-9, 4, 5, 6, 7, 8, 9}
	back, err := ParseSamples(FormatSamples(x))
	if err != nil || len(back) != len(x) || back[2] != 3e-9 {
		t.Errorf("FormatSamples does not round-trip: %v %v", back, err)
	}
	if !strings.Contains(FormatSamples(x), ",\n9") {
		t.Error("eight to a line")
	}
}

func TestTransformPredicates(t *testing.T) {
	for tr := TransformFFT; tr <= TransformIDST; tr++ {
		inv := tr == TransformIFFT || tr == TransformIRFFT || tr == TransformIDCT || tr == TransformIDST
		if tr.Inverse() != inv {
			t.Errorf("%s.Inverse() = %v", TransformNames[tr], tr.Inverse())
		}
		if tr.Typed() != (tr >= TransformDCT) {
			t.Errorf("%s.Typed()", TransformNames[tr])
		}
		if tr.ComplexOut() != (tr <= TransformRFFT && tr != TransformIRFFT) {
			t.Errorf("%s.ComplexOut()", TransformNames[tr])
		}
	}
}

func TestWindowCoefficients(t *testing.T) {
	if WindowNone.Coefficients(8) != nil {
		t.Error("no window has coefficients")
	}
	want := map[Window][]float64{
		WindowHann: fft.Hann(9), WindowHamming: fft.Hamming(9), WindowBlackman: fft.Blackman(9),
		WindowBlackmanHarris: fft.BlackmanHarris(9), WindowBartlett: fft.Bartlett(9),
	}
	for w, c := range want {
		if got := w.Coefficients(9); got[4] != c[4] || len(got) != 9 {
			t.Errorf("%s: %v", WindowNames[w], got)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Config{Fs: 1, Type: 2}
	cases := []struct {
		c    Config
		n    int
		want string
	}{
		{ok, 0, "empty"},
		{Config{Transform: TransformDCT, Type: 5, Fs: 1}, 4, "types 1 to 4"},
		{Config{Transform: TransformIDCT, Type: 1, Fs: 1}, 1, "DCT-I"},
		{Config{Transform: TransformIRFFT, Fs: 1}, 1, "IRFFT"},
		{Config{Fs: 0}, 4, "sample rate"},
		{Config{Fs: math.NaN()}, 4, "sample rate"},
	}
	for _, tc := range cases {
		err := tc.c.Validate(tc.n)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%+v, %d) = %v, want %q", tc.c, tc.n, err, tc.want)
		}
		if _, err := Run(make([]float64, tc.n), tc.c); err == nil {
			t.Errorf("Run accepted %+v", tc.c)
		}
		if _, err := Time(make([]float64, tc.n), tc.c, time.Millisecond, time.Now); err == nil {
			t.Errorf("Time accepted %+v", tc.c)
		}
	}
	if err := (Config{Transform: TransformDST, Type: 1, Fs: 1}).Validate(1); err != nil {
		t.Errorf("DST-I of one sample refused: %v", err)
	}
}

// TestRunMatchesGoFFT: for every transform, precision and norm, the output is
// what the go-fft call computes, and the round trip is at rounding level.
func TestRunMatchesGoFFT(t *testing.T) {
	x := Generate(SignalParams{Preset: PresetNoise, N: 30, Fs: 30, F1: 3, A1: 1, A2: 1, Seed: 7})
	for tr := TransformFFT; tr <= TransformIDST; tr++ {
		for _, prec := range []Precision{Float64, Float32} {
			for norm := fft.NormBackward; norm <= fft.NormForward; norm++ {
				c := Config{Transform: tr, Type: 2, Norm: norm, Precision: prec, Fs: 30}
				r, err := Run(x, c)
				if err != nil {
					t.Fatalf("%s: %v", TransformNames[tr], err)
				}
				tol := 1e-12
				if prec == Float32 {
					tol = 1e-4
				}
				if r.RoundTrip > tol*10 {
					t.Errorf("%s %s %s: round trip %g", TransformNames[tr], PrecisionNames[prec], NormNames[norm], r.RoundTrip)
				}
				want := reference(x, c)
				if len(want) != len(r.Re) {
					t.Fatalf("%s: %d values, want %d", TransformNames[tr], len(r.Re), len(want))
				}
				for i, w := range want {
					got := complex(r.Re[i], 0)
					if r.Im != nil {
						got = complex(r.Re[i], r.Im[i])
					}
					if cmplx.Abs(got-w) > tol*(1+cmplx.Abs(w))*10 {
						t.Fatalf("%s %s %s [%d] = %v, want %v", TransformNames[tr], PrecisionNames[prec], NormNames[norm], i, got, w)
					}
				}
				if len(r.Axis) != len(r.Re) || r.AxisName == "" {
					t.Errorf("%s: axis %d %q", TransformNames[tr], len(r.Axis), r.AxisName)
				}
			}
		}
	}
}

// reference computes c on x with the float64 go-fft calls directly.
func reference(x []float64, c Config) []complex128 {
	o := fft.Options{Norm: c.Norm}
	cx := func(r []float64) []complex128 { return toComplex(r) }
	switch c.Transform {
	case TransformFFT:
		return fft.FFTWith(cx(x), o)
	case TransformIFFT:
		return fft.IFFTWith(cx(x), o)
	case TransformRFFT:
		return fft.RFFTWith(x, o)
	case TransformIRFFT:
		return cx(fft.IRFFTWith(cx(x[:len(x)/2+1]), fft.Options{N: len(x), Norm: c.Norm}))
	case TransformDCT:
		return cx(fft.DCT(x, c.Type, c.Norm))
	case TransformIDCT:
		return cx(fft.IDCT(x, c.Type, c.Norm))
	case TransformDST:
		return cx(fft.DST(x, c.Type, c.Norm))
	}
	return cx(fft.IDST(x, c.Type, c.Norm))
}

func TestRunAxesAndWindow(t *testing.T) {
	x := []float64{1, 2, 3, 4}
	r, _ := Run(x, Config{Transform: TransformFFT, Fs: 4, Shift: true})
	if r.Axis[0] != -2 || r.Axis[2] != 0 || r.Re[2] != 10 {
		t.Errorf("shifted FFT: axis %v re %v", r.Axis, r.Re)
	}
	r, _ = Run(x, Config{Transform: TransformRFFT, Fs: 4})
	if len(r.Axis) != 3 || r.Axis[2] != 2 || r.AxisName != "frequency (Hz)" {
		t.Errorf("RFFT axis %v %q", r.Axis, r.AxisName)
	}
	r, _ = Run(x, Config{Transform: TransformIFFT, Fs: 4})
	if r.Axis[1] != 0.25 || r.AxisName != "time (s)" {
		t.Errorf("IFFT axis %v %q", r.Axis, r.AxisName)
	}
	r, _ = Run(x, Config{Transform: TransformDCT, Type: 2, Fs: 4, Window: WindowHann})
	if r.Axis[3] != 3 || r.Input[0] != 0 || r.Input[1] == 2 {
		t.Errorf("DCT axis %v, windowed input %v", r.Axis, r.Input)
	}
	if x[0] != 1 {
		t.Error("Run modified its input")
	}
	r, _ = Run(x, Config{Transform: TransformIRFFT, Fs: 4})
	if len(r.Input) != 3 || len(r.Re) != 4 {
		t.Errorf("IRFFT reads N/2+1 bins and writes N: %d %d", len(r.Input), len(r.Re))
	}
}

func TestTime(t *testing.T) {
	x := make([]float64, 16)
	var clock time.Time
	tick := func() time.Time { clock = clock.Add(time.Millisecond); return clock }
	// Each batch reads the clock twice, one tick apart: every batch "takes"
	// 1 ms, so a 1 ms budget is met by the first batch of one run.
	tm, err := Time(x, Config{Transform: TransformRFFT, Fs: 1}, time.Millisecond, tick)
	if err != nil || tm.Runs != 1 || tm.PerCall != time.Millisecond {
		t.Errorf("Time = %+v, %v", tm, err)
	}
	// A clock that never moves stops at the cap.
	still := func() time.Time { return clock }
	tm, _ = Time([]float64{1, 2}, Config{Transform: TransformFFT, Fs: 1}, time.Second, still)
	if tm.Runs != 1<<20 || tm.PerCall != 0 {
		t.Errorf("stopped clock: %+v", tm)
	}
}

func TestFactor(t *testing.T) {
	for n, want := range map[int]string{1: "1", 2: "prime", 12: "2^2·3", 1296: "2^4·3^4", 1009: "prime", 2018: "2·1009", 1000: "2^3·5^3"} {
		if got := Factor(n); got != want {
			t.Errorf("Factor(%d) = %q, want %q", n, got, want)
		}
	}
	if !Smooth(1296) || Smooth(1009) || !Smooth(1) || Smooth(17*2) {
		t.Error("Smooth")
	}
}
