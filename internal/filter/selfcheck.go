package filter

import (
	"regexp"
	"sort"
	"strings"
)

// The game refuses a whole filter when one Class or BaseType value matches
// nothing it knows (seen with `BaseType == "Catalyst"`, v2.8.1). A patch can
// remove or rename a class or base that a rule of ours names (built-in names
// such as "Stackable Currency" or "Divine Orb", default lists), and the player
// would be left without a loot filter. SelfCheck runs on the generated block
// before it is written: every value must be one the game knows, judged by the
// base filter's own Class names and the known base types.

// Below these sizes the knowledge itself looks broken (an unreadable base
// filter, a failed download); checking against it would strip good rules, so
// that kind is not checked at all.
const (
	selfCheckMinClasses = 10
	selfCheckMinBases   = 500
)

var (
	ruleStart   = regexp.MustCompile(`^(Show|Hide|Minimal)\b`)
	nameCond    = regexp.MustCompile(`^(\s+)(Class|BaseType)\b\s*(==|!=|!|=)?\s*(.*)$`)
	quotedValue = regexp.MustCompile(`"([^"]*)"`)
)

// SelfCheckResult says what SelfCheck took out.
type SelfCheckResult struct {
	Unknown []string // values no item has, sorted
	Dropped int      // rules left out because a condition kept no value
}

// SelfCheck returns block without the Class and BaseType values the game does
// not know. classes are the known class names (lowercase), bases the known
// base types (lowercase -> canonical), loose the base filter's own loose
// BaseType values (lowercase). An exact list (==) keeps a value only when it
// names a class or base; a loose list keeps it when it is part of one, or of
// one of the base filter's loose values.
// A rule whose name condition keeps no value is left out entirely: dropping
// the condition alone would widen the rule (a Hide could hide everything). A
// negated list that keeps no value is dropped as a condition, which is what
// it meant.
func SelfCheck(block string, classes map[string]bool, bases map[string]string, loose []string) (string, SelfCheckResult) {
	checkClasses := len(classes) >= selfCheckMinClasses
	checkBases := len(bases) >= selfCheckMinBases
	if !checkClasses && !checkBases {
		return block, SelfCheckResult{}
	}
	var classList, baseList []string
	for c := range classes {
		classList = append(classList, c)
	}
	for b := range bases {
		baseList = append(baseList, b)
	}
	baseList = append(baseList, loose...)
	known := func(kind, op, value string) bool {
		v := strings.ToLower(value)
		switch {
		case kind == "Class" && !checkClasses, kind == "BaseType" && !checkBases:
			return true
		case kind == "Class" && op == "==":
			return classes[v]
		case kind == "BaseType" && op == "==":
			_, ok := bases[v]
			return ok
		}
		list := classList
		if kind == "BaseType" {
			list = baseList
		}
		for _, name := range list {
			if strings.Contains(name, v) {
				return true
			}
		}
		return false
	}

	unknown := map[string]bool{}
	var res SelfCheckResult
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if !ruleStart.MatchString(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}
		// A rule: its header and the indented lines under it.
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" &&
			(lines[end][0] == ' ' || lines[end][0] == '\t') {
			end++
		}
		rule := []string{lines[i]}
		keep := true
		for _, line := range lines[i+1 : end] {
			m := nameCond.FindStringSubmatch(line)
			values := quotedValue.FindAllStringSubmatch(line, -1)
			if m == nil || len(values) == 0 {
				rule = append(rule, line)
				continue
			}
			indent, kind, op := m[1], m[2], m[3]
			var kept []string
			for _, v := range values {
				if known(kind, op, v[1]) {
					kept = append(kept, `"`+v[1]+`"`)
				} else {
					unknown[kind+" "+v[1]] = true
				}
			}
			switch {
			case len(kept) == len(values):
				rule = append(rule, line)
			case len(kept) > 0:
				cond := indent + kind + " "
				if op != "" {
					cond += op + " "
				}
				rule = append(rule, cond+strings.Join(kept, " "))
			case op == "!" || op == "!=":
				// "not any of nothing" holds for every item
			default:
				keep = false
			}
		}
		if keep {
			out = append(out, rule...)
		} else {
			res.Dropped++
		}
		i = end
	}
	for v := range unknown {
		res.Unknown = append(res.Unknown, v)
	}
	sort.Strings(res.Unknown)
	if len(res.Unknown) == 0 {
		return block, res
	}
	return strings.Join(out, "\n"), res
}
