package tnnlpcore

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func testParser(opts ...func(*parserTestOptions)) *NaturalLanguageParserCore {
	options := parserTestOptions{defaultToScheduled: false, language: "en"}
	for _, opt := range opts {
		opt(&options)
	}
	return NewNaturalLanguageParserCore(options.statuses, options.priorities, options.defaultToScheduled, options.language, options.triggers, options.userFields, options.parserOptions)
}

type parserTestOptions struct {
	statuses           []StatusConfig
	priorities         []PriorityConfig
	defaultToScheduled bool
	language           string
	triggers           *NLPTriggersConfig
	userFields         []UserMappedField
	parserOptions      ParserOptions
}

func TestExplicitDateTriggers(t *testing.T) {
	for _, input := range []string{"Task due 2026-05-13", "Task scheduled for 2026-05-13"} {
		result := testParser().ParseInput(input)
		if result.Title != "Task" {
			t.Fatalf("%q title = %q", input, result.Title)
		}
	}

	for _, input := range []string{"Scheduled 2026-05-01 Due 2026-05-13", "Start 2026-05-01 Due 2026-05-13"} {
		result := testParser().ParseInput(input)
		if result.Title != "Untitled Task" || result.ScheduledDate != "2026-05-01" || result.DueDate != "2026-05-13" {
			t.Fatalf("%q parsed as %#v", input, result)
		}
	}

	result := testParser().ParseInput("Scheduled for 2026-05-01 Due 2026-05-13")
	if result.Title != "Untitled Task" || result.ScheduledDate != "2026-05-01" || result.DueDate != "2026-05-13" {
		t.Fatalf("long trigger parsed as %#v", result)
	}

	for _, input := range []string{"one 2026-05-01", "only 2026-05-01", "started 2026-05-01"} {
		result := testParser().ParseInput(input)
		if result.Title != input[:len(input)-11] || result.DueDate != "2026-05-01" || result.ScheduledDate != "" {
			t.Fatalf("%q parsed as %#v", input, result)
		}
	}

	german := testParser(func(o *parserTestOptions) { o.language = "de" }).ParseInput("Geplant 2026-05-01 fällig 2026-05-13")
	if german.ScheduledDate != "2026-05-01" || german.DueDate != "2026-05-13" {
		t.Fatalf("german parsed as %#v", german)
	}
}

func TestLocaleAwareNumericDates(t *testing.T) {
	gb := testParser(func(o *parserTestOptions) {
		o.defaultToScheduled = true
		o.parserOptions.DateLocale = "en-GB"
	}).ParseInput("11/06/2026")
	if gb.ScheduledDate != "2026-06-11" {
		t.Fatalf("en-GB date = %#v", gb)
	}

	us := testParser(func(o *parserTestOptions) {
		o.defaultToScheduled = true
		o.parserOptions.DateLocale = "en-US"
	}).ParseInput("11/06/2026")
	if us.ScheduledDate != "2026-11-06" {
		t.Fatalf("en-US date = %#v", us)
	}

	override := testParser(func(o *parserTestOptions) {
		o.defaultToScheduled = true
		o.parserOptions.DateOrder = DateOrderDayFirst
	}).ParseInput("11.06.2026")
	if override.ScheduledDate != "2026-06-11" {
		t.Fatalf("override date = %#v", override)
	}
}

func TestLiteralEscapes(t *testing.T) {
	cases := []struct {
		input string
		title string
	}{
		{`BIO "123H" - HW1`, "BIO 123H - HW1"},
		{`Something "Today"`, "Something Today"},
		{"Read `tomorrow` magazine", "Read tomorrow magazine"},
		{"Review 'Today is the Day' book", "Review Today is the Day book"},
		{`Some task \@ABC`, "Some task @ABC"},
		{`BIO \123H - HW1`, "BIO 123H - HW1"},
		{`Use C:\Users folder`, `Use C:\Users folder`},
	}
	for _, tc := range cases {
		result := testParser(func(o *parserTestOptions) { o.defaultToScheduled = true }).ParseInput(tc.input)
		if result.Title != tc.title || result.Estimate != 0 || result.DueDate != "" || result.ScheduledDate != "" {
			t.Fatalf("%q parsed as %#v", tc.input, result)
		}
	}
}

