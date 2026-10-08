// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-fft/fft"
)

// goWindow and npWindow name each window in go-fft and in numpy/scipy.
var (
	goWindow = []string{"", "fft.Hann", "fft.Hamming", "fft.Blackman", "fft.BlackmanHarris", "fft.Bartlett"}
	npWindow = []string{"", "np.hanning", "np.hamming", "np.blackman", "scipy.signal.windows.blackmanharris", "np.bartlett"}
)

// num writes a float the way a person would type it in source.
func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// floatLit writes v as a Go floating-point literal, so 1/fs divides as a float
// even when fs is a whole number.
func floatLit(v float64) string {
	s := num(v)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// GoCode is the Go program fragment that computes what c computes on an
// n-sample input x — the exact go-fft calls, so a visitor can copy it.
func GoCode(c Config, n int) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	single := c.Precision == Float32
	w(`import "github.com/go-fft/fft"`)
	w("")
	switch {
	case c.Transform == TransformIRFFT:
		w("// X: the %d bins of a half spectrum, []complex%d", n/2+1, map[bool]int{false: 128, true: 64}[single])
	case single:
		w("// x: the %d input samples, []float32, taken at %s Hz", n, num(c.Fs))
	default:
		w("// x: the %d input samples, []float64, taken at %s Hz", n, num(c.Fs))
	}
	if c.Window != WindowNone && c.Transform != TransformIRFFT {
		w("win := %s(len(x))", goWindow[c.Window])
		w("for i := range x {")
		if single {
			w("\tx[i] *= float32(win[i])")
		} else {
			w("\tx[i] *= win[i]")
		}
		w("}")
	}
	suffix := ""
	if single {
		suffix = "32"
	}
	opts := ""
	if c.Norm != fft.NormBackward {
		opts = fmt.Sprintf("fft.Options{Norm: fft.%s}", normIdent(c.Norm))
	}
	call := func(name, arg string) string {
		if opts == "" {
			return fmt.Sprintf("fft.%s%s(%s)", name, suffix, arg)
		}
		return fmt.Sprintf("fft.%s%sWith(%s, %s)", name, suffix, arg, opts)
	}
	switch c.Transform {
	case TransformFFT, TransformIFFT:
		cplx := "complex128"
		if single {
			cplx = "complex64"
		}
		w("xc := make([]%s, len(x))", cplx)
		w("for i, v := range x {")
		w("\txc[i] = complex(v, 0)")
		w("}")
		w("y := %s", call(TransformNames[c.Transform], "xc"))
	case TransformRFFT:
		w("y := %s", call("RFFT", "x"))
	case TransformIRFFT:
		if opts == "" {
			w("y := fft.IRFFT%s(X, %d)", suffix, n)
		} else {
			w("y := fft.IRFFT%sWith(X, fft.Options{N: %d, Norm: fft.%s})", suffix, n, normIdent(c.Norm))
		}
	default:
		w("y := fft.%s%s(x, %d, fft.%s)", TransformNames[c.Transform], suffix, c.Type, normIdent(c.Norm))
	}
	switch c.Transform {
	case TransformFFT:
		w("freq := fft.FFTFreq(len(x), 1/%s)", floatLit(c.Fs))
		if c.Shift {
			w("y, freq = fft.FFTShift(y), fft.FFTShift(freq)")
		}
	case TransformRFFT:
		w("freq := fft.RFFTFreq(len(x), 1/%s)", floatLit(c.Fs))
	}
	return strings.TrimRight(b.String(), "\n")
}

// normIdent is the Go identifier of a Norm.
func normIdent(m fft.Norm) string {
	return [...]string{"NormBackward", "NormOrtho", "NormForward"}[m]
}

// NumpyCode is the numpy / scipy.fft call that computes the same thing.
func NumpyCode(c Config, n int) string {
	single := c.Precision == Float32
	x := "x"
	if c.Window != WindowNone && c.Transform != TransformIRFFT {
		x = fmt.Sprintf("x * %s(%d)", npWindow[c.Window], n)
	}
	if single {
		dt := "np.float32"
		if c.Transform == TransformFFT || c.Transform == TransformIFFT {
			dt = "np.complex64"
		}
		if c.Transform == TransformIRFFT {
			x = "X.astype(np.complex64)"
		} else {
			x = fmt.Sprintf("(%s).astype(%s)", x, dt)
		}
	} else if c.Transform == TransformIRFFT {
		x = "X"
	}
	norm := ""
	if c.Norm != fft.NormBackward {
		norm = fmt.Sprintf(", norm=%q", NormNames[c.Norm])
	}
	var line string
	switch c.Transform {
	case TransformFFT, TransformIFFT, TransformRFFT:
		line = fmt.Sprintf("y = np.fft.%s(%s%s)", strings.ToLower(TransformNames[c.Transform]), x, norm)
	case TransformIRFFT:
		line = fmt.Sprintf("y = np.fft.irfft(%s, n=%d%s)", x, n, norm)
	default:
		line = fmt.Sprintf("y = scipy.fft.%s(%s, type=%d%s)", strings.ToLower(TransformNames[c.Transform]), x, c.Type, norm)
	}
	switch c.Transform {
	case TransformFFT:
		line += fmt.Sprintf("\nfreq = np.fft.fftfreq(%d, d=1/%s)", n, num(c.Fs))
		if c.Shift {
			line += "\ny, freq = np.fft.fftshift(y), np.fft.fftshift(freq)"
		}
	case TransformRFFT:
		line += fmt.Sprintf("\nfreq = np.fft.rfftfreq(%d, d=1/%s)", n, num(c.Fs))
	}
	return line
}
