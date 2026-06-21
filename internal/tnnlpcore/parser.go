package tnnlpcore

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type NaturalLanguageParserCore struct {
	statusConfigs      []StatusConfig
	priorityConfigs    []PriorityConfig
	defaultToScheduled bool
	languageConfig     languageConfig
	triggerConfig      *TriggerConfigService
	options            ParserOptions
}

type phraseMatch struct {
	full       string
	phrase     string
	start, end int
}

type dateMatch struct {
	date        time.Time
	hasTime     bool
	timeText    string
	matchedText string
	start, end  int
}

func NewNaturalLanguageParserCore(statuses []StatusConfig, priorities []PriorityConfig, defaultToScheduled bool, languageCode string, triggers *NLPTriggersConfig, userFields []UserMappedField, options ParserOptions) *NaturalLanguageParserCore {
	effective := DefaultNLPTriggers
	if triggers != nil {
		effective = *triggers
	}
	return &NaturalLanguageParserCore{
		statusConfigs:      statuses,
		priorityConfigs:    priorities,
		defaultToScheduled: defaultToScheduled,
		languageConfig:     getLanguageConfig(languageCode),
		triggerConfig:      NewTriggerConfigService(effective, userFields),
		options:            options,
	}
}

func (p *NaturalLanguageParserCore) ParseInput(input string) ParsedTaskData {
	result := ParsedTaskData{Tags: []string{}, Contexts: []string{}, Projects: []string{}}
	protectedText, literals := p.protectNLPLiterals(input)
	normalized := p.normalizeNumericDateLiterals(protectedText)
	working, details := extractTitleAndDetails(normalized)
	if details != "" {
		result.Details = details
	}

	working = p.extractTags(working, &result)
	working = p.extractContexts(working, &result)
	working = p.extractProjects(working, &result)
	working = p.extractPriority(working, &result)
	working = p.extractStatus(working, &result)
	working = p.extractRecurrence(working, &result)
	working = p.extractTimeEstimate(working, &result)
	working = p.extractUserFields(working, &result)
	working = p.parseUnifiedDatesAndTimes(working, &result)

	result.Title = strings.TrimSpace(working)
	result = p.validateAndCleanupResult(result)
	p.restoreProtectedLiterals(&result, literals)
	return result
}

func (p *NaturalLanguageParserCore) protectNLPLiterals(input string) (string, []string) {
	text, literals := p.protectQuotedLiterals(input)
	return p.protectEscapedLiterals(text, literals)
}

func (p *NaturalLanguageParserCore) protectQuotedLiterals(input string) (string, []string) {
	var out strings.Builder
	literals := []string{}
	for i := 0; i < len(input); {
		ch, size := utf8.DecodeRuneInString(input[i:])
		if !isQuoteDelimiter(ch) || !p.isEligibleQuoteStart(input, i) {
			out.WriteString(input[i : i+size])
			i += size
			continue
		}
		end := findClosingQuote(input, i+size, ch)
		if end == -1 || !p.isEligibleQuoteEnd(input, end) {
			out.WriteString(input[i : i+size])
			i += size
			continue
		}
		literal := input[i+size : end]
		if strings.TrimSpace(literal) == "" {
			out.WriteString(input[i : i+size])
			i += size
			continue
		}
		out.WriteString(fmt.Sprintf("__TASKNOTES_NLP_LITERAL_%d__", len(literals)))
		literals = append(literals, literal)
		_, quoteSize := utf8.DecodeRuneInString(input[end:])
		i = end + quoteSize
	}
	return out.String(), literals
}

func (p *NaturalLanguageParserCore) protectEscapedLiterals(input string, literals []string) (string, []string) {
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] != '\\' || !p.shouldProtectEscapedLiteral(input, i) {
			r, size := utf8.DecodeRuneInString(input[i:])
			out.WriteRune(r)
			i += size
			continue
		}
		start := i + 1
		end := start
		for end < len(input) {
			r, size := utf8.DecodeRuneInString(input[end:])
			if unicode.IsSpace(r) {
				break
			}
			end += size
		}
		literals = append(literals, input[start:end])
		out.WriteString(fmt.Sprintf("__TASKNOTES_NLP_LITERAL_%d__", len(literals)-1))
		i = end
	}
	return out.String(), literals
}

func (p *NaturalLanguageParserCore) restoreProtectedLiterals(parsed *ParsedTaskData, literals []string) {
	if len(literals) == 0 {
		return
	}
	parsed.Title = cleanupWhitespace(restoreLiterals(parsed.Title, literals))
	parsed.Details = restoreLiterals(parsed.Details, literals)
	for key, value := range parsed.UserFields {
		switch v := value.(type) {
		case string:
			parsed.UserFields[key] = restoreLiterals(v, literals)
		case []string:
			for i := range v {
				v[i] = restoreLiterals(v[i], literals)
			}
			parsed.UserFields[key] = v
		}
	}
}