func TestCustomFieldsStatusPriorityAndCollections(t *testing.T) {
	triggers := NLPTriggersConfig{Triggers: []PropertyTriggerConfig{
		{PropertyID: "assignee", Trigger: "assignee:", Enabled: true},
		{PropertyID: "tags", Trigger: "#", Enabled: true},
		{PropertyID: "contexts", Trigger: "@", Enabled: true},
		{PropertyID: "projects", Trigger: "+", Enabled: true},
	}}
	parser := testParser(func(o *parserTestOptions) {
		o.triggers = &triggers
		o.userFields = []UserMappedField{{ID: "assignee", DisplayName: "Assignee", Key: "assignee", Type: "text"}}
		o.statuses = []StatusConfig{{ID: "done", Value: "done", Label: "Done", Color: "#22c55e"}}
		o.priorities = []PriorityConfig{{ID: "high", Value: "high", Label: "High", Color: "#ef4444"}}
	})
	result := parser.ParseInput(`Done, High: my task #tag @home +[[Big Project]] +small assignee:"John Doe"`)
	if result.Title != "my task" || result.Status != "done" || result.Priority != "high" {
		t.Fatalf("status/priority/title parsed as %#v", result)
	}
	if !reflect.DeepEqual(result.Tags, []string{"tag"}) || !reflect.DeepEqual(result.Contexts, []string{"home"}) || !reflect.DeepEqual(result.Projects, []string{"[[Big Project]]", "small"}) {
		t.Fatalf("collections parsed as %#v", result)
	}
	if result.UserFields["assignee"] != "John Doe" {
		t.Fatalf("user fields parsed as %#v", result.UserFields)
	}
}

func TestFallbackBoundariesAndCJKPriority(t *testing.T) {
	parser := testParser()
	done := parser.ParseInput("Done, file taxes")
	if done.Title != "file taxes" || done.Status != "done" {
		t.Fatalf("done parsed as %#v", done)
	}
	high := parser.ParseInput("High: file taxes")
	if high.Title != "file taxes" || high.Priority != "high" {
		t.Fatalf("high parsed as %#v", high)
	}
	if parser.ParseInput("redone file taxes").Status != "" {
		t.Fatal("matched status inside longer word")
	}
	if parser.ParseInput("highlight file taxes").Priority != "" {
		t.Fatal("matched priority inside longer word")
	}

	japanese := testParser(func(o *parserTestOptions) { o.language = "ja" }).ParseInput("タスク 優先度 高")
	if japanese.Title != "タスク 優先度" || japanese.Priority != "high" {
		t.Fatalf("japanese parsed as %#v", japanese)
	}
	chinese := testParser(func(o *parserTestOptions) { o.language = "zh" }).ParseInput("任务 优先级 高")
	if chinese.Title != "任务 优先级" || chinese.Priority != "high" {
		t.Fatalf("chinese parsed as %#v", chinese)
	}
}

func TestRecurrenceEstimatePreviewAndRelativeDates(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.Local)
	parser := testParser(func(o *parserTestOptions) {
		o.defaultToScheduled = true
		o.parserOptions.Now = func() time.Time { return now }
	})
	result := parser.ParseInput("Write report tomorrow at 3pm every second monday 1h30m")
	if result.Title != "Write report" || result.ScheduledDate != "2026-06-22" || result.ScheduledTime != "15:00" {
		t.Fatalf("date parsed as %#v", result)
	}
	if result.Recurrence != "FREQ=MONTHLY;BYDAY=MO;BYSETPOS=2" || result.Estimate != 90 {
		t.Fatalf("recurrence/estimate parsed as %#v", result)
	}
	preview := parser.GetPreviewText(result)
	if preview == "" {
		t.Fatal("expected preview text")
	}
}

func TestUserFieldTypes(t *testing.T) {
	triggers := NLPTriggersConfig{Triggers: []PropertyTriggerConfig{
		{PropertyID: "labels", Trigger: "label:", Enabled: true},
		{PropertyID: "flagged", Trigger: "flagged:", Enabled: true},
		{PropertyID: "review", Trigger: "review:", Enabled: true},
		{PropertyID: "points", Trigger: "points:", Enabled: true},
	}}
	parser := testParser(func(o *parserTestOptions) {
		o.triggers = &triggers
		o.userFields = []UserMappedField{
			{ID: "labels", DisplayName: "Labels", Key: "labels", Type: "list"},
			{ID: "flagged", DisplayName: "Flagged", Key: "flagged", Type: "boolean"},
			{ID: "review", DisplayName: "Review", Key: "review", Type: "date"},
			{ID: "points", DisplayName: "Points", Key: "points", Type: "number"},
		}
	})

	result := parser.ParseInput(`Ship task label:frontend label:"needs review" flagged:false review:2026-07-01 points:8`)
	if result.Title != "Ship task" {
		t.Fatalf("title = %q", result.Title)
	}
	if !reflect.DeepEqual(result.UserFields["labels"], []string{"frontend", "needs review"}) {
		t.Fatalf("labels = %#v", result.UserFields["labels"])
	}
	for key, want := range map[string]string{"flagged": "false", "review": "2026-07-01", "points": "8"} {
		if result.UserFields[key] != want {
			t.Fatalf("%s = %#v, want %q", key, result.UserFields[key], want)
		}
	}
}

