package design

import (
	"strings"
	"testing"
)

func design(t *testing.T, setting string) *Design {
	t.Helper()
	d, err := Load(testFS(), "designs/test")
	if err != nil {
		t.Fatal(err)
	}
	d.Template = setting
	return d
}

func problems(ps []Problem) string {
	var s []string
	for _, p := range ps {
		s = append(s, p.Tool+"/"+p.Input+": "+p.Msg)
	}
	return strings.Join(s, " | ")
}

func TestCheckAcceptsGoodDesign(t *testing.T) {
	if ps := Check(design(t, goodSetting)); len(ps) != 0 {
		t.Fatalf("problems: %s", problems(ps))
	}
}

func TestCheckRefusesCode(t *testing.T) {
	for _, expr := range []string{
		`io.open(\"x\")`,
		`os.execute(\"rm\")`,
		`require(\"socket\")`,
		`(\"o\"..\"s\")`,
		`_G`,
		`HUDData:SetInput(\"NumberIn1\", 3)`,
		`load(\"return 1\")()`,
		`HUDData[\"NumberIn1\"]`,
		`string.rep(\"a\", 99)`,
		`function() end`,
	} {
		s := strings.Replace(goodSetting, `Expression = "HUDData3.NumberIn5"`, `Expression = "`+expr+`"`, 1)
		if ps := Check(design(t, s)); len(ps) == 0 {
			t.Errorf("accepted %s", expr)
		}
	}
	script := strings.Replace(goodSetting, `WriteLength = Input {`, `FrameRenderScript = Input { Value = "print(1)", },
		WriteLength = Input {`, 1)
	if ps := Check(design(t, script)); len(ps) == 0 {
		t.Error("accepted a FrameRenderScript input")
	}
	fuse := goodSetting + "\nSpeedFuse = Fuse.Evil {\n\tInputs = {\n\t},\n},\n"
	if ps := Check(design(t, fuse)); len(ps) == 0 {
		t.Error("accepted a Fuse tool")
	}
}

func TestCheckMissingPieceOutput(t *testing.T) {
	s := strings.Replace(goodSetting, "PathOut = Background", "PathEnd = Background", 1)
	ps := Check(design(t, s))
	if !strings.Contains(problems(ps), "PathOut") {
		t.Fatalf("problems: %s", problems(ps))
	}
}

func TestCheckBrokenSourceOp(t *testing.T) {
	s := strings.Replace(goodSetting, `SourceOp = "PathLine"`, `SourceOp = "Nowhere"`, 1)
	ps := Check(design(t, s))
	if !strings.Contains(problems(ps), "Nowhere") {
		t.Fatalf("problems: %s", problems(ps))
	}
}

func TestCheckUnbalanced(t *testing.T) {
	if ps := Check(design(t, goodSetting+"Broken = TextPlus {\n")); len(ps) == 0 {
		t.Fatal("accepted unbalanced braces")
	}
}

// Ways around a text-scanning check, found in review. Each must be refused.
func TestCheckRefusesBypasses(t *testing.T) {
	speed := `SpeedOut = TextPlus {
	Inputs = {
		StyledText = Input { Expression = "Text(\"x\")", },
	},
},
`
	cases := map[string]string{
		"call in a value":        strings.Replace(speed, `Expression = "Text(\"x\")"`, `Value = os["execute"]("touch /tmp/pwn")`, 1),
		"call in a nested value": strings.Replace(speed, `Expression = "Text(\"x\")"`, `Value = { X = math.max(1, 2) }`, 1),
		"bracketed input name":   strings.Replace(speed, `StyledText = Input`, `["StyledText"] = Input`, 1),
		"bracketed script input": strings.Replace(speed, `StyledText = Input { Expression = "Text(\"x\")", }`, `["FrameRenderScript"] = Input { Value = "print(1)", }`, 1),
		"duplicate key":          strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = "Text(\"x\")", Expression = "os.exit()"`, 1),
		"single-quoted string":   strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = 'os.exit()'`, 1),
		"long string":            strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = [[os.exit()]]`, 1),
		"decimal escape":         strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = "Text(\"a\034 .. os['execute']('x') .. \034b\")"`, 1),
		"user controls":          strings.Replace(speed, "\tInputs = {", "\tUserControls = ordered() { Go = { BTNCS_Execute = \"os.exit()\", }, },\n\tInputs = {", 1),
		"unknown constructor":    strings.Replace(speed, `Expression = "Text(\"x\")"`, `Value = Evil { 1 }`, 1),
		"tool method call":       strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = "SpeedOut.SetInput(\"Size\", 2)"`, 1),
		"calling a data tool":    strings.Replace(speed, `Expression = "Text(\"x\")"`, `Expression = "HUDData.NumberIn1(1)"`, 1),
		"comment":                speed + "-- hidden\n",
	}
	for name, s := range cases {
		d := design(t, s)
		d.Pieces = map[string]PieceDef{"speed": {X: 0.5, Y: 0.5, Size: 1}}
		if ps := Check(d); len(ps) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Plain words in text are not code.
func TestCheckAllowsWordsInText(t *testing.T) {
	s := strings.Replace(goodSetting, `Font = Input { Value = "{{font}}", },`, `Font = Input { Value = "{{font}}", },
		Text = Input { Value = "debug package os.time io", },`, 1)
	if ps := Check(design(t, s)); len(ps) != 0 {
		t.Fatalf("problems: %s", problems(ps))
	}
}

// A hidden piece is left out of the comp, so pieces can't use each other.
func TestCheckRefusesLinksBetweenPieces(t *testing.T) {
	src := strings.Replace(goodSetting, `TopLeftAlpha = Input { Expression = "HUDData3.NumberIn6 * {{aspect}}", },`,
		`TopLeftAlpha = Input { Expression = "SpeedOutText.Size", },`, 1)
	if ps := Check(design(t, src)); !strings.Contains(problems(ps), "SpeedOutText") {
		t.Errorf("expression link: %s", problems(ps))
	}
	src = strings.Replace(goodSetting, `EffectMask = Input { SourceOp = "PathLine"`, `EffectMask = Input { SourceOp = "SpeedOut"`, 1)
	if ps := Check(design(t, src)); !strings.Contains(problems(ps), "SpeedOut") {
		t.Errorf("connection: %s", problems(ps))
	}
}

// What the comp gets is rewritten from the parsed tools, not copied text.
func TestParseToolsRewritesBody(t *testing.T) {
	tools, err := ParseTools("SpeedOut = TextPlus {\n  Inputs = {   Size = Input {Value=0.5},\n StyledText = Input { Expression = \"Text(\\\"a\\nb\\\")\" } } }")
	if err != nil {
		t.Fatal(err)
	}
	want := "\n\t\t\tInputs = {\n\t\t\t\tSize = Input { Value = 0.5, },\n\t\t\t\tStyledText = Input { Expression = \"Text(\\\"a\\nb\\\")\", },\n\t\t\t},\n\t\t"
	if len(tools) != 1 || tools[0].Body != want {
		t.Fatalf("body %q\nwant %q", tools[0].Body, want)
	}
}