func restoreLiterals(text string, literals []string) string {
	for i, literal := range literals {
		text = strings.ReplaceAll(text, fmt.Sprintf("__TASKNOTES_NLP_LITERAL_%d__", i), literal)
	}
	return text
}

func isQuoteDelimiter(r rune) bool { return r == '"' || r == '\'' || r == '`' }

func (p *NaturalLanguageParserCore) isEligibleQuoteStart(input string, index int) bool {
	if input[index] != '\'' {
		return true
	}
	return !isWordRune(runeBefore(input, index))
}

func (p *NaturalLanguageParserCore) isEligibleQuoteEnd(input string, index int) bool {
	if input[index] != '\'' {
		return true
	}
	return !isWordRune(runeAt(input, index+1))
}

func findClosingQuote(input string, start int, delimiter rune) int {
	for i := start; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		if r == delimiter && (i == 0 || input[i-1] != '\\') {
			return i
		}
		i += size
	}
	return -1
}

func (p *NaturalLanguageParserCore) shouldProtectEscapedLiteral(input string, escapeIndex int) bool {
	literalStart := escapeIndex + 1
	if literalStart >= len(input) {
		return false
	}
	for _, trigger := range p.escapableTriggers() {
		if strings.HasPrefix(input[literalStart:], trigger) {
			return true
		}
	}
	if prev := runeBefore(input, escapeIndex); prev != 0 && !unicode.IsSpace(prev) {
		return false
	}
	return isWordRune(runeAt(input, literalStart))
}

func (p *NaturalLanguageParserCore) escapableTriggers() []string {
	configured := p.triggerConfig.GetTriggersOrderedByLength()
	if len(configured) == 0 {
		return []string{"#", "@", "+", "*", "!"}
	}
	out := make([]string, 0, len(configured))
	for _, trigger := range configured {
		if trigger.Trigger != "" {
			out = append(out, trigger.Trigger)
		}
	}
	return out
}

func extractTitleAndDetails(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if idx := strings.Index(trimmed, "\n"); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx]), strings.TrimSpace(trimmed[idx+1:])
	}
	return trimmed, ""
}

func (p *NaturalLanguageParserCore) extractTags(text string, result *ParsedTaskData) string {
	return p.extractTriggeredWords(text, p.triggerConfig.GetTagTrigger(), &result.Tags)
}

func (p *NaturalLanguageParserCore) extractContexts(text string, result *ParsedTaskData) string {
	return p.extractTriggeredWords(text, p.triggerConfig.GetContextTrigger(), &result.Contexts)
}

func (p *NaturalLanguageParserCore) extractTriggeredWords(text, trigger string, target *[]string) string {
	if trigger == "" {
		return text
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(trigger) + `[\p{L}\p{N}\p{M}_/-]+`)
	matches := pattern.FindAllString(text, -1)
	for _, match := range matches {
		*target = append(*target, match[len(trigger):])
	}
	return cleanupWhitespace(pattern.ReplaceAllString(text, ""))
}

func (p *NaturalLanguageParserCore) extractProjects(text string, result *ParsedTaskData) string {
	trigger := p.triggerConfig.GetProjectTrigger()
	if trigger == "" {
		return text
	}
	working := text
	wikilink := regexp.MustCompile(regexp.QuoteMeta(trigger) + `\[\[.*?\]\]`)
	for _, match := range wikilink.FindAllString(working, -1) {
		result.Projects = append(result.Projects, match[len(trigger):])
	}
	working = cleanupWhitespace(wikilink.ReplaceAllString(working, ""))
	return p.extractTriggeredWords(working, trigger, &result.Projects)
}

func (p *NaturalLanguageParserCore) extractUserFields(text string, result *ParsedTaskData) string {
	working := text
	for _, triggerDef := range p.triggerConfig.GetAllEnabledTriggers() {
		if !p.triggerConfig.IsUserField(triggerDef.PropertyID) {
			continue
		}
		field, _ := p.triggerConfig.GetUserField(triggerDef.PropertyID)
		pattern := regexp.MustCompile(regexp.QuoteMeta(triggerDef.Trigger) + `(?:"([^"]+)"|([\p{L}\p{N}\p{M}_/-]+))`)
		if field.Type == "list" {
			values := []string{}
			for _, match := range pattern.FindAllStringSubmatch(working, -1) {
				values = append(values, firstNonEmpty(match[1], match[2]))
			}
			if len(values) > 0 {
				if result.UserFields == nil {
					result.UserFields = map[string]any{}
				}
				result.UserFields[field.ID] = values
				working = cleanupWhitespace(pattern.ReplaceAllString(working, ""))
			}
			continue
		}
		match := pattern.FindStringSubmatch(working)
		if len(match) == 0 {
			continue
		}
		value := firstNonEmpty(match[1], match[2])
		if field.Type == "boolean" && strings.ToLower(value) != "true" {
			value = "false"
		}
		if result.UserFields == nil {
			result.UserFields = map[string]any{}
		}
		result.UserFields[field.ID] = value
		working = cleanupWhitespace(pattern.ReplaceAllString(working, ""))
	}
	return working
}

