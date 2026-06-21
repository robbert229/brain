package tnnlpcore

import (
	"reflect"
	"testing"
)

func TestTriggerConfigServiceLookupsAndOrdering(t *testing.T) {
	config := NLPTriggersConfig{Triggers: []PropertyTriggerConfig{
		{PropertyID: "tags", Trigger: "#", Enabled: true},
		{PropertyID: "contexts", Trigger: "@@", Enabled: true},
		{PropertyID: "projects", Trigger: "+", Enabled: false},
		{PropertyID: "status", Trigger: "::status:", Enabled: true},
	}}
	service := NewTriggerConfigService(config, nil)

	if trigger, ok := service.GetTriggerForProperty("contexts"); !ok || trigger.Trigger != "@@" {
		t.Fatalf("context trigger = %#v, %v", trigger, ok)
	}
	if property, ok := service.GetPropertyForTrigger("::status:"); !ok || property != "status" {
		t.Fatalf("status property = %q, %v", property, ok)
	}
	if service.GetProjectTrigger() != "" {
		t.Fatalf("disabled project trigger = %q", service.GetProjectTrigger())
	}

	ordered := service.GetTriggersOrderedByLength()
	got := []string{ordered[0].Trigger, ordered[1].Trigger, ordered[2].Trigger}
	if !reflect.DeepEqual(got, []string{"::status:", "@@", "#"}) {
		t.Fatalf("ordered triggers = %#v", got)
	}
}

func TestTriggerConfigServiceSuggesterTypesAndUpdates(t *testing.T) {
	service := NewTriggerConfigService(DefaultNLPTriggers, []UserMappedField{
		{ID: "source", Type: "text", AutosuggestFilter: "folder"},
		{ID: "choice", Type: "text"},
		{ID: "flag", Type: "boolean"},
		{ID: "labels", Type: "list"},
		{ID: "dueish", Type: "date"},
	})

	cases := map[string]string{
		"tags":     "native-tag",
		"contexts": "list",
		"projects": "file",
		"status":   "status",
		"priority": "priority",
		"source":   "file",
		"choice":   "list",
		"flag":     "boolean",
		"labels":   "list",
		"dueish":   "none",
		"missing":  "none",
	}
	for property, want := range cases {
		if got := service.GetSuggesterType(property); got != want {
			t.Fatalf("GetSuggesterType(%q) = %q, want %q", property, got, want)
		}
	}

	service.UpdateConfig(NLPTriggersConfig{Triggers: []PropertyTriggerConfig{{PropertyID: "tags", Trigger: "::", Enabled: true}}})
	if service.UsesNativeTagSuggester() {
		t.Fatal("custom tag trigger should not use native tag suggester")
	}
	if service.GetTagTrigger() != "::" {
		t.Fatalf("tag trigger after update = %q", service.GetTagTrigger())
	}

	service.UpdateUserFields([]UserMappedField{{ID: "new_flag", Type: "boolean"}})
	if !service.IsUserField("new_flag") || service.IsUserField("flag") {
		t.Fatalf("updated user field set not reflected")
	}
	if got := service.GetSuggesterType("new_flag"); got != "boolean" {
		t.Fatalf("new flag suggester = %q", got)
	}
}
