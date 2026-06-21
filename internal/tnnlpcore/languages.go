package tnnlpcore

type languageConfig struct {
	Code             string
	Name             string
	DateTriggers     dateTriggers
	Recurrence       recurrenceConfig
	TimeEstimate     timeEstimateConfig
	FallbackStatus   fallbackStatusConfig
	FallbackPriority fallbackPriorityConfig
}

type dateTriggers struct {
	Due       []string
	Scheduled []string
}

type recurrenceConfig struct {
	Frequencies    frequencyConfig
	Every          []string
	Other          []string
	Weekdays       weekdayConfig
	PluralWeekdays weekdayConfig
	Ordinals       ordinalConfig
	Periods        periodConfig
}

type frequencyConfig struct {
	Daily   []string
	Weekly  []string
	Monthly []string
	Yearly  []string
}

type weekdayConfig struct {
	Monday    []string
	Tuesday   []string
	Wednesday []string
	Thursday  []string
	Friday    []string
	Saturday  []string
	Sunday    []string
}

type ordinalConfig struct {
	First  []string
	Second []string
	Third  []string
	Fourth []string
	Last   []string
}

type periodConfig struct {
	Day   []string
	Week  []string
	Month []string
	Year  []string
}

type timeEstimateConfig struct {
	Hours   []string
	Minutes []string
}

type fallbackStatusConfig struct {
	Open       []string
	InProgress []string
	Done       []string
	Cancelled  []string
	Waiting    []string
}

type fallbackPriorityConfig struct {
	Urgent []string
	High   []string
	Normal []string
	Low    []string
}

type LanguageOption struct {
	Value string
	Label string
}

var englishConfig = languageConfig{
	Code: "en",
	Name: "English",
	DateTriggers: dateTriggers{
		Due:       []string{"due", "deadline", "must be done by", "by"},
		Scheduled: []string{"scheduled for", "start on", "begin on", "work on", "on", "scheduled", "start"},
	},
	Recurrence: recurrenceConfig{
		Frequencies: frequencyConfig{
			Daily:   []string{"daily", "every day"},
			Weekly:  []string{"weekly", "every week"},
			Monthly: []string{"monthly", "every month"},
			Yearly:  []string{"yearly", "annually", "every year"},
		},
		Every: []string{"every"},
		Other: []string{"other"},
		Weekdays: weekdayConfig{
			Monday: []string{"monday"}, Tuesday: []string{"tuesday"}, Wednesday: []string{"wednesday"},
			Thursday: []string{"thursday"}, Friday: []string{"friday"}, Saturday: []string{"saturday"}, Sunday: []string{"sunday"},
		},
		PluralWeekdays: weekdayConfig{
			Monday: []string{"mondays"}, Tuesday: []string{"tuesdays"}, Wednesday: []string{"wednesdays"},
			Thursday: []string{"thursdays"}, Friday: []string{"fridays"}, Saturday: []string{"saturdays"}, Sunday: []string{"sundays"},
		},
		Ordinals: ordinalConfig{
			First: []string{"first"}, Second: []string{"second"}, Third: []string{"third"}, Fourth: []string{"fourth"}, Last: []string{"last"},
		},
		Periods: periodConfig{
			Day: []string{"day", "days"}, Week: []string{"week", "weeks"}, Month: []string{"month", "months"}, Year: []string{"year", "years"},
		},
	},
	TimeEstimate: timeEstimateConfig{
		Hours:   []string{"h", "hr", "hrs", "hour", "hours"},
		Minutes: []string{"m", "min", "mins", "minute", "minutes"},
	},
	FallbackStatus: fallbackStatusConfig{
		Open: []string{"todo", "to do", "open"}, InProgress: []string{"in progress", "in-progress", "doing"},
		Done: []string{"done", "completed", "finished"}, Cancelled: []string{"cancelled", "canceled"}, Waiting: []string{"waiting", "blocked", "on hold"},
	},
	FallbackPriority: fallbackPriorityConfig{
		Urgent: []string{"urgent", "critical", "highest"}, High: []string{"high", "important"}, Normal: []string{"medium", "normal"}, Low: []string{"low", "minor"},
	},
}