func (p *NaturalLanguageParserCore) extractPriority(text string, result *ParsedTaskData) string {
	if len(p.priorityConfigs) > 0 {
		configs := append([]PriorityConfig(nil), p.priorityConfigs...)
		sort.SliceStable(configs, func(i, j int) bool { return len(configs[i].Label) > len(configs[j].Label) })
		trigger := p.triggerConfig.GetPriorityTrigger()
		for _, cfg := range configs {
			for _, candidate := range []string{cfg.Label, cfg.Value} {
				if strings.TrimSpace(candidate) == "" {
					continue
				}
				if trigger != "" {
					if match := p.findTextMatch(text, trigger+candidate); match != nil {
						result.Priority = cfg.Value
						return cleanupWhitespace(text[:match.start] + text[match.end:])
					}
				}
				if match := p.findTextMatch(text, candidate); match != nil {
					result.Priority = cfg.Value
					return cleanupWhitespace(text[:match.start] + text[match.end:])
				}
			}
		}
		return text
	}
	if value, match := p.findPhraseValueMatch(text, map[string][]string{
		"urgent": p.languageConfig.FallbackPriority.Urgent,
		"high":   p.languageConfig.FallbackPriority.High,
		"normal": p.languageConfig.FallbackPriority.Normal,
		"low":    p.languageConfig.FallbackPriority.Low,
	}); match != nil {
		result.Priority = value
		return p.removePhraseMatch(text, *match)
	}
	return text
}

func (p *NaturalLanguageParserCore) extractStatus(text string, result *ParsedTaskData) string {
	if len(p.statusConfigs) > 0 {
		configs := append([]StatusConfig(nil), p.statusConfigs...)
		sort.SliceStable(configs, func(i, j int) bool { return len(configs[i].Label) > len(configs[j].Label) })
		trigger := p.triggerConfig.GetStatusTrigger()
		for _, cfg := range configs {
			for _, candidate := range []string{cfg.Label, cfg.Value} {
				if strings.TrimSpace(candidate) == "" {
					continue
				}
				if trigger != "" {
					if match := p.findTextMatch(text, trigger+candidate); match != nil {
						result.Status = cfg.Value
						return cleanupWhitespace(text[:match.start] + text[match.end:])
					}
				}
				if match := p.findTextMatch(text, candidate); match != nil {
					result.Status = cfg.Value
					return cleanupWhitespace(text[:match.start] + text[match.end:])
				}
			}
		}
		return text
	}
	if value, match := p.findPhraseValueMatch(text, map[string][]string{
		"open":        p.languageConfig.FallbackStatus.Open,
		"in-progress": p.languageConfig.FallbackStatus.InProgress,
		"done":        p.languageConfig.FallbackStatus.Done,
		"cancelled":   p.languageConfig.FallbackStatus.Cancelled,
		"waiting":     p.languageConfig.FallbackStatus.Waiting,
	}); match != nil {
		result.Status = value
		return p.removePhraseMatch(text, *match)
	}
	return text
}

func (p *NaturalLanguageParserCore) findTextMatch(text, search string) *phraseMatch {
	match := p.findPhraseMatch(text, []string{search})
	if match == nil {
		return nil
	}
	match.end = includeTrailingPhraseSeparator(text, match.end)
	match.full = text[match.start:match.end]
	return match
}

func (p *NaturalLanguageParserCore) findPhraseValueMatch(text string, groups map[string][]string) (string, *phraseMatch) {
	var best *phraseMatch
	bestValue := ""
	for value, phrases := range groups {
		match := p.findPhraseMatch(text, phrases)
		if match != nil && (best == nil || match.start < best.start || (match.start == best.start && len(match.phrase) > len(best.phrase))) {
			copied := *match
			best = &copied
			bestValue = value
		}
	}
	return bestValue, best
}

