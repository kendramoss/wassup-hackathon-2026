// Package domain holds Unde's core types: products described as
// components and operations performed at plants, CBP rulings, origin
// predictions, plan moves and precedent verdicts.
//
// It is pure: no I/O and no SDK imports. Every type that crosses an API or
// storage boundary has a Validate method; callers validate at the edge and the
// rest of the code may assume valid values.
package domain
