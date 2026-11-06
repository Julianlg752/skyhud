package design

import (
	"fmt"
	"strings"
)

// A design.setting is Lua source that Fusion runs when it loads the comp, so
// SkyHUD never passes it on as written. It reads a strict subset that can
// only describe data (names, numbers, double-quoted strings, true/false,
// tables and a few known constructors), then writes the tools out again from
// what it read. Anything else, such as a function call, an indexed key, a
// comment, another string form or a repeated key, is an error.

// Constructors a value may use, besides the tool types.
var constructors = map[string]bool{"Input": true, "Polyline": true, "FuID": true, "OperatorInfo": true, "ordered": true}

// Fields a tool may have (UserControls, for one, can hold buttons that run code).
var toolFields = map[string]bool{"Inputs": true, "DrawMode": true, "ViewInfo": true, "CtrlWZoom": true, "CtrlWShown": true, "NameSet": true}

// Fields an Input may have.
var inputFields = map[string]bool{"Value": true, "Expression": true, "SourceOp": true, "Source": true}

type valueKind int

const (
	numberValue valueKind = iota
	stringValue
	boolValue
	tableValue
)

// value is one parsed value. A table keeps its entries in order; an entry's
// key is "" when it has none.
type value struct {
	kind    valueKind
	text    string // number as written, string contents, "true" or "false"
	ctor    string // constructor of a table, "" for a plain one
	entries []entry
}

type entry struct {
	key string
	val value
}

func (v value) field(key string) (value, bool) {
	for _, e := range v.entries {
		if e.key == key {
			return e.val, true
		}
	}
	return value{}, false
}

type token struct {
	kind byte // 'i' name, 'n' number, 's' string, or the punctuation itself
	text string
	pos  int
}

func lex(src string) ([]token, error) {
	var out []token
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case strings.IndexByte("{}=,()", c) >= 0:
			out = append(out, token{c, string(c), i})
			i++
		case c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			j := i
			for j < len(src) && (src[j] == '_' || src[j] == '.' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			out = append(out, token{'i', src[i:j], i})
			i = j
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			return nil, fmt.Errorf("comments are not allowed (at %s)", where(src, i))
		case c == '-' || c == '.' || c >= '0' && c <= '9':
			j := i
			if c == '-' {
				j++
			}
			start := j
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.' ||
				(src[j] == 'e' || src[j] == 'E') && j > start ||
				(src[j] == '-' || src[j] == '+') && (src[j-1] == 'e' || src[j-1] == 'E')) {
				j++
			}
			if j == start {
				return nil, fmt.Errorf("unexpected %q at %s", string(c), where(src, i))
			}
			out = append(out, token{'n', src[i:j], i})
			i = j
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\n' {
					return nil, fmt.Errorf("unfinished string at %s", where(src, i))
				}
				if src[j] != '\\' {
					b.WriteByte(src[j])
					continue
				}
				j++
				if j >= len(src) {
					break
				}
				switch src[j] {
				case '"', '\\':
					b.WriteByte(src[j])
				case 'n':
					b.WriteByte('\n')
				default:
					return nil, fmt.Errorf("only \\\" \\\\ and \\n escapes are allowed in strings (at %s)", where(src, j-1))
				}
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unfinished string at %s", where(src, i))
			}
			out = append(out, token{'s', b.String(), i})
			i = j + 1
		default:
			return nil, fmt.Errorf("%q is not allowed in a design (at %s)", string(c), where(src, i))
		}
	}
	return out, nil
}

// where names a position as line:column.
func where(src string, i int) string {
	line := strings.Count(src[:i], "\n") + 1
	return fmt.Sprintf("line %d, column %d", line, i-strings.LastIndexByte(src[:i], '\n'))
}

type parser struct {
	src  string
	toks []token
	i    int
}

func (p *parser) peek() token {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return token{kind: 0, pos: len(p.src)}
}

func (p *parser) expect(kind byte) (token, error) {
	t := p.peek()
	if t.kind != kind {
		got := t.text
		if t.kind == 0 {
			got = "the end"
		}
		return t, fmt.Errorf("expected %q, found %q at %s", string(kind), got, where(p.src, t.pos))
	}
	p.i++
	return t, nil
}

