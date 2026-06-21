package tnnlpcore

import "sort"

type TriggerConfigService struct {
	config      NLPTriggersConfig
	userFields  []UserMappedField
	triggerMap  map[string]PropertyTriggerConfig
	propertyMap map[string]PropertyTriggerConfig
}

func NewTriggerConfigService(config NLPTriggersConfig, userFields []UserMappedField) *TriggerConfigService {
	s := &TriggerConfigService{config: config, userFields: userFields}
	s.buildMaps()
	return s
}

func (s *TriggerConfigService) buildMaps() {
	s.triggerMap = map[string]PropertyTriggerConfig{}
	s.propertyMap = map[string]PropertyTriggerConfig{}
	for _, trigger := range s.config.Triggers {
		if trigger.Enabled {
			s.triggerMap[trigger.Trigger] = trigger
			s.propertyMap[trigger.PropertyID] = trigger
		}
	}
}

func (s *TriggerConfigService) GetTriggerForProperty(propertyID string) (PropertyTriggerConfig, bool) {
	trigger, ok := s.propertyMap[propertyID]
	return trigger, ok
}

func (s *TriggerConfigService) GetPropertyForTrigger(trigger string) (string, bool) {
	cfg, ok := s.triggerMap[trigger]
	return cfg.PropertyID, ok
}

func (s *TriggerConfigService) GetAllEnabledTriggers() []PropertyTriggerConfig {
	out := make([]PropertyTriggerConfig, 0, len(s.config.Triggers))
	for _, trigger := range s.config.Triggers {
		if trigger.Enabled {
			out = append(out, trigger)
		}
	}
	return out
}

func (s *TriggerConfigService) GetTriggersOrderedByLength() []PropertyTriggerConfig {
	out := s.GetAllEnabledTriggers()
	sort.SliceStable(out, func(i, j int) bool {
		return len([]rune(out[i].Trigger)) > len([]rune(out[j].Trigger))
	})
	return out
}

func (s *TriggerConfigService) UsesNativeTagSuggester() bool {
	trigger, ok := s.GetTriggerForProperty("tags")
	return ok && trigger.Enabled && trigger.Trigger == "#"
}

func (s *TriggerConfigService) triggerFor(propertyID string) string {
	trigger, ok := s.GetTriggerForProperty(propertyID)
	if !ok || !trigger.Enabled {
		return ""
	}
	return trigger.Trigger
}

func (s *TriggerConfigService) GetTagTrigger() string      { return s.triggerFor("tags") }
func (s *TriggerConfigService) GetContextTrigger() string  { return s.triggerFor("contexts") }
func (s *TriggerConfigService) GetProjectTrigger() string  { return s.triggerFor("projects") }
func (s *TriggerConfigService) GetStatusTrigger() string   { return s.triggerFor("status") }
func (s *TriggerConfigService) GetPriorityTrigger() string { return s.triggerFor("priority") }

func (s *TriggerConfigService) GetUserField(fieldID string) (UserMappedField, bool) {
	for _, field := range s.userFields {
		if field.ID == fieldID {
			return field, true
		}
	}
	return UserMappedField{}, false
}

func (s *TriggerConfigService) IsUserField(propertyID string) bool {
	_, ok := s.GetUserField(propertyID)
	return ok
}

func (s *TriggerConfigService) GetSuggesterType(propertyID string) string {
	switch propertyID {
	case "tags":
		if s.UsesNativeTagSuggester() {
			return "native-tag"
		}
		return "list"
	case "contexts":
		return "list"
	case "projects":
		return "file"
	case "status":
		return "status"
	case "priority":
		return "priority"
	}

	field, ok := s.GetUserField(propertyID)
	if !ok {
		return "none"
	}
	switch field.Type {
	case "text":
		if field.AutosuggestFilter != nil {
			return "file"
		}
		return "list"
	case "list":
		return "list"
	case "boolean":
		return "boolean"
	default:
		return "none"
	}
}

func (s *TriggerConfigService) UpdateConfig(config NLPTriggersConfig) {
	s.config = config
	s.buildMaps()
}

func (s *TriggerConfigService) UpdateUserFields(userFields []UserMappedField) {
	s.userFields = userFields
}
