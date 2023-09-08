package startest

// iir2 represents a 2nd order IIR filter in Direct-Form II (transposed).
// The filter weights must be already normalized.
type iir2 struct {
	b [3]float64
	a [2]float64
	w [2]float64
}

// Filter applies the filter to the sample x, returns the filtered sample
// and updates the internal state.
func (f *iir2) Filter(x float64) float64 {
	y := f.w[0] + f.b[0]*x
	f.w[0] = f.w[1] - f.a[0]*y + f.b[1]*x
	f.w[1] = f.b[2]*x - f.a[1]*y
	return y
}

// BatchFilter applies batch-filtering to signal x
// to obtain a zero-phase filter[1] and avoid unwanted
// effects at the beginning and end of the sequence.
//
// [1]: Gustafsson, Fredrik.
//
//	"Determining the initial states in forward-backward filtering."
//	IEEE Transactions on signal processing 44.4 (1996): 988-992.
//	https://www.diva-portal.org/smash/get/diva2:315708/FULLTEXT02
func (f *iir2) BatchFilter(x []float64) []float64 {
	// The zero-phase technique requires
	//    len(x) > 3*order
	// samples to work.
	if len(x) < 7 {
		return x // too small
	}

	kdc := (f.b[0] + f.b[1] + f.b[2]) / (1 + f.a[0] + f.a[1])

	// Transposed sum
	si := [2]float64{}
	si[1] = f.b[2] - kdc*f.a[1]
	si[0] = si[1] + f.b[1] - kdc*f.a[0]

	f.w = [2]float64{
		si[0] * (x[0]*2 - x[6]),
		si[1] * (x[0]*2 - x[6]),
	}

	v := []float64{}

	for i := 6; i >= 1; i-- {
		v = append(v, f.Filter(x[0]*2-x[i]))
	}
	for _, x_i := range x {
		v = append(v, f.Filter(x_i))
	}
	for i := 1; i <= 6; i++ {
		v = append(v, f.Filter(x[len(x)-1]*2-x[len(x)-1-i]))
	}

	f.w = [2]float64{
		si[0] * v[len(v)-1],
		si[1] * v[len(v)-1],
	}
	for i := len(v) - 1; i >= 0; i-- {
		v[i] = f.Filter(v[i])
	}

	return v[6 : len(x)+6]
}
