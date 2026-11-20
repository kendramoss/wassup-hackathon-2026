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

// Components are the yearly figures; nil is unknown, 0 is a known zero.
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
	// The legacy plan fields: known parts, an unknown side read as 0 (as
	// economics() reads a blank), only meaningful with Priced.
	DutyEffectYear, AddedCostYear, NetYear float64
}

// Assumptions stated in every result.
const (
	AssumptionParallel     = "lead times of moved groups run in parallel: the move's lead time is the longest of them (no scheduling proof)"
	AssumptionStraightLine = "one-time costs are amortized straight-line over horizon_years: no discounting, residual value or tax"
)
