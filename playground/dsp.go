// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package playground is the go-fft WebAssembly playground: a go-widgets canvas
// application in which a visitor builds a signal, picks a transform and sees
// what github.com/go-fft/fft makes of it — computed by go-fft itself, compiled
// to WebAssembly, with no JavaScript FFT anywhere.
//
// The package is tagless and native-testable. dsp.go is the model (signals,
// transforms, the round trip and the timing), codegen.go writes the Go and the
// numpy equivalent of a configuration, viewmodel.go holds every piece of UI
// state as go-widgets/mvvm observables, and scene.go is the View: toolkit
// widgets bound to the view-model. cmd/playground-wasm is the thin js/wasm shell.
package playground

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/go-fft/fft"
)

// Preset names a built-in input signal.
type Preset int

const (
	PresetSines Preset = iota
	PresetChirp
	PresetSquare
	PresetNoise
	PresetImpulse
	PresetCustom
)

// PresetNames are the presets in Preset order, as the picker lists them.
var PresetNames = []string{"Sum of two sines", "Chirp f1 → f2", "Square wave", "Noise + tone", "Impulse", "Custom samples"}

// SignalParams describes a generated signal. N samples taken at Fs Hz; F1, F2,
// A1 and A2 are read by each preset as its doc says.
type SignalParams struct {
	Preset Preset
	N      int
	Fs     float64
	F1, F2 float64
	A1, A2 float64
	Seed   uint64
}

// Generate returns the N samples of a preset signal:
//
//   - sines:   A1·sin(2π·F1·t) + A2·sin(2π·F2·t)
//   - chirp:   A1·sin(2π·(F1·t + (F2−F1)·t²/(2T))), a linear sweep over T = N/Fs
//   - square:  ±A1 at F1 (the sign of sin(2π·F1·t), +A1 at zero)
//   - noise:   Gaussian noise of standard deviation A1, plus A2·sin(2π·F1·t)
//   - impulse: A1 at n = 0, zero elsewhere
//
// with t = n/Fs. The noise is seeded, so a configuration always draws the same
// samples. PresetCustom has no generator: it returns nil.
func Generate(p SignalParams) []float64 {
	if p.Preset == PresetCustom || p.N <= 0 {
		return nil
	}
	x := make([]float64, p.N)
	rng := rand.New(rand.NewPCG(p.Seed, 0x9e3779b97f4a7c15))
	T := float64(p.N) / p.Fs
	for n := range x {
		t := float64(n) / p.Fs
		switch p.Preset {
		case PresetSines:
			x[n] = p.A1*math.Sin(2*math.Pi*p.F1*t) + p.A2*math.Sin(2*math.Pi*p.F2*t)
		case PresetChirp:
			x[n] = p.A1 * math.Sin(2*math.Pi*(p.F1*t+(p.F2-p.F1)*t*t/(2*T)))
		case PresetSquare:
			x[n] = p.A1
			if math.Sin(2*math.Pi*p.F1*t) < 0 {
				x[n] = -p.A1
			}
		case PresetNoise:
			x[n] = p.A1*rng.NormFloat64() + p.A2*math.Sin(2*math.Pi*p.F1*t)
		case PresetImpulse:
			if n == 0 {
				x[n] = p.A1
			}
		}
	}
	return x
}

// MaxN is the longest signal the playground accepts: long enough to show that
// a big transform is fast, short enough that a careless paste cannot hang a tab.
const MaxN = 1 << 20

// ParseSamples reads numbers separated by commas, semicolons, spaces or new
// lines — what a pasted numpy array, CSV column or Go slice literal looks like.
// Brackets, braces and a leading "[]float64" are ignored, so `[1, 2, 3]` and
// `[]float64{1, 2, 3}` both parse. A line starting with # is a comment.
func ParseSamples(s string) ([]float64, error) {
	var out []float64
	for ln, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.NewReplacer("[]float64", " ", "[", " ", "]", " ", "{", " ", "}", " ", "(", " ", ")", " ").Replace(line)
		for _, f := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r'
		}) {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: %q is not a number", ln+1, f)
			}
			if len(out) == MaxN {
				return nil, fmt.Errorf("more than %d samples", MaxN)
			}
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no samples")
	}
	return out, nil
}

