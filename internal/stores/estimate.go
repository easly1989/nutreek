package stores

import "math"

// Estimate prices what a shopping row needs, in whole packs of the linked product:
// need is in needUnit (g or ml), the package is size in sizeUnit (g, ml or pz; a piece
// weighs unitWeightG of the ingredient's unit). ok is false when the units cannot be
// compared or the package has no size.
func Estimate(need float64, needUnit string, unitWeightG, size float64, sizeUnit string, priceCents int) (total, packs int, ok bool) {
	if need <= 0 || size <= 0 {
		return 0, 0, false
	}
	switch {
	case needUnit == sizeUnit:
	case sizeUnit == UnitPiece && unitWeightG > 0:
		need /= unitWeightG
	default:
		return 0, 0, false
	}
	packs = max(1, int(math.Ceil(need/size-1e-9)))
	return packs * priceCents, packs, true
}