func (p *NaturalLanguageParserCore) findPhraseMatch(text string, phrases []string) *phraseMatch {
	searchable := []string{}
	for _, phrase := range phrases {
		if strings.TrimSpace(phrase) != "" {
			searchable = append(searchable, phrase)
		}
	}
	sort.SliceStable(searchable, func(i, j int) bool { return len(searchable[i]) > len(searchable[j]) })
	lower := strings.ToLower(text)
	var best *phraseMatch
	for _, phrase := range searchable {
		needle := strings.ToLower(phrase)
		for start := 0; start <= len(lower); {
			idx := strings.Index(lower[start:], needle)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + len(needle)
			if p.hasPhraseBoundaries(text, idx, end) {
				candidate := &phraseMatch{full: text[idx:end], phrase: phrase, start: idx, end: end}
				if best == nil || candidate.start < best.start || (candidate.start == best.start && len(candidate.phrase) > len(best.phrase)) {
					best = candidate
				}
			}
			_, size := utf8.DecodeRuneInString(lower[idx:])
			if size == 0 {
				size = 1
			}
			start = idx + size
		}
	}
	return best
}

func (p *NaturalLanguageParserCore) hasPhraseBoundaries(text string, start, end int) bool {
	if p.languageConfig.Code == "zh" || p.languageConfig.Code == "ja" {
		return true
	}
	return !isWordRune(runeBefore(text, start)) && !isWordRune(runeAt(text, end))
}

func (p *NaturalLanguageParserCore) removePhraseMatch(text string, match phraseMatch) string {
	end := includeTrailingPhraseSeparator(text, match.end)
	return cleanupWhitespace(text[:match.start] + text[end:])
}

func includeTrailingPhraseSeparator(text string, end int) int {
	if end < len(text) && strings.ContainsRune(":,;", rune(text[end])) {
		return end + 1
	}
	return end
}

func (p *NaturalLanguageParserCore) extractRecurrence(text string, result *ParsedTaskData) string {
	if recurrence, start, end, ok := p.matchRecurrence(text); ok {
		result.Recurrence = recurrence
		return cleanupWhitespace(text[:start] + text[end:])
	}
	return text
}

func (p *NaturalLanguageParserCore) matchRecurrence(text string) (string, int, int, bool) {
	lower := strings.ToLower(text)
	weekdays := p.weekdayWords(false)
	plurals := p.weekdayWords(true)
	ordinals := p.ordinalWords()
	periods := p.periodWords()

	for _, every := range p.languageConfig.Recurrence.Every {
		for _, ordinal := range ordinals {
			for day, words := range weekdays {
				for _, word := range words {
					if m := p.findPhraseMatch(lower, []string{every + " " + ordinal.word + " " + word}); m != nil {
						return "FREQ=MONTHLY;BYDAY=" + day + ";BYSETPOS=" + strconv.Itoa(ordinal.pos), m.start, m.end, true
					}
				}
			}
		}
		reInterval := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(every) + `\s+(\d+)\s+(` + regexp.QuoteMeta(strings.Join(periods.all, "\x00")) + `)\b`)
		if loc, freq, interval, ok := matchEveryInterval(reInterval, lower, periods); ok {
			return "FREQ=" + freq + ";INTERVAL=" + interval, loc[0], loc[1], true
		}
		for _, other := range p.languageConfig.Recurrence.Other {
			for _, period := range periods.all {
				if m := p.findPhraseMatch(lower, []string{every + " " + other + " " + period}); m != nil {
					return "FREQ=" + periods.freq(period) + ";INTERVAL=2", m.start, m.end, true
				}
			}
		}
		for day, words := range weekdays {
			for _, word := range words {
				if m := p.findPhraseMatch(lower, []string{every + " " + word}); m != nil {
					return "FREQ=WEEKLY;BYDAY=" + day, m.start, m.end, true
				}
			}
		}
	}
	for day, words := range plurals {
		if m := p.findPhraseMatch(lower, words); m != nil {
			return "FREQ=WEEKLY;BYDAY=" + day, m.start, m.end, true
		}
	}
	for freq, phrases := range map[string][]string{
		"DAILY":   p.languageConfig.Recurrence.Frequencies.Daily,
		"WEEKLY":  p.languageConfig.Recurrence.Frequencies.Weekly,
		"MONTHLY": p.languageConfig.Recurrence.Frequencies.Monthly,
		"YEARLY":  p.languageConfig.Recurrence.Frequencies.Yearly,
	} {
		if m := p.findPhraseMatch(lower, phrases); m != nil {
			return "FREQ=" + freq, m.start, m.end, true
		}
	}
	return "", 0, 0, false
}

type ordinalWord struct {
	word string
	pos  int
}
type periodWords struct{ all, days, weeks, months, years []string }

func (p periodWords) freq(word string) string {
	for _, w := range p.weeks {
		if word == w {
			return "WEEKLY"
		}
	}
	for _, w := range p.months {
		if word == w {
			return "MONTHLY"
		}
	}
	for _, w := range p.years {
		if word == w {
			return "YEARLY"
		}
	}
	return "DAILY"
}

