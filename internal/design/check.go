package design

import (
	"fmt"
	"regexp"
	"strings"
)

var dataInput = regexp.MustCompile(`^NumberIn[1-8]$`)

// Problem is one reason a design can't be used.
type Problem struct {
	Tool, Input, Msg string
}

func (p Problem) String() string {
	where := p.Tool
	if p.Input != "" {
		where += "." + p.Input
	}
	if where == "" {
		return p.Msg
	}
	return where + ": " + p.Msg
}

// Tool types a design may use: drawing and compositing only. Fuses, macros,
// Custom tools (per-pixel scripts) and loaders are not allowed.
var allowedTypes = map[string]bool{
	"TextPlus": true, "Background": true, "EllipseMask": true, "RectangleMask": true,
	"PolylineMask": true, "BSplineMask": true, "Merge": true, "Transform": true,
	"BrightnessContrast": true, "ColorCorrector": true, "SoftGlow": true, "Blur": true,
	"ChannelBoolean": true, "Dissolve": true,
}

// Data tools SkyHUD adds; designs read them.
var dataTools = map[string]bool{"HUDData": true, "HUDData2": true, "HUDData3": true}

// Names SkyHUD itself uses in the comp.
var reserved = map[string]bool{"HUD": true, "MediaOut1": true}

var allowedWords = map[string]bool{
	"and": true, "or": true, "not": true, "true": true, "false": true, "nil": true,
	"time": true, "Text": true, "Point": true, "string": true, "math": true,
}

var stringFuncs = map[string]bool{"format": true}

var mathFuncs = map[string]bool{
	"abs": true, "min": true, "max": true, "floor": true, "ceil": true, "sin": true, "cos": true,
	"tan": true, "rad": true, "deg": true, "pi": true, "sqrt": true, "fmod": true, "huge": true,
}

// Check reports everything that keeps a design from being used safely.
func Check(d *Design) []Problem {
	return CheckWith(d, Vars{Mono: "Menlo", Sans: "Helvetica", Aspect: 16.0 / 9, Path: "{ Linear = true, X = 0, Y = 0 }"})
}

// CheckWith checks a design rendered with v. The setting is read by a strict
// parser that only accepts data (see setting.go); then every tool type,
// field, connection and expression is checked against an allowlist.
func CheckWith(d *Design, v Vars) []Problem {
	var ps []Problem
	src, err := d.Render(v)
	if err != nil {
		return []Problem{{Msg: err.Error()}}
	}
	tools, err := parseTools(src)
	if err != nil {
		return []Problem{{Msg: err.Error()}}
	}
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name] = true
	}
	for p := range d.Pieces {
		if !names[ToolName[p]+"Out"] {
			ps = append(ps, Problem{Tool: ToolName[p] + "Out", Msg: fmt.Sprintf("piece %q has no output tool %sOut", p, ToolName[p])})
		}
	}
	for _, t := range tools {
		ps = append(ps, t.problems...)
		if !allowedTypes[t.Type] {
			ps = append(ps, Problem{Tool: t.Name, Msg: fmt.Sprintf("tool type %q is not allowed in designs", t.Type)})
		}
		if reserved[t.Name] || dataTools[t.Name] || isPieceTransform(t.Name) {
			ps = append(ps, Problem{Tool: t.Name, Msg: "this name is used by SkyHUD itself"})
		} else if !ownedName(t.Name) {
			ps = append(ps, Problem{Tool: t.Name, Msg: "tool names must start with a piece name (Speed, Height, GForce, Horizon, Path, Sticks, Battery, Flip) or FX"})
		}
		piece := PieceOf(t.Name)
		for _, in := range t.Inputs {
			if strings.Contains(strings.ToLower(in.Name), "script") {
				ps = append(ps, Problem{Tool: t.Name, Input: in.Name, Msg: "script inputs are not allowed"})
			}
			switch {
			case in.SourceOp == "" || dataTools[in.SourceOp]:
			case !names[in.SourceOp]:
				ps = append(ps, Problem{Tool: t.Name, Input: in.Name, Msg: fmt.Sprintf("connects to %q, which is not in the design", in.SourceOp)})
			case PieceOf(in.SourceOp) != piece:
				ps = append(ps, Problem{Tool: t.Name, Input: in.Name, Msg: fmt.Sprintf("connects to %q, a tool of another piece (each piece can be hidden on its own)", in.SourceOp)})
			}
			if in.Expression != "" {
				if msg := checkExpression(in.Expression, names, piece); msg != "" {
					ps = append(ps, Problem{Tool: t.Name, Input: in.Name, Msg: msg})
				}
			}
		}
	}
	return ps
}

func isPieceTransform(name string) bool {
	for _, n := range ToolName {
		if name == n {
			return true
		}
	}
	return false
}

func ownedName(name string) bool {
	if strings.HasPrefix(name, "FX") {
		return true
	}
	for _, n := range ToolName {
		if strings.HasPrefix(name, n) {
			return true
		}
	}
	return false
}

// PieceOf returns the piece a tool belongs to, or "" for FX tools.
func PieceOf(name string) string {
	best := ""
	for p, n := range ToolName {
		if strings.HasPrefix(name, n) && len(n) > len(ToolName[best]) {
			best = p
		}
	}
	return best
}

type input struct {
	Name, Expression, SourceOp string
}

// Tool is one tool of a design, as read and written back out.
type Tool struct {
	Name, Type string
	Body       string // the fields between the tool's braces, rewritten from what was read
	Inputs     []input

	problems []Problem // fields a design may not have
}

// ParseTools reads rendered design tools (see setting.go).
func ParseTools(src string) ([]Tool, error) { return parseTools(src) }

