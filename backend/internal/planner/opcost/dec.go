package opcost

import (
	"math/big"
	"strconv"
)

// dec is the exact value of the decimal a user typed: the shortest decimal that
// round-trips the float64 (55 and 2.2 are 55 and 11/5, not their binary
// neighbours). Only finite inputs are passed in.
func dec(x float64) *big.Rat {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(x, 'g', -1, 64))
	if !ok {
		return new(big.Rat)
	}
	return r
}

// cents is the exact value rounded to whole cents, half away from zero.
func cents(r *big.Rat) *big.Int {
	x := new(big.Rat).Mul(r, big.NewRat(100, 1))
	neg := x.Sign() < 0
	if neg {
		x.Neg(x)
	}
	x.Add(x, big.NewRat(1, 2))
	q := new(big.Int).Quo(x.Num(), x.Denom())
	if neg {
		q.Neg(q)
	}
	return q
}
