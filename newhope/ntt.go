package newhope

// ntt performes the ntt transformation on the CRT coefficients a Polynomial, based on the target context.
func (context *Context) NTT(p1, p2 *Poly) {
	ntt(p1.Coeffs, p2.Coeffs, context.n, context.nttPsi, context.q, context.mredparam, context.bredparam)
}

// Invntt performes the inverse ntt transformation on the CRT coefficients of a polynomial, based on the target context.
func (context *Context) InvNTT(p1, p2 *Poly) {
	invntt(p1.Coeffs, p2.Coeffs, context.n, context.nttPsiInv, context.nttNInv, context.q, context.mredparam)
}

// Buttefly computes X, Y = U + V*Psi, U - V*Psi mod Q.
func butterfly(U, V, Psi, Q, Qinv uint32) (X, Y uint32) {
	if U > 2*Q {
		U -= 2 * Q
	}
	V = mredconstant(V, Psi, Q, Qinv)
	X = U + V
	Y = U + 2*Q - V
	return
}

// invbutterfly computes X, Y = U + V, (U - V) * Psi mod Q.
func invbutterfly(U, V, Psi, Q, Qinv uint32) (X, Y uint32) {
	X = U + V
	if X > 2*Q {
		X -= 2 * Q
	}
	Y = mredconstant(U+2*Q-V, Psi, Q, Qinv) // At the moment it is not possible to use mredconstant if Q > 61 bits
	return
}

// ntt computes the ntt transformation on the input coefficients given the provided params.
func ntt(coeffs_in, coeffs_out []uint32, N uint32, nttPsi []uint32, Q, mredParams uint32, bredParams uint64) {
	var j1, j2, t uint32
	var F uint32

	// Copies the result of the first round of butterflies on p2 with approximate reduction
	t = N >> 1
	j2 = t - 1
	F = nttPsi[1]
	for j := uint32(0); j <= j2; j++ {
		coeffs_out[j], coeffs_out[j+t] = butterfly(coeffs_in[j], coeffs_in[j+t], F, Q, mredParams)
	}

	// Continues the rest of the second to the n-1 butterflies on p2 with approximate reduction
	for m := uint32(2); m < N; m <<= 1 {
		t >>= 1
		for i := uint32(0); i < m; i++ {

			j1 = (i * t) << 1

			j2 = j1 + t - 1

			F = nttPsi[m+i]

			for j := j1; j <= j2; j++ {
				coeffs_out[j], coeffs_out[j+t] = butterfly(coeffs_out[j], coeffs_out[j+t], F, Q, mredParams)
			}
		}
	}

	// Finishes with an exact reduction
	for i := uint32(0); i < N; i++ {
		coeffs_out[i] = bredadd(coeffs_out[i], Q, bredParams)
	}
}

// Invntt computes the Invntt transformation on the input coefficients given the provided params.
func invntt(coeffs_in, coeffs_out []uint32, N uint32, nttPsiInv []uint32, nttNInv, Q, mredParams uint32) {

	var j1, j2, h, t uint32
	var F uint32

	// Copies the result of the first round of butterflies on p2 with approximate reduction
	t = 1
	j1 = 0
	h = N >> 1

	for i := uint32(0); i < h; i++ {

		j2 = j1

		F = nttPsiInv[h+i]

		for j := j1; j <= j2; j++ {
			coeffs_out[j], coeffs_out[j+t] = invbutterfly(coeffs_in[j], coeffs_in[j+t], F, Q, mredParams)
		}

		j1 = j1 + (t << 1)
	}

	// Continues the rest of the second to the n-1 butterflies on p2 with approximate reduction
	t <<= 1
	for m := N >> 1; m > 1; m >>= 1 {

		j1 = 0
		h = m >> 1

		for i := uint32(0); i < h; i++ {

			j2 = j1 + t - 1

			F = nttPsiInv[h+i]

			for j := j1; j <= j2; j++ {
				coeffs_out[j], coeffs_out[j+t] = invbutterfly(coeffs_out[j], coeffs_out[j+t], F, Q, mredParams)
			}

			j1 = j1 + (t << 1)
		}

		t <<= 1
	}

	// Finishes with an exact reduction given
	for j := uint32(0); j < N; j++ {
		coeffs_out[j] = mred(coeffs_out[j], nttNInv, Q, mredParams)
	}
}