func (p *NaturalLanguageParserCore) ordinalWords() []ordinalWord {
	o := p.languageConfig.Recurrence.Ordinals
	var out []ordinalWord
	for _, w := range o.First {
		out = append(out, ordinalWord{w, 1})
	}
	for _, w := range o.Second {
		out = append(out, ordinalWord{w, 2})
	}
	for _, w := range o.Third {
		out = append(out, ordinalWord{w, 3})
	}
	for _, w := range o.Fourth {
		out = append(out, ordinalWord{w, 4})
	}
	for _, w := range o.Last {
		out = append(out, ordinalWord{w, -1})
	}
	return out
}

func (p *NaturalLanguageParserCore) periodWords() periodWords {
	periods := p.languageConfig.Recurrence.Periods
	out := periodWords{days: periods.Day, weeks: periods.Week, months: periods.Month, years: periods.Year}
	out.all = append(append(append(append([]string{}, out.days...), out.weeks...), out.months...), out.years...)
	return out
}

func (p *NaturalLanguageParserCore) weekdayWords(plural bool) map[string][]string {
	w := p.languageConfig.Recurrence.Weekdays
	if plural {
		w = p.languageConfig.Recurrence.PluralWeekdays
	}
	return map[string][]string{"MO": w.Monday, "TU": w.Tuesday, "WE": w.Wednesday, "TH": w.Thursday, "FR": w.Friday, "SA": w.Saturday, "SU": w.Sunday}
}

func matchEveryInterval(_ *regexp.Regexp, lower string, periods periodWords) ([]int, string, string, bool) {
	for _, period := range periods.all {
		re := regexp.MustCompile(`(?i)\bevery\s+(\d+)\s+` + regexp.QuoteMeta(period) + `\b`)
		if loc := re.FindStringSubmatchIndex(lower); loc != nil {
			return loc, periods.freq(period), lower[loc[2]:loc[3]], true
		}
	}
	return nil, "", "", false
}

func (p *NaturalLanguageParserCore) extractTimeEstimate(text string, result *ParsedTaskData) string {
	working := text
	total := 0
	hours := unitAlt(p.languageConfig.TimeEstimate.Hours)
	minutes := unitAlt(p.languageConfig.TimeEstimate.Minutes)
	for _, pattern := range []struct {
		re          *regexp.Regexp
		handler     func([]string) int
		replacement string
	}{
		{compileUnitRegex(`(\d+)\s*`, p.languageConfig.TimeEstimate.Hours, `\s*(\d+)\s*`, p.languageConfig.TimeEstimate.Minutes), func(m []string) int {
			h, _ := strconv.Atoi(m[2])
			min, _ := strconv.Atoi(m[4])
			return h*60 + min
		}, "${1}${6}"},
		{regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(\d+)\s*(` + hours + `)($|[^\p{L}\p{N}_])`), func(m []string) int { h, _ := strconv.Atoi(m[2]); return h * 60 }, "${1}${4}"},
		{regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(\d+)\s*(` + minutes + `)($|[^\p{L}\p{N}_])`), func(m []string) int { min, _ := strconv.Atoi(m[2]); return min }, "${1}${4}"},
	} {
		if match := pattern.re.FindStringSubmatch(working); len(match) > 0 {
			total += pattern.handler(match)
			working = cleanupWhitespace(pattern.re.ReplaceAllString(working, pattern.replacement))
		}
	}
	if total > 0 {
		result.Estimate = total
	}
	return working
}

