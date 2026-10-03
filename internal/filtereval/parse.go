// Package filtereval answers "what does the loot filter do with this item,
// and why": it reads a written .filter file into blocks and tries an item's
// known facts against them top to bottom, as the game does. Conditions the
// copied item text cannot tell (UnidentifiedItemTier, a base's inventory
// size...) evaluate to "unknown", so an answer can be certain or probable.
package filtereval

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

// Action is what a block does to the items it matches.
type Action string

const (
	Show    Action = "Show"
	Hide    Action = "Hide"
	Minimal Action = "Minimal"
)

// Source tells whose rule a block is.
type Source string

const (
	SourceOurs      Source = "ours"      // our generated block ([[DYNAMIC LOOT FILTER]])
	SourceNeverSink Source = "neversink" // the base filter
)

// Cond is one condition line: Key, an optional operator and its values
// ("Rarity Normal Magic", "ItemLevel >= 82", `HasExplicitMod >=1 "Merciless"`).
type Cond struct {
	Key    string   `json:"key"`
	Op     string   `json:"op"`
	Values []string `json:"values"`
	// Count is HasExplicitMod's own comparison ("HasExplicitMod >=2 ...");
	// CountOp is empty when the line gives none (at least one).
	CountOp string `json:"countOp,omitempty"`
	Count   int    `json:"count,omitempty"`
	Line    string `json:"line"`
}

// Block is one Show/Hide/Minimal rule of the filter.
type Block struct {
	Action   Action `json:"action"`
	Continue bool   `json:"continue"`
	Conds    []Cond `json:"conds"`
	// Line is the 1-based line of the block's first line in the file.
	Line   int    `json:"line"`
	Source Source `json:"source"`
	// Section is the nearest heading above the block: our section title or
	// NeverSink's waypoint title ("Jewellery - Endgame Magic&Normal").
	Section string `json:"section"`
	// Type and Tier are NeverSink's "$type->..." and "$tier->..." tags,
	// Style its "!style" tag; empty for our rules.
	Type  string `json:"type,omitempty"`
	Tier  string `json:"tier,omitempty"`
	Style string `json:"style,omitempty"`
	// Text is the block as written, conditions and looks.
	Text string `json:"text"`
}

// actionKeys are the lines that style a block rather than select items.
var actionKeys = map[string]bool{
	"SetFontSize": true, "SetTextColor": true, "SetBorderColor": true, "SetBackgroundColor": true,
	"PlayAlertSound": true, "PlayAlertSoundPositional": true, "CustomAlertSound": true,
	"CustomAlertSoundOptional": true, "PlayEffect": true, "MinimapIcon": true,
	"DisableDropSound": true, "EnableDropSound": true, "DisableDropSoundIfAlertSound": true,
	"EnableDropSoundIfAlertSound": true, "Continue": true,
}

var (
	waypointRE = regexp.MustCompile(`^#\s*!!\s*Waypoint\s+\S+\s*:\s*"([^"]*)"`)
	typeRE     = regexp.MustCompile(`\$type->(\S+)`)
	tierRE     = regexp.MustCompile(`\$tier->(\S+)`)
	styleRE    = regexp.MustCompile(`!(\S+)\s*$`)
	quotedRE   = regexp.MustCompile(`"([^"]*)"|(\S+)`)
	countOpRE  = regexp.MustCompile(`^(==|!=|<=|>=|<|>|=)?(\d+)$`)
)

var operators = map[string]bool{"=": true, "==": true, "!": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true}

// Parse splits a filter file into its blocks. Commented-out rules are
// skipped: the game ignores them too.
func Parse(content string) []Block {
	var out []Block
	var cur *Block
	var text []string
	section := ""
	source := SourceNeverSink
	prevRule := false // previous line was a "#===" rule, so this comment may be a title
	pendingTitle := ""

	flush := func() {
		if cur != nil {
			cur.Text = strings.TrimRight(strings.Join(text, "\n"), "\n")
			out = append(out, *cur)
		}
		cur, text = nil, nil
	}

	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<20), 4<<20)
	n := 0
	for sc.Scan() {
		n++
		raw := strings.TrimRight(sc.Text(), " \t\r")
		line := strings.TrimSpace(raw)

		if strings.HasPrefix(line, "#") {
			switch {
			case strings.Contains(line, "[[END DYNAMIC LOOT FILTER]]"):
				flush()
				source, section = SourceNeverSink, ""
			case strings.Contains(line, "[[DYNAMIC LOOT FILTER]]"):
				flush()
				source, section = SourceOurs, ""
			}
			isRule := strings.HasPrefix(line, "#==") || strings.HasPrefix(line, "#--")
			switch {
			case isRule && pendingTitle != "":
				// "#===" / "# TITLE" / "#===": a section heading.
				if source == SourceOurs {
					section = pendingTitle
				}
				pendingTitle = ""
			case prevRule && !isRule:
				pendingTitle = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			default:
				pendingTitle = ""
			}
			if m := waypointRE.FindStringSubmatch(line); m != nil {
				section = m[1]
			}
			prevRule = isRule
			if cur != nil {
				flush()
			}
			continue
		}
		prevRule, pendingTitle = false, ""

		if line == "" {
			flush()
			continue
		}

		head, comment, _ := strings.Cut(line, "#")
		fields := strings.Fields(head)
		if len(fields) == 0 {
			continue
		}
		key := fields[0]
		if key == "Show" || key == "Hide" || key == "Minimal" {
			flush()
			cur = &Block{Action: Action(key), Line: n, Source: source, Section: section}
			if m := typeRE.FindStringSubmatch(comment); m != nil {
				cur.Type = m[1]
			}
			if m := tierRE.FindStringSubmatch(comment); m != nil {
				cur.Tier = m[1]
			}
			if m := styleRE.FindStringSubmatch(comment); m != nil {
				cur.Style = m[1]
			}
			text = append(text, line)
			continue
		}
		if cur == nil {
			continue
		}
		text = append(text, line)
		if key == "Continue" {
			cur.Continue = true
			continue
		}
		if actionKeys[key] {
			continue
		}
		cur.Conds = append(cur.Conds, parseCond(key, strings.TrimSpace(strings.TrimPrefix(head, key)), line))
	}
	flush()
	return out
}

func parseCond(key, rest, line string) Cond {
	c := Cond{Key: key, Line: line}
	var tokens []string
	for _, m := range quotedRE.FindAllStringSubmatch(rest, -1) {
		if m[1] != "" || strings.HasPrefix(m[0], `"`) {
			tokens = append(tokens, m[1])
		} else {
			tokens = append(tokens, m[2])
		}
	}
	if len(tokens) > 0 && !strings.HasPrefix(rest, `"`) {
		// HasExplicitMod's count: ">=1", "2", "== 2".
		if key == "HasExplicitMod" || key == "HasEnchantment" {
			if m := countOpRE.FindStringSubmatch(tokens[0]); m != nil {
				c.CountOp = m[1]
				c.Count, _ = strconv.Atoi(m[2])
				tokens = tokens[1:]
			} else if operators[tokens[0]] && len(tokens) > 1 {
				if n, err := strconv.Atoi(tokens[1]); err == nil {
					c.CountOp, c.Count = tokens[0], n
					tokens = tokens[2:]
				} else {
					c.Op = tokens[0]
					tokens = tokens[1:]
				}
			}
		} else if operators[tokens[0]] {
			c.Op = tokens[0]
			tokens = tokens[1:]
		}
	}
	c.Values = tokens
	return c
}