func TestRecurrenceVariants(t *testing.T) {
	cases := []struct {
		input string
		want  string
		title string
	}{
		{"Pay rent every 3 months", "FREQ=MONTHLY;INTERVAL=3", "Pay rent"},
		{"Water plants every other week", "FREQ=WEEKLY;INTERVAL=2", "Water plants"},
		{"Team sync fridays", "FREQ=WEEKLY;BYDAY=FR", "Team sync"},
		{"Backup weekly", "FREQ=WEEKLY", "Backup"},
	}
	parser := testParser()
	for _, tc := range cases {
		result := parser.ParseInput(tc.input)
		if result.Recurrence != tc.want || result.Title != tc.title {
			t.Fatalf("%q parsed as %#v", tc.input, result)
		}
	}
}

func TestImplicitDateTimesAndMonthNames(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.Local)
	parser := testParser(func(o *parserTestOptions) {
		o.defaultToScheduled = true
		o.parserOptions.Now = func() time.Time { return now }
	})

	tuesday := parser.ParseInput("Call Alex tuesday 9:30am")
	if tuesday.Title != "Call Alex" || tuesday.ScheduledDate != "2026-06-23" || tuesday.ScheduledTime != "09:30" {
		t.Fatalf("weekday parsed as %#v", tuesday)
	}

	monthName := parser.ParseInput("Plan launch Jul 4, 2026")
	if monthName.Title != "Plan launch" || monthName.ScheduledDate != "2026-07-04" {
		t.Fatalf("month name parsed as %#v", monthName)
	}
}

func TestDisabledAndCustomCollectionTriggers(t *testing.T) {
	triggers := NLPTriggersConfig{Triggers: []PropertyTriggerConfig{
		{PropertyID: "tags", Trigger: "::", Enabled: true},
		{PropertyID: "contexts", Trigger: "@", Enabled: false},
		{PropertyID: "projects", Trigger: "proj:", Enabled: true},
	}}
	parser := testParser(func(o *parserTestOptions) { o.triggers = &triggers })

	result := parser.ParseInput("Build ::go @office proj:brain #ignored")
	if result.Title != "Build @office #ignored" {
		t.Fatalf("title = %q", result.Title)
	}
	if !reflect.DeepEqual(result.Tags, []string{"go"}) {
		t.Fatalf("tags = %#v", result.Tags)
	}
	if len(result.Contexts) != 0 {
		t.Fatalf("contexts = %#v", result.Contexts)
	}
	if !reflect.DeepEqual(result.Projects, []string{"brain"}) {
		t.Fatalf("projects = %#v", result.Projects)
	}
}

func TestStatusSuggestionsAndPreviewDetails(t *testing.T) {
	parser := testParser(func(o *parserTestOptions) {
		o.statuses = []StatusConfig{
			{ID: "todo", Value: "todo", Label: "To Do"},
			{ID: "done", Value: "done", Label: "Done"},
			{ID: "blocked", Value: "blocked", Label: "Blocked"},
		}
	})

	suggestions := parser.GetStatusSuggestions("do", 2)
	if len(suggestions) != 2 || suggestions[0]["value"] != "todo" || suggestions[1]["value"] != "done" {
		t.Fatalf("suggestions = %#v", suggestions)
	}

	parsed := ParsedTaskData{
		Title:         "Draft plan",
		Details:       "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
		DueDate:       "2026-07-01",
		DueTime:       "14:05",
		Priority:      "high",
		Status:        "todo",
		Tags:          []string{"work"},
		Contexts:      []string{"office"},
		Projects:      []string{"brain"},
		Recurrence:    "FREQ=WEEKLY",
		Estimate:      45,
		UserFields:    map[string]any{"owner": "Jane"},
		ScheduledDate: "2026-06-30",
	}
	preview := parser.GetPreviewText(parsed)
	for _, want := range []string{`"Draft plan"`, "Due: 2026-07-01 at 14:05", "Scheduled: 2026-06-30", "Priority: high", "Status: todo", "Contexts: @office", "Projects: +brain", "Tags: #work", "Recurrence: every week", "Estimate: 45 min", "owner: Jane"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview %q missing %q", preview, want)
		}
	}
	if !strings.Contains(preview, "Details: \"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx...\"") {
		t.Fatalf("preview details not truncated as expected: %q", preview)
	}
}