func compileUnitRegex(prefix string, hours []string, middle string, minutes []string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])` + prefix + `(` + strings.Join(quoteAll(hours), "|") + `)` + middle + `(` + strings.Join(quoteAll(minutes), "|") + `)($|[^\p{L}\p{N}_])`)
}

func unitAlt(values []string) string {
	return strings.Join(quoteAll(values), "|")
}

func quoteAll(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = regexp.QuoteMeta(value)
	}
	return out
}

func (p *NaturalLanguageParserCore) parseUnifiedDatesAndTimes(text string, result *ParsedTaskData) string {
	working := text
	foundExplicit := false
	for _, group := range []struct {
		kind    string
		phrases []string
	}{{"due", p.languageConfig.DateTriggers.Due}, {"scheduled", p.languageConfig.DateTriggers.Scheduled}} {
		if match := p.findPhraseMatch(working, group.phrases); match != nil {
			remaining := working[match.end:]
			if parsed, ok := p.parseDateFromBeginning(remaining); ok {
				foundExplicit = true
				if group.kind == "due" {
					result.DueDate = formatDate(parsed.date)
					if parsed.hasTime {
						result.DueTime = parsed.timeText
					}
				} else {
					result.ScheduledDate = formatDate(parsed.date)
					if parsed.hasTime {
						result.ScheduledTime = parsed.timeText
					}
				}
				dateEnd := match.end + parsed.end
				working = cleanupWhitespace(working[:match.start] + working[dateEnd:])
			}
		}
	}
	if foundExplicit {
		return working
	}
	if parsed, ok := p.findDateInText(text); ok {
		dateString := formatDate(parsed.date)
		if p.defaultToScheduled {
			result.ScheduledDate = dateString
			if parsed.hasTime {
				result.ScheduledTime = parsed.timeText
			}
		} else {
			result.DueDate = dateString
			if parsed.hasTime {
				result.DueTime = parsed.timeText
			}
		}
		working = cleanupWhitespace(text[:parsed.start] + text[parsed.end:])
	}
	return working
}

func (p *NaturalLanguageParserCore) parseDateFromBeginning(text string) (dateMatch, bool) {
	trimmedLeft := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	candidates := []string{"for", "on", "at", "am", "le", "für", "fur"}
	offsetText := strings.TrimLeftFunc(text, unicode.IsSpace)
	offset := trimmedLeft
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(offsetText), c+" ") {
			offset += len(c) + 1
			offsetText = offsetText[len(c)+1:]
			break
		}
	}
	if parsed, ok := p.findDateInText(offsetText); ok && parsed.start <= 3 {
		parsed.start += offset
		parsed.end += offset
		return parsed, true
	}
	return dateMatch{}, false
}

func (p *NaturalLanguageParserCore) findDateInText(text string) (dateMatch, bool) {
	now := p.now()
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(\d{4})-(\d{2})-(\d{2})(?:\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?)?($|[^\p{L}\p{N}_])`),
		regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(\d{1,2})[/.](\d{1,2})[/.](\d{4})(?:\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?)?($|[^\p{L}\p{N}_])`),
	}
	for idx, re := range patterns {
		if loc := re.FindStringSubmatchIndex(text); loc != nil {
			m := re.FindStringSubmatch(text)
			prefixLen := len(m[1])
			var y, mo, d int
			if idx == 0 {
				y, _ = strconv.Atoi(m[2])
				mo, _ = strconv.Atoi(m[3])
				d, _ = strconv.Atoi(m[4])
			} else {
				a, _ := strconv.Atoi(m[2])
				b, _ := strconv.Atoi(m[3])
				y, _ = strconv.Atoi(m[4])
				if p.numericDateOrder() == DateOrderDayFirst {
					d, mo = a, b
				} else {
					mo, d = a, b
				}
			}
			if t, ok := validDate(y, time.Month(mo), d); ok {
				trailingLen := len(m[len(m)-1])
				parsed := dateMatch{date: t, matchedText: strings.TrimSpace(m[0]), start: loc[0] + prefixLen, end: loc[1] - trailingLen}
				if len(m) > 5 && m[5] != "" {
					h, _ := strconv.Atoi(m[5])
					min := 0
					if m[6] != "" {
						min, _ = strconv.Atoi(m[6])
					}
					if strings.EqualFold(m[7], "pm") && h < 12 {
						h += 12
					}
					if strings.EqualFold(m[7], "am") && h == 12 {
						h = 0
					}
					parsed.hasTime = true
					parsed.timeText = fmt.Sprintf("%02d:%02d", h, min)
				}
				return parsed, true
			}
		}
	}
	relative := map[string]int{"today": 0, "tomorrow": 1, "yesterday": -1}
	for word, delta := range relative {
		if m := p.findPhraseMatch(text, []string{word}); m != nil {
			d := now.AddDate(0, 0, delta)
			parsed := dateMatch{date: d, matchedText: m.full, start: m.start, end: m.end}
			parsed = p.attachFollowingTime(text, parsed)
			return parsed, true
		}
	}
	if parsed, ok := p.findWeekdayDate(text, now); ok {
		return parsed, true
	}
	if parsed, ok := p.findMonthNameDate(text, now); ok {
		return parsed, true
	}
	return dateMatch{}, false
}

func (p *NaturalLanguageParserCore) attachFollowingTime(text string, parsed dateMatch) dateMatch {
	re := regexp.MustCompile(`(?i)^\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)\b`)
	rest := text[parsed.end:]
	if m := re.FindStringSubmatch(rest); len(m) > 0 {
		h, _ := strconv.Atoi(m[1])
		min := 0
		if m[2] != "" {
			min, _ = strconv.Atoi(m[2])
		}
		if strings.EqualFold(m[3], "pm") && h < 12 {
			h += 12
		}
		if strings.EqualFold(m[3], "am") && h == 12 {
			h = 0
		}
		parsed.hasTime = true
		parsed.timeText = fmt.Sprintf("%02d:%02d", h, min)
		parsed.end += len(m[0])
	}
	return parsed
}

func (p *NaturalLanguageParserCore) findWeekdayDate(text string, now time.Time) (dateMatch, bool) {
	weekdayMap := map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday}
	for word, weekday := range weekdayMap {
		if m := p.findPhraseMatch(text, []string{word}); m != nil {
			delta := (int(weekday) - int(now.Weekday()) + 7) % 7
			if delta == 0 {
				delta = 7
			}
			parsed := dateMatch{date: now.AddDate(0, 0, delta), matchedText: m.full, start: m.start, end: m.end}
			return p.attachFollowingTime(text, parsed), true
		}
	}
	return dateMatch{}, false
}

func (p *NaturalLanguageParserCore) findMonthNameDate(text string, now time.Time) (dateMatch, bool) {
	months := map[string]time.Month{"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3, "apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7, "aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12}
	re := regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(` + strings.Join(keys(months), "|") + `)\s+(\d{1,2})(?:,\s*(\d{4}))?($|[^\p{L}\p{N}_])`)
	if loc := re.FindStringSubmatchIndex(text); loc != nil {
		m := re.FindStringSubmatch(text)
		day, _ := strconv.Atoi(m[3])
		year := now.Year()
		if m[4] != "" {
			year, _ = strconv.Atoi(m[4])
		}
		if d, ok := validDate(year, months[strings.ToLower(m[2])], day); ok {
			prefix := len(m[1])
			return dateMatch{date: d, matchedText: strings.TrimSpace(m[0]), start: loc[0] + prefix, end: loc[1] - len(m[5])}, true
		}
	}
	return dateMatch{}, false
}

