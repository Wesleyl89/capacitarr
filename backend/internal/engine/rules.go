package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/integrations"
)

// applyRules checks if a media item meets any protection/target rules and applies score modifiers.
// Returns (isAbsolutelyProtected, scoreModifier, reasonString, ruleFactors)
func applyRules(item integrations.MediaItem, rules []db.ProtectionRule) (bool, float64, string, []ScoreFactor) {
	var reasons []string
	var ruleFactors []ScoreFactor
	modifier := 1.0

	for _, rule := range rules {
		if matchesRule(item, rule) {
			ruleName := fmt.Sprintf("%s %s %s", rule.Field, rule.Operator, rule.Value)
			if rule.Type == "protect" {
				if rule.Intensity == "absolute" {
					factor := ScoreFactor{
						Name:         fmt.Sprintf("Protected: %s", ruleName),
						RawScore:     0.0,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					}
					return true, 0.0, fmt.Sprintf("Protected absolutely by rule: %s", ruleName), []ScoreFactor{factor}
				} else if rule.Intensity == "strong" {
					modifier *= 0.2
					reasons = append(reasons, fmt.Sprintf("Strongly protected (%s %s)", rule.Field, rule.Value))
					ruleFactors = append(ruleFactors, ScoreFactor{
						Name:         fmt.Sprintf("Strong Protect: %s", ruleName),
						RawScore:     0.2,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					})
				} else {
					modifier *= 0.5
					reasons = append(reasons, fmt.Sprintf("Slightly protected (%s %s)", rule.Field, rule.Value))
					ruleFactors = append(ruleFactors, ScoreFactor{
						Name:         fmt.Sprintf("Slight Protect: %s", ruleName),
						RawScore:     0.5,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					})
				}
			} else if rule.Type == "target" {
				if rule.Intensity == "absolute" {
					modifier *= 100.0 // Ensure it hits the ceiling
					reasons = append(reasons, fmt.Sprintf("Absolutely targeted (%s %s)", rule.Field, rule.Value))
					ruleFactors = append(ruleFactors, ScoreFactor{
						Name:         fmt.Sprintf("Absolute Target: %s", ruleName),
						RawScore:     1.0,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					})
				} else if rule.Intensity == "strong" {
					modifier *= 2.0
					reasons = append(reasons, fmt.Sprintf("Strongly targeted (%s %s)", rule.Field, rule.Value))
					ruleFactors = append(ruleFactors, ScoreFactor{
						Name:         fmt.Sprintf("Strong Target: %s", ruleName),
						RawScore:     1.0,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					})
				} else {
					modifier *= 1.2
					reasons = append(reasons, fmt.Sprintf("Slightly targeted (%s %s)", rule.Field, rule.Value))
					ruleFactors = append(ruleFactors, ScoreFactor{
						Name:         fmt.Sprintf("Slight Target: %s", ruleName),
						RawScore:     1.0,
						Weight:       0,
						Contribution: 0.0,
						Type:         "rule",
					})
				}
			}
		}
	}
	return false, modifier, strings.Join(reasons, ", "), ruleFactors
}

func matchesRule(item integrations.MediaItem, rule db.ProtectionRule) bool {
	prop := strings.ToLower(rule.Field)
	cond := strings.ToLower(rule.Operator)
	val := strings.ToLower(rule.Value)

	switch prop {
	case "title":
		return stringMatch(strings.ToLower(item.Title), cond, val)
	case "quality":
		return stringMatch(strings.ToLower(item.QualityProfile), cond, val)
	case "availability":
		// Match against status (e.g., Ended, Continuing)
		return stringMatch(strings.ToLower(item.ShowStatus), cond, val)
	case "tag":
		for _, tag := range item.Tags {
			if stringMatch(strings.ToLower(tag), cond, val) {
				return true
			}
		}
		return false
	case "genre":
		return stringMatch(strings.ToLower(item.Genre), cond, val)
	case "rating":
		// condition should be <, >, ==
		ruleNum, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return numberMatch(item.Rating, cond, ruleNum)
	case "sizebytes":
		ruleNum, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return numberMatch(float64(item.SizeBytes), cond, ruleNum)
	case "timeinlibrary":
		if item.AddedAt == nil || item.AddedAt.IsZero() {
			return false
		}
		ruleNum, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		days := time.Since(*item.AddedAt).Hours() / 24.0
		return numberMatch(days, cond, ruleNum)
	case "seasoncount":
		// Compare against the item's SeasonNumber (number of seasons for a show)
		ruleNum, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return numberMatch(float64(item.SeasonNumber), cond, ruleNum)
	case "episodecount":
		// Compare against the item's EpisodeCount
		ruleNum, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return numberMatch(float64(item.EpisodeCount), cond, ruleNum)
	case "monitored":
		// Boolean match: item.Monitored == (val == "true")
		expected := val == "true"
		return item.Monitored == expected
	}

	return false
}

func stringMatch(actual, cond, expected string) bool {
	switch cond {
	case "==":
		return actual == expected
	case "!=":
		return actual != expected
	case "contains":
		return strings.Contains(actual, expected)
	}
	return false
}

func numberMatch(actual float64, cond string, expected float64) bool {
	switch cond {
	case "==":
		return actual == expected
	case "!=":
		return actual != expected
	case ">":
		return actual > expected
	case ">=":
		return actual >= expected
	case "<":
		return actual < expected
	case "<=":
		return actual <= expected
	}
	return false
}
