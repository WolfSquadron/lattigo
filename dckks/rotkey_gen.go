package dckks

import (
	"github.com/ldsec/lattigo/ckks"
	"github.com/ldsec/lattigo/ring"
)

// RTGProtocol is the structure storing the parameters for the collective rotation-keys generation.
type RTGProtocol struct {
	dckksContext *dckksContext

	galElRotRow uint64
	galElRotCol map[ckks.Rotation][]uint64

	tmpSwitchKey [][2]*ring.Poly
	tmpPoly      *ring.Poly
}

// RTGShare is a struct storing the share of the RTG protocol.
type RTGShare struct {
	Type  ckks.Rotation
	K     uint64
	Value []*ring.Poly
}

// AllocateShare allocates the share the the RTG protocol.
func (rtg *RTGProtocol) AllocateShare() (rtgShare RTGShare) {
	rtgShare.Value = make([]*ring.Poly, rtg.dckksContext.beta)
	for i := uint64(0); i < rtg.dckksContext.beta; i++ {
		rtgShare.Value[i] = rtg.dckksContext.contextQP.NewPoly()
	}
	return
}

// NewRotKGProtocol creates a new rotkg object and will be used to generate collective rotation-keys from a shared secret-key among j parties.
func NewRotKGProtocol(params *ckks.Parameters) (rtg *RTGProtocol) {

	rtg = new(RTGProtocol)

	dckksContext := newDckksContext(params)

	rtg.dckksContext = dckksContext

	rtg.tmpSwitchKey = make([][2]*ring.Poly, rtg.dckksContext.beta)
	for i := range rtg.tmpSwitchKey {
		rtg.tmpSwitchKey[i][0] = dckksContext.contextQP.NewPoly()
		rtg.tmpSwitchKey[i][1] = dckksContext.contextQP.NewPoly()
	}

	rtg.tmpPoly = dckksContext.contextQP.NewPoly()

	N := dckksContext.n

	rtg.galElRotCol = make(map[ckks.Rotation][]uint64)
	for _, rotType := range []ckks.Rotation{ckks.RotationLeft, ckks.RotationRight} {

		gen := ckks.GaloisGen
		if rotType == ckks.RotationRight {
			gen = ring.ModExp(gen, (N<<1)-1, N<<1)
		}

		rtg.galElRotCol[rotType] = ring.GenGaloisParams(N, gen)

	}

	rtg.galElRotRow = (N << 1) - 1

	return rtg
}

// GenShare is the first and unique round of the rotkg protocol. Each party, using its secret share of the collective secret-key
// and a collective random polynomial, a public share of the rotation-key by computing :
//
// [a*s_i + (pi(s_i) - s_i) + e]
//
// and broadcasts it to the other j-1 parties. The protocol must be repeated for each desired rotation.
func (rtg *RTGProtocol) GenShare(rotType ckks.Rotation, k uint64, sk *ring.Poly, crp []*ring.Poly, shareOut *RTGShare) {
	shareOut.Type = rotType
	shareOut.K = k
	switch rotType {
	case ckks.RotationRight, ckks.RotationLeft:
		rtg.genShare(sk, rtg.galElRotCol[rotType][k&((rtg.dckksContext.n>>1)-1)], crp, shareOut.Value)
		return
	case ckks.Conjugate:
		rtg.genShare(sk, rtg.galElRotRow, crp, shareOut.Value)
		return
	}
}

// genswitchkey is a generic method to generate the public-share of the collective rotation-key.
func (rtg *RTGProtocol) genShare(sk *ring.Poly, galEl uint64, crp []*ring.Poly, evakey []*ring.Poly) {

	contextQP := rtg.dckksContext.contextQP

	ring.PermuteNTT(sk, galEl, rtg.tmpPoly)
	contextQP.Sub(rtg.tmpPoly, sk, rtg.tmpPoly)

	contextQP.MulScalarBigint(rtg.tmpPoly, rtg.dckksContext.contextP.ModulusBigint, rtg.tmpPoly)

	contextQP.InvMForm(rtg.tmpPoly, rtg.tmpPoly)

	var index uint64

	for i := uint64(0); i < rtg.dckksContext.beta; i++ {

		// e
		evakey[i] = rtg.dckksContext.gaussianSampler.SampleNTTNew()

		// a is the CRP

		// e + sk_in * (qiBarre*qiStar) * 2^w
		// (qiBarre*qiStar)%qi = 1, else 0
		for j := uint64(0); j < rtg.dckksContext.alpha; j++ {

			index = i*rtg.dckksContext.alpha + j

			qi := contextQP.Modulus[index]
			tmp0 := rtg.tmpPoly.Coeffs[index]
			tmp1 := evakey[i].Coeffs[index]

			for w := uint64(0); w < contextQP.N; w++ {
				tmp1[w] = ring.CRed(tmp1[w]+tmp0[w], qi)
			}

			// Handles the case where nb pj does not divides nb qi
			if index >= uint64(len(rtg.dckksContext.params.Q)-1) {
				break
			}
		}

		// sk_in * (qiBarre*qiStar) * 2^w - a*sk + e
		contextQP.MulCoeffsMontgomeryAndSub(crp[i], sk, evakey[i])
		contextQP.MForm(evakey[i], evakey[i])

	}

	rtg.tmpPoly.Zero()

	return
}

// Aggregate is the second part of the unique round of the rotkg protocol. Uppon receiving the j-1 public shares,
// each party computes  :
//
// [sum(a*a_j + (pi(a_j) - a_j) + e_j), a]
func (rtg *RTGProtocol) Aggregate(share1, share2, shareOut RTGShare) {
	contextQP := rtg.dckksContext.contextQP

	if share1.Type != share2.Type || share1.K != share2.K {
		panic("cannot aggregate shares of different types")
	}

	shareOut.Type = share1.Type
	shareOut.K = share1.K
	for i := uint64(0); i < rtg.dckksContext.beta; i++ {
		contextQP.Add(share1.Value[i], share2.Value[i], shareOut.Value[i])
	}
}

// Finalize finalizes the RTG protocol and populates the input RotationKey with the computed collective SwitchingKey.
func (rtg *RTGProtocol) Finalize(params *ckks.Parameters, share RTGShare, crp []*ring.Poly, rotKey *ckks.RotationKeys) {

	k := share.K & ((rtg.dckksContext.n >> 1) - 1)

	for i := uint64(0); i < rtg.dckksContext.beta; i++ {
		rtg.tmpSwitchKey[i][0].Copy(share.Value[i])
		rtg.dckksContext.contextQP.MForm(crp[i], rtg.tmpSwitchKey[i][1])
	}

	rotKey.SetRotKey(params, rtg.tmpSwitchKey, share.Type, k)
}
