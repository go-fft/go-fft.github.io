// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-fft/fft"
)

// allConfigs is every combination the pickers can make that changes the code.
func allConfigs() []Config {
	var out []Config
	for tr := TransformFFT; tr <= TransformIDST; tr++ {
		for _, prec := range []Precision{Float64, Float32} {
			for norm := fft.NormBackward; norm <= fft.NormForward; norm++ {
				for _, w := range []Window{WindowNone, WindowBlackmanHarris} {
					for _, shift := range []bool{false, true} {
						out = append(out, Config{Transform: tr, Type: 3, Norm: norm, Window: w, Precision: prec, Shift: shift, Fs: 44100.5})
					}
				}
			}
		}
	}
	return out
}

// TestGoCodeCompiles builds every snippet the playground can show, against
// the go-fft this module requires: the code a visitor copies is real Go.
func TestGoCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	var b strings.Builder
	b.WriteString("package main\n\nimport \"github.com/go-fft/fft\"\n\nvar _ = fft.Hann\n\n")
	for i, c := range allConfigs() {
		code := GoCode(c, 16)
		body, ok := strings.CutPrefix(code, "import \"github.com/go-fft/fft\"\n")
		if !ok {
			t.Fatalf("code does not start with the import:\n%s", code)
		}
		decl := "x := make([]float64, 16)"
		switch {
		case c.Transform == TransformIRFFT && c.Precision == Float32:
			decl = "X := make([]complex64, 9)"
		case c.Transform == TransformIRFFT:
			decl = "X := make([]complex128, 9)"
		case c.Precision == Float32:
			decl = "x := make([]float32, 16)"
		}
		use := "_ = y"
		if strings.Contains(body, "freq :=") {
			use = "_, _ = y, freq"
		}
		fmt.Fprintf(&b, "func f%d() {\n%s\n%s\n%s\n}\n\n", i, decl, body, use)
	}
	b.WriteString("func main() {}\n")
	dir, err := os.MkdirTemp(".", "codegen-check-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "vet", "./"+dir).CombinedOutput()
	if err != nil {
		t.Fatalf("the generated Go does not build: %v\n%s", err, out)
	}
}

func TestGoCodeNamesTheCalls(t *testing.T) {
	cases := []struct {
		c    Config
		want []string
	}{
		{Config{Transform: TransformRFFT, Window: WindowHann, Fs: 1000}, []string{"fft.Hann(len(x))", "y := fft.RFFT(x)", "fft.RFFTFreq(len(x), 1/1000.0)"}},
		{Config{Transform: TransformFFT, Norm: fft.NormOrtho, Precision: Float32, Shift: true, Fs: 8}, []string{"[]complex64", "fft.FFT32With(xc, fft.Options{Norm: fft.NormOrtho})", "fft.FFTShift(y)", "[]float32"}},
		{Config{Transform: TransformIRFFT, Fs: 8}, []string{"y := fft.IRFFT(X, 16)", "half spectrum"}},
		{Config{Transform: TransformIRFFT, Norm: fft.NormForward, Precision: Float32, Fs: 8}, []string{"fft.IRFFT32With(X, fft.Options{N: 16, Norm: fft.NormForward})", "[]complex64"}},
		{Config{Transform: TransformIDST, Type: 4, Window: WindowBartlett, Precision: Float32, Fs: 8}, []string{"y := fft.IDST32(x, 4, fft.NormBackward)", "float32(win[i])"}},
	}
	for _, tc := range cases {
		code := GoCode(tc.c, 16)
		for _, w := range tc.want {
			if !strings.Contains(code, w) {
				t.Errorf("GoCode(%+v) lacks %q:\n%s", tc.c, w, code)
			}
		}
	}
	if floatLit(1000) != "1000.0" || floatLit(0.5) != "0.5" || floatLit(1e21) != "1e+21" {
		t.Errorf("floatLit: %s %s %s", floatLit(1000), floatLit(0.5), floatLit(1e21))
	}
}

func TestNumpyCode(t *testing.T) {
	cases := []struct {
		c    Config
		want []string
	}{
		{Config{Transform: TransformRFFT, Window: WindowHann, Fs: 1000}, []string{"y = np.fft.rfft(x * np.hanning(16))", "np.fft.rfftfreq(16, d=1/1000)"}},
		{Config{Transform: TransformFFT, Norm: fft.NormOrtho, Precision: Float32, Shift: true, Fs: 8}, []string{`np.fft.fft((x).astype(np.complex64), norm="ortho")`, "np.fft.fftshift(y)", "fftfreq(16, d=1/8)"}},
		{Config{Transform: TransformIRFFT, Fs: 8}, []string{"np.fft.irfft(X, n=16)"}},
		{Config{Transform: TransformIRFFT, Precision: Float32, Fs: 8}, []string{"np.fft.irfft(X.astype(np.complex64), n=16)"}},
		{Config{Transform: TransformDCT, Type: 2, Norm: fft.NormForward, Window: WindowBlackmanHarris, Precision: Float32, Fs: 8}, []string{`scipy.fft.dct((x * scipy.signal.windows.blackmanharris(16)).astype(np.float32), type=2, norm="forward")`}},
	}
	for _, tc := range cases {
		code := NumpyCode(tc.c, 16)
		for _, w := range tc.want {
			if !strings.Contains(code, w) {
				t.Errorf("NumpyCode(%+v) lacks %q:\n%s", tc.c, w, code)
			}
		}
	}
}