func (p *NaturalLanguageParserCore) normalizeNumericDateLiterals(input string) string {
	yearFirst := regexp.MustCompile(`(^|[^\p{L}\p{N}_])(\d{4})[/.](\d{1,2})[/.](\d{1,2})($|[^\p{L}\p{N}_])`)
	out := replaceDateLiterals(input, yearFirst, func(m []string) (int, int, int) {
		y, _ := strconv.Atoi(m[2])
		mo, _ := strconv.Atoi(m[3])
		d, _ := strconv.Atoi(m[4])
		return y, mo, d
	})
	if p.numericDateOrder() != DateOrderDayFirst {
		return out
	}
	dayFirst := regexp.MustCompile(`(^|[^\p{L}\p{N}_])(\d{1,2})[/.](\d{1,2})[/.](\d{4})($|[^\p{L}\p{N}_])`)
	return replaceDateLiterals(out, dayFirst, func(m []string) (int, int, int) {
		d, _ := strconv.Atoi(m[2])
		mo, _ := strconv.Atoi(m[3])
		y, _ := strconv.Atoi(m[4])
		return y, mo, d
	})
}

func replaceDateLiterals(input string, re *regexp.Regexp, parts func([]string) (int, int, int)) string {
	return re.ReplaceAllStringFunc(input, func(match string) string {
		m := re.FindStringSubmatch(match)
		if len(m) == 0 {
			return match
		}
		y, mo, d := parts(m)
		if _, ok := validDate(y, time.Month(mo), d); !ok {
			return match
		}
		return fmt.Sprintf("%s%04d-%02d-%02d%s", m[1], y, mo, d, m[5])
	})
}

func (p *NaturalLanguageParserCore) numericDateOrder() NumericDateOrder {
	if p.options.DateOrder != "" {
		return p.options.DateOrder
	}
	locale := strings.ToLower(p.options.DateLocale)
	if locale == "" {
		locale = strings.ToLower(p.languageConfig.Code)
	}
	switch locale {
	case "en-gb", "en-au", "en-nz", "en-ie", "de", "de-de", "fr", "fr-fr", "es", "it", "nl", "pt", "sv", "ru", "uk":
		return DateOrderDayFirst
	default:
		return DateOrderMonthFirst
	}
}

func (p *NaturalLanguageParserCore) now() time.Time {
	if p.options.Now != nil {
		return p.options.Now()
	}
	return time.Now()
}

func (p *NaturalLanguageParserCore) validateAndCleanupResult(result ParsedTaskData) ParsedTaskData {
	if strings.TrimSpace(result.Title) == "" {
		result.Title = "Untitled Task"
	}
	result.Tags = dedupeNonEmpty(result.Tags)
	result.Contexts = dedupeNonEmpty(result.Contexts)
	result.Projects = dedupeNonEmpty(result.Projects)
	if !validDateString(result.DueDate) {
		result.DueDate = ""
	}
	if !validDateString(result.ScheduledDate) {
		result.ScheduledDate = ""
	}
	if !validTimeString(result.DueTime) {
		result.DueTime = ""
	}
	if !validTimeString(result.ScheduledTime) {
		result.ScheduledTime = ""
	}
	return result
}

