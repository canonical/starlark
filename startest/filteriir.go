package startest

// filterIIR represents a filter which remove high-pitch noise from a sequence
// of measurements. An IIR (Infinite Impulse Response) filter usually removes
// significantly more noise than a simpler one, such as a moving average.
// Specifically, this struct implements a 2nd order IIR filter in Direct-Form II
// (transposed).
//
// More info at https://en.wikipedia.org/wiki/Infinite_impulse_response
type filterIIR struct {
	B [3]float64
	A [3]float64
	w [2]float64
}

// Filter applies the filter to the given samples, updating the internal state.
// It returns the filtered sample.
func (f *filterIIR) Filter(sample float64) float64 {
	f.normalise()

	y := f.w[0] + f.B[0]*sample
	f.w[0] = f.w[1] - f.A[1]*y + f.B[1]*sample
	f.w[1] = f.B[2]*sample - f.A[2]*y
	return y
}

// BatchFilter filters and returns a complete signal. This function
// implements a zero-phase filter [1], avoiding unwanted effects at
// the ends of the sequence.
//
//	 [1]: Gustafsson, Fredrik.
//		"Determining the initial states in forward-backward filtering."
//		IEEE Transactions on signal processing 44.4 (1996): 988-992.
//		https://www.diva-portal.org/smash/get/diva2:315708/FULLTEXT02
func (f *filterIIR) BatchFilter(signal []float64) []float64 {
	// The zero-phase technique we use requires more than 3*order samples.
	if len(signal) <= 6 {
		return signal // too small
	}

	f.normalise()
	kdc := (f.B[0] + f.B[1] + f.B[2]) / (1 + f.A[1] + f.A[2])
	si := [2]float64{}
	si[1] = f.B[2] - kdc*f.A[2]
	si[0] = si[1] + f.B[1] - kdc*f.A[1]

	v := make([]float64, 0, len(signal)+12)

	// Forward pass
	f.w = [2]float64{
		si[0] * (signal[0]*2 - signal[6]),
		si[1] * (signal[0]*2 - signal[6]),
	}
	for i := 6; i >= 1; i-- {
		v = append(v, f.Filter(signal[0]*2-signal[i]))
	}
	for i := range signal {
		v = append(v, f.Filter(signal[i]))
	}
	for i := 1; i <= 6; i++ {
		v = append(v, f.Filter(signal[len(signal)-1]*2-signal[len(signal)-1-i]))
	}

	// Backward pass
	f.w = [2]float64{
		si[0] * v[len(v)-1],
		si[1] * v[len(v)-1],
	}
	for i := len(v) - 1; i >= 0; i-- {
		v[i] = f.Filter(v[i])
	}

	return v[6 : len(signal)+6]
}

func (f *filterIIR) normalise() {
	if f.A[0] != 1 {
		for i := range f.B {
			f.B[i] /= f.A[0]
		}
		for i := range f.A[1:] {
			f.A[i] /= f.A[0]
		}
		f.A[0] = 1
	}
}
