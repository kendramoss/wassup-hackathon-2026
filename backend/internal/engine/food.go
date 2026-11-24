package engine

import (
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// FoodVersion names FoodRules; a prediction for a food case made with them
// records PromptVersion+"+"+FoodVersion (D856, #815).
const FoodVersion = "food-v1"

// FoodRules is CBP practice on processing a food, added to the system prompt
// only for a case FoodCase names. It was written from development and
// training rulings (D856), so a development result is not independent.
const FoodRules = `CBP practice on food products:
1. The same food, prepared, keeps its origin. Cleaning, gutting, filleting, cutting into pieces, portioning, peeling, deveining, thawing, freezing, glazing, salting, brining or marinating, adding phosphates, blending or refining an oil, and packing generally do not substantially transform a food. Origin stays where the raw food was grown, caught or first made into that food. Frozen squid, scallops, octopus and shrimp cleaned, cut or peeled in a second country stayed Peru, Japan, the Philippines and Ecuador.
2. A new food with a new name and use does transform. Grinding, pureeing or extracting a fruit, vegetable or seed into a puree, paste, powder or oil (tahini from sesame, a puree from fruit), spray drying, smoking, frying or baking a battered or breaded item, and making confectionery or a sauce from separate ingredients generally do.
3. Cooking or canning alone, with no new form, generally does not: grilling or steaming meat or fish pieces, or cooking a shellfish in its shell and canning it, left origin with the raw food (chargrilled chicken: United States). A ready-to-eat product made by dressing and frying or smoking the raw food (fully cooked battered chicken, smoked salmon) is a new food.
4. Compare each step of the facts with these three. When rulings for the same kind of food point both ways, follow the one whose processing is closest to these facts.`

// foodStrong are words that on their own mark a food in the facts.
var foodStrong = regexp.MustCompile(`(?i)\b(fish|salmon|tuna|cod|halibut|shrimps?|prawns?|crab(?:meat)?|clams|squid|octopus|scallops?|oysters?|mussels?|seafood|shellfish|chicken|beef|pork|poultry|bacon|sausages?|puree|pur[eé]es?|fruits?|vegetables?|juices?|sesame|tahini|kimchi|seaweed|noodles?|jerky|tea|coffee|cocoa|chocolate|candy|candies|foodstuffs?|edible|honey|anchov\w+|sardines?)\b`)

// foodWeak are words that also turn up on appliances and packaging; two
// different ones mark a food.
var foodWeak = regexp.MustCompile(`(?i)\b(almonds?|peanuts?|walnuts?|cashews?|pistachios?|rice|flour|sauces?|spices?|seasoning|snacks?|potato(?:es)?|beans?|tomato(?:es)?|cheese|milk|dairy|beverages?|pasta|bread|biscuits?|cookies?|cereals?|dough|jam|syrups?|pickles?|olives?|pineapple|banana|mango|garlic|onions?|mushrooms?|vinegar|yogurt|tofu|soybeans?|wine|beer|liquor|vodka|whisk(?:e)?y|sugar)\b`)

// FoodCase reports whether the case's facts describe a food: one strong word
// or two different weak ones. Deterministic and facts only.
func FoodCase(c domain.Case) bool {
	if foodStrong.MatchString(c.Facts) {
		return true
	}
	seen := map[string]bool{}
	for _, m := range foodWeak.FindAllString(c.Facts, -1) {
		seen[strings.ToLower(m)] = true
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

// FoodCaseRules is Options.CaseRules for food-v1: FoodRules for a food case,
// "" for any other.
func FoodCaseRules(c domain.Case) string {
	if FoodCase(c) {
		return FoodRules
	}
	return ""
}