// FormatSamples writes samples back as text ParseSamples reads, eight to a
// line, each with the shortest representation that round-trips.
func FormatSamples(x []float64) string {
	var b strings.Builder
	for i, v := range x {
		if i > 0 {
			if i%8 == 0 {
				b.WriteString(",\n")
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString(strconv.FormatFloat(v, 'g', 6, 64))
	}
	return b.String()
}

// Transform names a transform the playground runs.
type Transform int

const (
	TransformFFT Transform = iota
	TransformIFFT
	TransformRFFT
	TransformIRFFT
	TransformDCT
	TransformIDCT
	TransformDST
	TransformIDST
)

// TransformNames are the transforms in Transform order, as the picker lists them.
var TransformNames = []string{"FFT", "IFFT", "RFFT", "IRFFT", "DCT", "IDCT", "DST", "IDST"}

// Inverse reports whether t maps a spectrum back to samples.
func (t Transform) Inverse() bool {
	return t == TransformIFFT || t == TransformIRFFT || t == TransformIDCT || t == TransformIDST
}

// Typed reports whether t takes a DCT/DST type (I to IV).
func (t Transform) Typed() bool { return t >= TransformDCT }

// ComplexOut reports whether t returns complex values.
func (t Transform) ComplexOut() bool {
	return t == TransformFFT || t == TransformIFFT || t == TransformRFFT
}

// Window names a window applied to the input before the transform.
type Window int

const (
	WindowNone Window = iota
	WindowHann
	WindowHamming
	WindowBlackman
	WindowBlackmanHarris
	WindowBartlett
)

// WindowNames are the windows in Window order.
var WindowNames = []string{"None (rectangular)", "Hann", "Hamming", "Blackman", "Blackman–Harris", "Bartlett"}

// Coefficients returns the window's n coefficients, from go-fft; nil for none.
func (w Window) Coefficients(n int) []float64 {
	switch w {
	case WindowHann:
		return fft.Hann(n)
	case WindowHamming:
		return fft.Hamming(n)
	case WindowBlackman:
		return fft.Blackman(n)
	case WindowBlackmanHarris:
		return fft.BlackmanHarris(n)
	case WindowBartlett:
		return fft.Bartlett(n)
	}
	return nil
}

// NormNames are numpy's names for the three fft.Norm values, in Norm order.
var NormNames = []string{"backward", "ortho", "forward"}

// Precision is the floating-point width the transform runs in.
type Precision int

const (
	Float64 Precision = iota
	Float32
)

// PrecisionNames are the precisions in Precision order.
var PrecisionNames = []string{"float64 (complex128)", "float32 (complex64)"}

// Config is everything that decides what transform runs on the input.
type Config struct {
	Transform Transform
	Type      int // DCT/DST type, 1..4
	Norm      fft.Norm
	Window    Window
	Precision Precision
	Shift     bool // FFTShift the FFT's output and its frequencies
	Fs        float64
}

// Result is one run of a transform.
type Result struct {
	// Input is what was transformed: the samples after the window, or for
	// IRFFT the N/2+1 values taken as a half spectrum.
	Input []float64
	// Re and Im are the output; Im is nil for a real-valued transform.
	Re, Im []float64
	// Axis is the abscissa of each output value, and AxisName its unit:
	// frequency in Hz for a forward FFT/RFFT, time in seconds for the
	// inverse FFTs, the coefficient index k for the DCT and DST.
	Axis     []float64
	AxisName string
	// RoundTrip is the largest |input − inverse(forward(input))|, the
	// inverse being run with the same Norm and precision.
	RoundTrip float64
}

// Validate reports why c cannot run on n samples, or nil.
func (c Config) Validate(n int) error {
	switch {
	case n < 1:
		return errors.New("the signal is empty")
	case c.Typed() && (c.Type < 1 || c.Type > 4):
		return fmt.Errorf("type %d: the DCT and DST have types 1 to 4", c.Type)
	case c.Typed() && c.Type == 1 && n < 2 && (c.Transform == TransformDCT || c.Transform == TransformIDCT):
		return errors.New("the DCT-I needs at least 2 samples")
	case c.Transform == TransformIRFFT && n < 2:
		return errors.New("IRFFT needs at least 2 output samples")
	case c.Fs <= 0 || math.IsNaN(c.Fs) || math.IsInf(c.Fs, 0):
		return errors.New("the sample rate must be a positive number")
	}
	return nil
}

// Typed reports whether the configured transform takes a type.
func (c Config) Typed() bool { return c.Transform.Typed() }

// Run transforms x as c says, then inverts the result to measure the round
// trip. x is not modified.
func Run(x []float64, c Config) (Result, error) {
	if err := c.Validate(len(x)); err != nil {
		return Result{}, err
	}
	n := len(x)
	in := prepare(x, c)
	var r Result
	r.Input = in
	op := operation(in, n, c)
	out := op.forward()
	back := op.inverse(out)
	r.RoundTrip = maxAbsDiff(op.reference, back)
	if c.Transform.ComplexOut() {
		r.Re, r.Im = make([]float64, len(out.c)), make([]float64, len(out.c))
		for i, v := range out.c {
			r.Re[i], r.Im[i] = real(v), imag(v)
		}
	} else {
		r.Re = out.r
	}
	r.Axis, r.AxisName = axis(len(r.Re), n, c)
	if c.Shift && c.Transform == TransformFFT {
		r.Re, r.Im = fft.FFTShift(r.Re), fft.FFTShift(r.Im)
	}
	return r, nil
}

// prepare applies the window (to a copy), or for IRFFT keeps the N/2+1 values
// it reads as a half spectrum.
func prepare(x []float64, c Config) []float64 {
	n := len(x)
	if c.Transform == TransformIRFFT {
		return append([]float64(nil), x[:n/2+1]...)
	}
	in := append([]float64(nil), x...)
	for i, w := range c.Window.Coefficients(n) {
		in[i] *= w
	}
	return in
}

// axis is the output's abscissa (see Result.Axis).
func axis(m, n int, c Config) ([]float64, string) {
	switch c.Transform {
	case TransformFFT:
		f := fft.FFTFreq(n, 1/c.Fs)
		if c.Shift {
			f = fft.FFTShift(f)
		}
		return f, "frequency (Hz)"
	case TransformRFFT:
		return fft.RFFTFreq(n, 1/c.Fs), "frequency (Hz)"
	case TransformIFFT, TransformIRFFT:
		t := make([]float64, m)
		for i := range t {
			t[i] = float64(i) / c.Fs
		}
		return t, "time (s)"
	}
	k := make([]float64, m)
	for i := range k {
		k[i] = float64(i)
	}
	return k, "coefficient k"
}

// value is a transform's output: complex (c) or real (r), held in float64
// whatever precision computed it.
type value struct {
	c []complex128
	r []float64
}

// op is one configured transform: forward runs it, inverse undoes it, and
// reference is what inverse(forward()) should give back.
type op struct {
	forward   func() value
	inverse   func(value) value
	reference value
}

func toComplex(x []float64) []complex128 {
	c := make([]complex128, len(x))
	for i, v := range x {
		c[i] = complex(v, 0)
	}
	return c
}

func to32(x []float64) []float32 {
	f := make([]float32, len(x))
	for i, v := range x {
		f[i] = float32(v)
	}
	return f
}

func from32(x []float32) []float64 {
	f := make([]float64, len(x))
	for i, v := range x {
		f[i] = float64(v)
	}
	return f
}

func c64(x []complex128) []complex64 {
	c := make([]complex64, len(x))
	for i, v := range x {
		c[i] = complex64(v)
	}
	return c
}

func c128(x []complex64) []complex128 {
	c := make([]complex128, len(x))
	for i, v := range x {
		c[i] = complex128(v)
	}
	return c
}

// operation binds the configured transform, in the configured precision, to
// its input.
func operation(in []float64, n int, c Config) op {
	o := fft.Options{Norm: c.Norm}
	single := c.Precision == Float32
	switch c.Transform {
	case TransformFFT, TransformIFFT:
		fwd, inv := fft.FFTWith, fft.IFFTWith
		fwd32, inv32 := fft.FFT32With, fft.IFFT32With
		if c.Transform == TransformIFFT {
			fwd, inv, fwd32, inv32 = inv, fwd, inv32, fwd32
		}
		xc := toComplex(in)
		if single {
			x32 := c64(xc)
			return op{
				forward:   func() value { return value{c: c128(fwd32(x32, o))} },
				inverse:   func(v value) value { return value{c: c128(inv32(c64(v.c), o))} },
				reference: value{c: xc},
			}
		}
		return op{
			forward:   func() value { return value{c: fwd(xc, o)} },
			inverse:   func(v value) value { return value{c: inv(v.c, o)} },
			reference: value{c: xc},
		}
	case TransformRFFT:
		on := fft.Options{N: n, Norm: c.Norm}
		if single {
			x32 := to32(in)
			return op{
				forward:   func() value { return value{c: c128(fft.RFFT32With(x32, o))} },
				inverse:   func(v value) value { return value{r: from32(fft.IRFFT32With(c64(v.c), on))} },
				reference: value{r: in},
			}
		}
		return op{
			forward:   func() value { return value{c: fft.RFFTWith(in, o)} },
			inverse:   func(v value) value { return value{r: fft.IRFFTWith(v.c, on)} },
			reference: value{r: in},
		}
	case TransformIRFFT:
		on := fft.Options{N: n, Norm: c.Norm}
		spec := toComplex(in)
		if single {
			s32 := c64(spec)
			return op{
				forward:   func() value { return value{r: from32(fft.IRFFT32With(s32, on))} },
				inverse:   func(v value) value { return value{c: c128(fft.RFFT32With(to32(v.r), o))} },
				reference: value{c: spec},
			}
		}
		return op{
			forward:   func() value { return value{r: fft.IRFFTWith(spec, on)} },
			inverse:   func(v value) value { return value{c: fft.RFFTWith(v.r, o)} },
			reference: value{c: spec},
		}
	}
	// The real-to-real transforms: a forward/inverse pair per family.
	fwd, inv := fft.DCT, fft.IDCT
	fwd32, inv32 := fft.DCT32, fft.IDCT32
	if c.Transform == TransformDST || c.Transform == TransformIDST {
		fwd, inv, fwd32, inv32 = fft.DST, fft.IDST, fft.DST32, fft.IDST32
	}
	if c.Transform == TransformIDCT || c.Transform == TransformIDST {
		fwd, inv, fwd32, inv32 = inv, fwd, inv32, fwd32
	}
	if single {
		x32 := to32(in)
		return op{
			forward:   func() value { return value{r: from32(fwd32(x32, c.Type, c.Norm))} },
			inverse:   func(v value) value { return value{r: from32(inv32(to32(v.r), c.Type, c.Norm))} },
			reference: value{r: in},
		}
	}
	return op{
		forward:   func() value { return value{r: fwd(in, c.Type, c.Norm)} },
		inverse:   func(v value) value { return value{r: inv(v.r, c.Type, c.Norm)} },
		reference: value{r: in},
	}
}

// maxAbsDiff is the largest elementwise distance between two values of the
// same kind and length.
func maxAbsDiff(a, b value) float64 {
	m := 0.0
	for i := range a.c {
		m = math.Max(m, cmplx.Abs(a.c[i]-b.c[i]))
	}
	for i := range a.r {
		m = math.Max(m, math.Abs(a.r[i]-b.r[i]))
	}
	return m
}

// Timing is how long one transform took, measured over Runs repetitions.
type Timing struct {
	PerCall time.Duration
	Runs    int
}

// Time runs the forward transform of c on x repeatedly — doubling the count
// until the batch takes at least budget — and reports the time per call. A
// single call to a tiny transform is far below the browser clock's resolution
// (performance.now is coarsened to 0.1 ms or worse), so one run would measure
// the clock, not the transform. now is the clock (time.Now in production).
func Time(x []float64, c Config, budget time.Duration, now func() time.Time) (Timing, error) {
	if err := c.Validate(len(x)); err != nil {
		return Timing{}, err
	}
	f := operation(prepare(x, c), len(x), c).forward
	f() // warm up: first-call plan building is not the transform
	for runs := 1; ; runs *= 2 {
		start := now()
		for range runs {
			f()
		}
		el := now().Sub(start)
		if el >= budget || runs >= 1<<20 {
			return Timing{PerCall: el / time.Duration(runs), Runs: runs}, nil
		}
	}
}

// Factor describes n as a product of primes ("2^4 · 3^4", "prime") — which
// decides how go-fft takes it: lengths whose prime factors are all at most
// 13 run on the mixed-radix Stockham engine, the others through Rader's or
// Bluestein's algorithm.
func Factor(n int) string {
	if n < 2 {
		return strconv.Itoa(n)
	}
	var parts []string
	m := n
	for p := 2; p*p <= m; p++ {
		k := 0
		for m%p == 0 {
			m /= p
			k++
		}
		if k == 1 {
			parts = append(parts, strconv.Itoa(p))
		} else if k > 1 {
			parts = append(parts, fmt.Sprintf("%d^%d", p, k))
		}
	}
	if m == n {
		return "prime"
	}
	if m > 1 {
		parts = append(parts, strconv.Itoa(m))
	}
	return strings.Join(parts, "·")
}

// Smooth reports whether every prime factor of n is at most 13 — a length the
// Stockham engine takes directly.
func Smooth(n int) bool {
	for _, p := range []int{2, 3, 5, 7, 11, 13} {
		for n > 1 && n%p == 0 {
			n /= p
		}
	}
	return n <= 1
}