func (p *NaturalLanguageParserCore) GetPreviewData(parsed ParsedTaskData) []PreviewItem {
	var parts []PreviewItem
	if parsed.Title != "" {
		parts = append(parts, PreviewItem{"edit-3", `"` + parsed.Title + `"`})
	}
	if parsed.Details != "" {
		d := parsed.Details
		if len(d) > 50 {
			d = d[:50] + "..."
		}
		parts = append(parts, PreviewItem{"file-text", `Details: "` + d + `"`})
	}
	if parsed.DueDate != "" {
		value := parsed.DueDate
		if parsed.DueTime != "" {
			value += " at " + parsed.DueTime
		}
		parts = append(parts, PreviewItem{"calendar", "Due: " + value})
	}
	if parsed.ScheduledDate != "" {
		value := parsed.ScheduledDate
		if parsed.ScheduledTime != "" {
			value += " at " + parsed.ScheduledTime
		}
		parts = append(parts, PreviewItem{"calendar-clock", "Scheduled: " + value})
	}
	if parsed.Priority != "" {
		parts = append(parts, PreviewItem{"alert-triangle", "Priority: " + parsed.Priority})
	}
	if parsed.Status != "" {
		parts = append(parts, PreviewItem{"activity", "Status: " + parsed.Status})
	}
	if len(parsed.Contexts) > 0 {
		parts = append(parts, PreviewItem{"map-pin", "Contexts: @" + strings.Join(parsed.Contexts, ", @")})
	}
	if len(parsed.Projects) > 0 {
		parts = append(parts, PreviewItem{"folder", "Projects: +" + strings.Join(parsed.Projects, ", +")})
	}
	if len(parsed.Tags) > 0 {
		parts = append(parts, PreviewItem{"tag", "Tags: #" + strings.Join(parsed.Tags, ", #")})
	}
	if parsed.Recurrence != "" {
		parts = append(parts, PreviewItem{"repeat", "Recurrence: " + recurrenceToText(parsed.Recurrence)})
	}
	if parsed.Estimate > 0 {
		parts = append(parts, PreviewItem{"clock", fmt.Sprintf("Estimate: %d min", parsed.Estimate)})
	}
	for key, value := range parsed.UserFields {
		display := key
		if field, ok := p.triggerConfig.GetUserField(key); ok && field.DisplayName != "" {
			display = field.DisplayName
		}
		parts = append(parts, PreviewItem{"box", display + ": " + displayAny(value)})
	}
	return parts
}

func (p *NaturalLanguageParserCore) GetPreviewText(parsed ParsedTaskData) string {
	items := p.GetPreviewData(parsed)
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = item.Text
	}
	return strings.Join(parts, " • ")
}

func (p *NaturalLanguageParserCore) GetStatusSuggestions(query string, limit int) []map[string]string {
	if limit <= 0 {
		limit = 10
	}
	q := strings.ToLower(query)
	out := []map[string]string{}
	for _, status := range p.statusConfigs {
		if status.Value == "" || status.Label == "" {
			continue
		}
		if strings.Contains(strings.ToLower(status.Value), q) || strings.Contains(strings.ToLower(status.Label), q) {
			out = append(out, map[string]string{"value": status.Value, "label": status.Label, "display": status.Label})
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func cleanupWhitespace(text string) string {
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(text, " "))
}
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func runeBefore(text string, index int) rune {
	if index <= 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return r
}
func runeAt(text string, index int) rune {
	if index >= len(text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return r
}
func isWordRune(r rune) bool {
	return r != 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_')
}
func formatDate(t time.Time) string { return t.Format("2006-01-02") }

func validDate(y int, m time.Month, d int) (time.Time, bool) {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	return t, t.Year() == y && t.Month() == m && t.Day() == d
}

func validDateString(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func validTimeString(value string) bool {
	if value == "" {
		return true
	}
	return regexp.MustCompile(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`).MatchString(value)
}

func dedupeNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, regexp.QuoteMeta(key))
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func recurrenceToText(rrule string) string {
	switch rrule {
	case "FREQ=DAILY":
		return "every day"
	case "FREQ=WEEKLY":
		return "every week"
	case "FREQ=MONTHLY":
		return "every month"
	case "FREQ=YEARLY":
		return "every year"
	default:
		return rrule
	}
}

func displayAny(value any) string {
	switch v := value.(type) {
	case []string:
		return strings.Join(v, ", ")
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
