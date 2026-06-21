package tnnlpcore

import "time"

type StatusConfig struct {
	ID               string
	Value            string
	Label            string
	Color            string
	Icon             string
	IsCompleted      bool
	Order            int
	AutoArchive      bool
	AutoArchiveDelay int
}

type PriorityConfig struct {
	ID     string
	Value  string
	Label  string
	Color  string
	Weight int
}

type UserMappedField struct {
	ID                string
	DisplayName       string
	Key               string
	Type              string
	AutosuggestFilter any
	DefaultValue      any
}

type PropertyTriggerConfig struct {
	PropertyID string
	Trigger    string
	Enabled    bool
}

type NLPTriggersConfig struct {
	Triggers []PropertyTriggerConfig
}

var DefaultNLPTriggers = NLPTriggersConfig{
	Triggers: []PropertyTriggerConfig{
		{PropertyID: "tags", Trigger: "#", Enabled: true},
		{PropertyID: "contexts", Trigger: "@", Enabled: true},
		{PropertyID: "projects", Trigger: "+", Enabled: true},
		{PropertyID: "status", Trigger: "*", Enabled: true},
		{PropertyID: "priority", Trigger: "!", Enabled: false},
	},
}

type NumericDateOrder string

const (
	DateOrderDayFirst   NumericDateOrder = "day-first"
	DateOrderMonthFirst NumericDateOrder = "month-first"
)

type ParserOptions struct {
	DateLocale string
	DateOrder  NumericDateOrder
	Now        func() time.Time
}

type ParsedTaskData struct {
	Title         string
	Details       string
	DueDate       string
	ScheduledDate string
	DueTime       string
	ScheduledTime string
	Priority      string
	Status        string
	Tags          []string
	Contexts      []string
	Projects      []string
	Recurrence    string
	Estimate      int
	IsCompleted   *bool
	UserFields    map[string]any
}

type PreviewItem struct {
	Icon string
	Text string
}
