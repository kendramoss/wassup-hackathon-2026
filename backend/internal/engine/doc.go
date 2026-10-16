// Package engine predicts the country of origin CBP would likely assign under
// the substantial-transformation test: Gemini reads the case facts plus the k
// most similar CBP rulings dated strictly before the case, and returns the
// origin, key factors, the decisive step, a confidence and its reasoning.
package engine