func parseTools(src string) ([]Tool, error) {
	parsed, err := parseSetting(src)
	if err != nil {
		return nil, err
	}
	var tools []Tool
	for _, pt := range parsed {
		t := Tool{Name: pt.name, Type: pt.typ, Body: toolBody(pt.body)}
		bad := func(in, msg string) { t.problems = append(t.problems, Problem{Tool: t.Name, Input: in, Msg: msg}) }
		for _, e := range pt.body.entries {
			switch {
			case e.key == "":
				bad("", "a tool's fields need names")
			case !toolFields[e.key]:
				bad("", fmt.Sprintf("tool field %s is not allowed in designs", e.key))
			case e.key == "Inputs" && (e.val.kind != tableValue || e.val.ctor != "" && e.val.ctor != "ordered"):
				bad("", "Inputs must be a table")
			case e.key == "Inputs":
				for _, ie := range e.val.entries {
					t.Inputs = append(t.Inputs, readInput(ie, bad))
				}
			}
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func readInput(e entry, bad func(in, msg string)) input {
	in := input{Name: e.key}
	if e.key == "" || e.val.kind != tableValue || e.val.ctor != "Input" {
		bad(e.key, "each input must be Name = Input { ... }")
		return in
	}
	for _, f := range e.val.entries {
		if !inputFields[f.key] {
			bad(e.key, fmt.Sprintf("input field %q is not allowed in designs", f.key))
			continue
		}
		if f.key != "Value" && f.val.kind != stringValue {
			bad(e.key, f.key+" must be a string")
			continue
		}
		switch f.key {
		case "Expression":
			in.Expression = f.val.text
		case "SourceOp":
			in.SourceOp = f.val.text
		}
	}
	return in
}

// matchBrace returns the index of the brace closing the one at open,
// skipping strings.
func matchBrace(s string, open int) (int, error) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unbalanced braces")
}

// checkExpression accepts arithmetic over numbers, strings, the data tools'
// inputs, inputs of the same piece's tools, and a few safe functions; the
// only calls allowed are Text, Point, string.format, math.* and GetValue.
func checkExpression(expr string, tools map[string]bool, piece string) string {
	toks, err := tokenize(expr)
	if err != nil {
		return err.Error()
	}
	at := func(i int) string {
		if i >= 0 && i < len(toks) {
			return toks[i]
		}
		return ""
	}
	for i, t := range toks {
		prev, prev2 := at(i-1), at(i-2)
		switch {
		case t == "(":
			callable := (prev == "Text" || prev == "Point") && prev2 != "." ||
				prev2 == "." && (at(i-3) == "string" || at(i-3) == "math") || prev == "GetValue"
			if !callable && (isIdent(prev) || prev == ")" || prev == `""` || prev == "0") {
				return "only Text, Point, string.format, math functions and GetValue may be called"
			}
		case t == ":":
			if i == 0 || !dataTools[prev] || at(i+1) != "GetValue" {
				return "only HUDData:GetValue(...) may use ':'"
			}
		case !isIdent(t):
			continue
		case prev == ":":
			// GetValue, checked above
		case prev == ".":
			switch {
			case dataTools[prev2]:
				if !dataInput.MatchString(t) {
					return fmt.Sprintf("%s.%s is not a data input", prev2, t)
				}
			case prev2 == "string":
				if !stringFuncs[t] {
					return fmt.Sprintf("string.%s is not allowed", t)
				}
			case prev2 == "math":
				if !mathFuncs[t] {
					return fmt.Sprintf("math.%s is not allowed", t)
				}
			case tools[prev2]:
				// another tool's input, e.g. SpeedArc.Width: a value, never called
				if next := at(i + 1); next == "(" || next == ":" || next == "." {
					return fmt.Sprintf("%s.%s can only be read", prev2, t)
				}
			default:
				return fmt.Sprintf("%q is not allowed after %q", t, prev2)
			}
		case tools[t]:
			if PieceOf(t) != piece {
				return fmt.Sprintf("%q is a tool of another piece (each piece can be hidden on its own)", t)
			}
			if at(i+1) != "." {
				return fmt.Sprintf("use one of %s's inputs, like %s.Size", t, t)
			}
		case allowedWords[t] || dataTools[t]:
		default:
			return fmt.Sprintf("%q is not allowed in expressions", t)
		}
	}
	return ""
}

func isIdent(t string) bool {
	return t != "" && (t[0] == '_' || t[0] >= 'A' && t[0] <= 'Z' || t[0] >= 'a' && t[0] <= 'z')
}

func tokenize(s string) ([]string, error) {
	var out []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == 'e' || s[j] == 'E' ||
				(s[j] == '-' || s[j] == '+') && j > i && (s[j-1] == 'e' || s[j-1] == 'E')) {
				j++
			}
			out = append(out, "0")
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unfinished string")
			}
			out = append(out, `""`)
			i = j + 1
		case isIdent(string(c)):
			j := i
			for j < len(s) && (isIdent(string(s[j])) || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			out = append(out, s[i:j])
			i = j
		default:
			two := ""
			if i+1 < len(s) {
				two = s[i : i+2]
			}
			switch {
			case two == "..":
				return nil, fmt.Errorf("joining strings with '..' is not allowed")
			case two == "==" || two == "~=" || two == "<=" || two == ">=":
				out = append(out, two)
				i += 2
			case strings.ContainsRune("+-*/%^(),.:<>", rune(c)):
				out = append(out, string(c))
				i++
			default:
				return nil, fmt.Errorf("%q is not allowed in expressions", string(c))
			}
		}
	}
	return out, nil
}