// table reads { entry, entry, ... } with the opening brace next.
func (p *parser) table(ctor string) (value, error) {
	if _, err := p.expect('{'); err != nil {
		return value{}, err
	}
	v := value{kind: tableValue, ctor: ctor}
	seen := map[string]bool{}
	for p.peek().kind != '}' {
		key := ""
		if p.peek().kind == 'i' && p.i+1 < len(p.toks) && p.toks[p.i+1].kind == '=' {
			key = p.peek().text
			if strings.Contains(key, ".") {
				return v, fmt.Errorf("key %q is not a plain name (at %s)", key, where(p.src, p.peek().pos))
			}
			if seen[key] {
				return v, fmt.Errorf("%s is set twice (at %s)", key, where(p.src, p.peek().pos))
			}
			seen[key] = true
			p.i += 2
		}
		val, err := p.value()
		if err != nil {
			return v, err
		}
		v.entries = append(v.entries, entry{key, val})
		if p.peek().kind == ',' {
			p.i++
		} else if p.peek().kind != '}' {
			_, err := p.expect('}')
			return v, err
		}
	}
	p.i++
	return v, nil
}

func (p *parser) value() (value, error) {
	t := p.peek()
	switch t.kind {
	case 'n':
		p.i++
		return value{kind: numberValue, text: t.text}, nil
	case 's':
		p.i++
		return value{kind: stringValue, text: t.text}, nil
	case '{':
		return p.table("")
	case 'i':
		if t.text == "true" || t.text == "false" {
			p.i++
			return value{kind: boolValue, text: t.text}, nil
		}
		if !constructors[t.text] {
			return value{}, fmt.Errorf("%q is not allowed in a design (at %s)", t.text, where(p.src, t.pos))
		}
		p.i++
		if t.text == "ordered" {
			if _, err := p.expect('('); err != nil {
				return value{}, err
			}
			if _, err := p.expect(')'); err != nil {
				return value{}, err
			}
		}
		return p.table(t.text)
	}
	got := t.text
	if t.kind == 0 {
		got = "the end"
	}
	return value{}, fmt.Errorf("unexpected %q at %s", got, where(p.src, t.pos))
}

// parsedTool is one tool as read: name, type and its fields.
type parsedTool struct {
	name, typ string
	body      value
}

func parseSetting(src string) ([]parsedTool, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{src: src, toks: toks}
	var out []parsedTool
	seen := map[string]bool{}
	for p.peek().kind != 0 {
		name, err := p.expect('i')
		if err != nil {
			return nil, fmt.Errorf("expected a tool (Name = Type { ... }): %w", err)
		}
		if strings.Contains(name.text, ".") || seen[name.text] {
			return nil, fmt.Errorf("bad or repeated tool name %q at %s", name.text, where(src, name.pos))
		}
		seen[name.text] = true
		if _, err := p.expect('='); err != nil {
			return nil, err
		}
		typ, err := p.expect('i')
		if err != nil {
			return nil, err
		}
		body, err := p.table(typ.text)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", name.text, err)
		}
		out = append(out, parsedTool{name.text, typ.text, body})
		if p.peek().kind == ',' {
			p.i++
		}
	}
	return out, nil
}

// write writes v back as setting text; depth is its indent.
func (v value) write(b *strings.Builder, depth int) {
	switch v.kind {
	case numberValue, boolValue:
		b.WriteString(v.text)
		return
	case stringValue:
		b.WriteString(quote(v.text))
		return
	}
	if v.ctor == "ordered" {
		b.WriteString("ordered() ")
	} else if v.ctor != "" {
		b.WriteString(v.ctor + " ")
	}
	// Small tables (Input, points) stay on one line.
	inline := v.ctor != "" && v.ctor != "ordered" || len(v.entries) <= 4 && !hasTable(v)
	if inline {
		b.WriteString("{ ")
		for _, e := range v.entries {
			if e.key != "" {
				b.WriteString(e.key + " = ")
			}
			e.val.write(b, depth)
			b.WriteString(", ")
		}
		b.WriteString("}")
		return
	}
	b.WriteString("{\n")
	for _, e := range v.entries {
		b.WriteString(strings.Repeat("\t", depth+1))
		if e.key != "" {
			b.WriteString(e.key + " = ")
		}
		e.val.write(b, depth+1)
		b.WriteString(",\n")
	}
	b.WriteString(strings.Repeat("\t", depth) + "}")
}

func hasTable(v value) bool {
	for _, e := range v.entries {
		if e.val.kind == tableValue {
			return true
		}
	}
	return false
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

// toolBody writes a tool's fields as the inside of its braces, indented for
// a comp's Tools list.
func toolBody(v value) string {
	var b strings.Builder
	b.WriteString("\n")
	for _, e := range v.entries {
		b.WriteString("\t\t\t" + e.key + " = ")
		e.val.write(&b, 3)
		b.WriteString(",\n")
	}
	b.WriteString("\t\t")
	return b.String()
}