var languageRegistry = map[string]languageConfig{
	"en": englishConfig,
	"de": mergeLanguage("de", "German", dateTriggers{
		Due:       []string{"fällig", "faellig", "frist", "bis"},
		Scheduled: []string{"geplant für", "geplant", "starten", "start"},
	}, fallbackPriorityConfig{}),
	"ja": mergeLanguage("ja", "Japanese", dateTriggers{}, fallbackPriorityConfig{High: []string{"高"}}),
	"zh": mergeLanguage("zh", "Chinese", dateTriggers{}, fallbackPriorityConfig{High: []string{"高"}}),
	"es": mergeLanguage("es", "Spanish", dateTriggers{Due: []string{"vence", "para"}, Scheduled: []string{"programado", "el"}}, fallbackPriorityConfig{}),
	"fr": mergeLanguage("fr", "French", dateTriggers{Due: []string{"échéance", "du", "pour"}, Scheduled: []string{"programmé", "le"}}, fallbackPriorityConfig{}),
	"it": mergeLanguage("it", "Italian", dateTriggers{Due: []string{"scadenza", "entro"}, Scheduled: []string{"programmato", "il"}}, fallbackPriorityConfig{}),
	"nl": mergeLanguage("nl", "Dutch", dateTriggers{Due: []string{"deadline", "voor"}, Scheduled: []string{"gepland", "op"}}, fallbackPriorityConfig{}),
	"pt": mergeLanguage("pt", "Portuguese", dateTriggers{Due: []string{"vence", "até"}, Scheduled: []string{"agendado", "em"}}, fallbackPriorityConfig{}),
	"ru": mergeLanguage("ru", "Russian", dateTriggers{Due: []string{"срок", "до"}, Scheduled: []string{"запланировано"}}, fallbackPriorityConfig{}),
	"sv": mergeLanguage("sv", "Swedish", dateTriggers{Due: []string{"förfaller", "senast"}, Scheduled: []string{"schemalagd", "på"}}, fallbackPriorityConfig{}),
	"uk": mergeLanguage("uk", "Ukrainian", dateTriggers{Due: []string{"термін", "до"}, Scheduled: []string{"заплановано"}}, fallbackPriorityConfig{}),
}

func mergeLanguage(code, name string, dates dateTriggers, priority fallbackPriorityConfig) languageConfig {
	cfg := englishConfig
	cfg.Code = code
	cfg.Name = name
	if len(dates.Due) > 0 {
		cfg.DateTriggers.Due = dates.Due
	}
	if len(dates.Scheduled) > 0 {
		cfg.DateTriggers.Scheduled = dates.Scheduled
	}
	if len(priority.Urgent) > 0 {
		cfg.FallbackPriority.Urgent = priority.Urgent
	}
	if len(priority.High) > 0 {
		cfg.FallbackPriority.High = priority.High
	}
	if len(priority.Normal) > 0 {
		cfg.FallbackPriority.Normal = priority.Normal
	}
	if len(priority.Low) > 0 {
		cfg.FallbackPriority.Low = priority.Low
	}
	return cfg
}

func GetAvailableLanguages() []LanguageOption {
	codes := []string{"en", "es", "fr", "de", "ru", "zh", "ja", "it", "nl", "pt", "sv", "uk"}
	out := make([]LanguageOption, 0, len(codes))
	for _, code := range codes {
		cfg := languageRegistry[code]
		out = append(out, LanguageOption{Value: cfg.Code, Label: cfg.Name})
	}
	return out
}

func getLanguageConfig(code string) languageConfig {
	if cfg, ok := languageRegistry[code]; ok {
		return cfg
	}
	return englishConfig
}

func DetectSystemLanguage() string {
	return "en"
}
