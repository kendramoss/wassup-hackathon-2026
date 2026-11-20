package opcost

// Feasibility and check statuses.
const (
	Feasible      = "feasible"
	Infeasible    = "infeasible"
	StatusUnknown = "unknown"
)

// Rejection decisions (domain.RejectionKind values in the wiring step).
const (
	DecisionInfeasible = "operationally_infeasible"
	DecisionCostlier   = "net_costlier"
)

// Move is one operation group moved to a plant. FromPlant is the group's
// current plant; empty means the baseline is ambiguous (a multi-plant group):
// the cost of the move is then unknown, never taken from an arbitrary plant.
type Move struct{ Group, FromPlant, ToPlant string }

// Input is the plain data one move is evaluated on.
type Input struct {
	AnnualImportValueUSD float64
	FromRate, ToRate     *float64 // nil: the user gave no rate for that origin
	GroupCosts           map[string]map[string]float64
	Plants               map[string]Ref // the product's plants by ID, canonical
	Moves                []Move
	Ops                  *Operations
}

// Unknown is one input that could not be read, in a financial or feasibility
// category.
type Unknown struct {
	Category string `json:"category"` // financial | feasibility
	Factor   string `json:"factor"`
	Group    string `json:"group,omitempty"`
	Reason   string `json:"reason"`
}

// Item names one entry (group, plant, factor).
type Item struct {
	Group  string `json:"group"`
	Plant  string `json:"plant"`
	Factor string `json:"factor"`
}

// Check is one evaluated feasibility check.
type Check struct {
	Kind   string   `json:"kind"` // lead_time | moq
	Group  string   `json:"group,omitempty"`
	Status string   `json:"status"`
	Value  *float64 `json:"value,omitempty"`
	Limit  *float64 `json:"limit,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

// Components are the yearly figures of the MODELLED terms. A number, including
// 0, is a computed value from entered inputs (an entered 0 is a known zero). nil
// is never a zero: it is either unknown (the term is listed in Result.Unknown,
// category financial) or not modelled (listed in Result.NotModelled: never
// entered, or a labour/logistics breakdown already inside the group cost). The
// aggregates sum only modelled terms, so an aggregate is nil when every term of
// it is not modelled, and the net treats a not-modelled aggregate as neutral
// (it adds nothing); that is a scope statement, not an expense of 0.
type Components struct {
	DutyEffectPerYear       *float64 `json:"duty_effect_per_year"`
	GroupCostDelta          *float64 `json:"group_cost_delta_per_year"`
	LabourDelta             *float64 `json:"labour_delta_per_year"`
	LogisticsDelta          *float64 `json:"logistics_delta_per_year"`
	OperatingDelta          *float64 `json:"operating_delta_per_year"`
	OneTimeTotal            *float64 `json:"one_time_total_usd"`
	AmortizedOneTimePerYear *float64 `json:"amortized_one_time_per_year"`
}

// Result is the outcome of Evaluate for one move.
type Result struct {
	Components         Components `json:"components"`
	NetModelledPerYear *float64   `json:"net_modelled_per_year"`
	Priced             bool       `json:"priced"`
	HorizonYears       *float64   `json:"horizon_years,omitempty"`
	Unknown            []Unknown  `json:"unknown"`
	NotModelled        []string   `json:"not_modelled"`
	SunkIgnored        []Item     `json:"sunk_ignored"`
	StaleRefs          []Item     `json:"stale_refs"`
	Feasibility        string     `json:"feasibility,omitempty"`
	Assessed           bool       `json:"feasibility_assessed"`
	Checks             []Check    `json:"checks"`
	Assumptions        []string   `json:"assumptions"`
	Decision           string     `json:"decision,omitempty"`
	// Baseline and Proposed are the absolute yearly figures of the MODELLED
	// terms of the moved groups, from the same arithmetic as the net. Unmoved
	// groups are identical on both sides and cancel; terms listed in
	// NotModelled are not in either side.
	Baseline *Side `json:"baseline"`
	Proposed *Side `json:"proposed"`

	// The legacy plan fields: known parts, an unknown side read as 0, as
	// economics() reads a blank. Private mechanics for the planner to copy into
	// its own fields; never serialised (the authoritative figures are the
	// nullable ones above).
	DutyEffectYear, AddedCostYear, NetYear float64 `json:"-"`
}

// Side is one side of the comparison. nil is not a usable number: look in the
// result's Unknown (an entered but incomplete input) or NotModelled (never
// entered, or already inside the group cost).
type Side struct {
	DutyUSD             *float64 `json:"duty_usd"`
	OperatingUSD        *float64 `json:"operating_usd"`
	AmortizedOneTimeUSD *float64 `json:"amortized_one_time_usd,omitempty"` // proposed only
	TotalUSD            *float64 `json:"total_usd"`
}

// Assumptions stated in every result.
const (
	AssumptionParallel     = "lead times of moved groups run in parallel: the move's lead time is the longest of them (no scheduling proof)"
	AssumptionStraightLine = "one-time costs are amortized straight-line over horizon_years: no discounting, residual value or tax"
)
