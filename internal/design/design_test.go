package design

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const goodJSON = `{"name": "Test", "note": "for tests", "font": "mono",
  "colors": {"main": [1, 1, 1], "accent": [1, 0.5, 0]},
  "pieces": {"speed": {"x": 0.1, "y": 0.9}, "path": {"x": 0.9, "y": 0.8, "size": 1.5}}}`

const goodSetting = `SpeedOutText = TextPlus {
	Inputs = {
		UseFrameFormatSettings = Input { Value = 1, },
		Font = Input { Value = "{{font}}", },
		Red1 = Input { Value = {{accent.r}}, },
		StyledText = Input { Expression = "Text(HUDData.NumberIn2 > 0.5 and string.format(\"%.0f\", HUDData.NumberIn1) or \"--\")", },
	},
},
SpeedOut = Transform {
	Inputs = {
		Input = Input { SourceOp = "SpeedOutText", Source = "Output", },
		Size = Input { Expression = "1 + math.min(1, HUDData:GetValue(\"NumberIn1\", time - 5) / 100)", },
	},
},
PathLine = PolylineMask {
	Inputs = {
		Polyline = Input { Value = Polyline { Points = { {{path}} }, }, },
		WriteLength = Input { Expression = "HUDData3.NumberIn5", },
	},
},
PathOut = Background {
	Inputs = {
		TopLeftAlpha = Input { Expression = "HUDData3.NumberIn6 * {{aspect}}", },
		EffectMask = Input { SourceOp = "PathLine", Source = "Mask", },
	},
},
`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"designs/test/design.json":    {Data: []byte(goodJSON)},
		"designs/test/design.setting": {Data: []byte(goodSetting)},
	}
}

func TestLoadAndRender(t *testing.T) {
	d, err := Load(testFS(), "designs/test")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "test" || d.Name != "Test" || d.Pieces["path"].Size != 1.5 || d.Pieces["speed"].Size != 1 {
		t.Fatalf("design = %+v", d)
	}
	out, err := d.Render(Vars{Mono: "Menlo", Sans: "Helvetica", Aspect: 4.0 / 3, Path: "{ X = 0, Y = 0 }"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Value = "Menlo"`, `Red1 = Input { Value = 1, }`, `Points = { { X = 0, Y = 0 } }`, `HUDData3.NumberIn6 * 1.333333333`} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %s", want)
		}
	}
	if strings.Contains(out, "{{") {
		t.Fatal("placeholder left")
	}
	over, _ := d.Render(Vars{Mono: "Menlo", Colors: map[string][3]float64{"accent": {0, 1, 0}}})
	if !strings.Contains(over, `Red1 = Input { Value = 0, }`) {
		t.Fatal("color override not applied")
	}
}

func TestFindPrefersUser(t *testing.T) {
	user := t.TempDir()
	d, _ := Load(testFS(), "designs/test")
	d.ID, d.Name = "goggles", "My goggles"
	if err := d.Save(filepath.Join(user, "goggles")); err != nil {
		t.Fatal(err)
	}
	got, err := Find("goggles", user)
	if err != nil || got.Name != "My goggles" || got.Builtin {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := Find("nope", user); err == nil {
		t.Fatal("unknown design found")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	d, _ := Load(testFS(), "designs/test")
	dir := filepath.Join(t.TempDir(), "copy")
	d.Colors["accent"] = [3]float64{0.2, 0.3, 0.4}
	if err := d.Save(dir); err != nil {
		t.Fatal(err)
	}
	back, err := Load(os.DirFS(filepath.Dir(dir)), "copy")
	if err != nil || back.Colors["accent"] != [3]float64{0.2, 0.3, 0.4} || back.Template != d.Template || back.ID != "copy" {
		t.Fatalf("back = %+v, %v", back, err)
	}
}

// Tools copied on Fusion's node editor come wrapped in { Tools = ordered() { ... } }.
func TestLoadPastedFusionClipboard(t *testing.T) {
	pasted := "{\n\tTools = ordered() {\n\t\tSpeedOut = TextPlus {\n\t\t\tCtrlWZoom = false,\n\t\t\tInputs = {\n" +
		"\t\t\t\tStyledText = Input { Expression = \"Text(string.format(\\\"%.0f\\\", HUDData.NumberIn1))\", },\n\t\t\t},\n" +
		"\t\t\tViewInfo = OperatorInfo { Pos = { 220, 49.5 } },\n\t\t}\n\t},\n\tActiveTool = \"SpeedOut\"\n}\n"
	fsys := fstest.MapFS{
		"d/pasted/design.json":    {Data: []byte(`{"name": "Pasted", "pieces": {"speed": {"x": 0.5, "y": 0.5}}}`)},
		"d/pasted/design.setting": {Data: []byte(pasted)},
	}
	d, err := Load(fsys, "d/pasted")
	if err != nil {
		t.Fatal(err)
	}
	if ps := Check(d); len(ps) > 0 {
		t.Fatalf("problems: %v", ps)
	}
	if strings.Contains(d.Template, "ordered()") || !strings.HasPrefix(strings.TrimSpace(d.Template), "SpeedOut = TextPlus") {
		t.Fatalf("template not unwrapped: %q", d.Template)
	}
}

func TestRenderUnits(t *testing.T) {
	d := &Design{ID: "u", Template: `A = Input { Expression = "Text(\"{{speedunit}} {{heightunit}} {{SPEEDUNIT}} {{HEIGHTUNIT}}\")", },`}
	metric, err := d.Render(Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metric, `km/h m KM/H M`) {
		t.Errorf("metric render = %s", metric)
	}
	imperial, err := d.Render(Vars{Imperial: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(imperial, `mph ft MPH FT`) {
		t.Errorf("imperial render = %s", imperial)
	}
	if !d.HasUnits() {
		t.Error("design with unit placeholders not detected")
	}
	if (&Design{Template: `Text(\"KM/H\")`}).HasUnits() {
		t.Error("design with fixed units detected as switchable")
	}
}

func TestBuiltinsUseUnitPlaceholders(t *testing.T) {
	for _, d := range Builtins() {
		if !d.HasUnits() {
			t.Errorf("%s: speed and height units are fixed", d.ID)
		}
		out, err := d.Render(Vars{Imperial: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(out), "km/h") {
			t.Errorf("%s: imperial render still shows km/h", d.ID)
		}
	}
}
