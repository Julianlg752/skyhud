# Making your own HUD design

There are two ways to make a design your own:

1. **No Fusion needed:** change colors, font, layout and size from the panel, then save it under a new name.
2. **With Fusion:** draw new pieces with Fusion's tools and wire them to SkyHUD's data.

Both end up the same way: a design folder in SkyHUD's designs folder, listed in the panel's **Design** menu.

## 1. Without Fusion

1. In the panel, pick the design to start from under **Design**.
2. Change its look:
   - **Colors**: a color picker for each of the design's colors, and a **Font** (the design's own, Mono or Sans). HUDs already on the timeline update as you pick. **Reset colors** goes back to the design's own.
   - **HUD size** scales everything, and **Show** turns pieces on or off (press **Add HUD to all shots** to see these).
   - To move pieces, open any overlay on the Fusion page, drag the piece Transforms (`Speed`, `Height`, `Path`, …), put the playhead on that overlay and press **Use this shot's layout for all**.
3. That's already enough for your own videos: each design remembers its colors and layout in the project. To keep the look as a design of its own (to use in other projects or share), type a **Name** under **Save as new design** and press **Save**. The panel switches to your new design.

Your design keeps the base design's look with your colors, font and layout. You can fine-tune it by editing its `design.json` in a text editor (see below).

## 2. With Fusion

### Where designs live

| | Designs folder |
|---|---|
| macOS | `~/Library/Application Support/SkyHUD/designs/` |
| Windows | `C:\ProgramData\SkyHUD\designs\` |

Each design is a folder:

```
designs/
  my-race/
    design.json      name, pieces, default places, colors, font
    design.setting   the Fusion tools that draw it
```

The easiest start is a copy of a built-in design. The download zip has one of each in its `designs` folder (`goggles-copy`, `race-copy`, `fighter-jet-copy`, `clean-copy`): put a folder in your designs folder and reopen the panel. Or make a copy with:

```sh
skyhud design save --from race --name "My Race" --dir ~/Library/Application\ Support/SkyHUD/designs
```

(`--from` takes `goggles`, `race`, `fighter`, `clean` or one of your designs; add `--color main=#ffcc00`, `--font sans` as you like.)

### design.json

```json
{
  "name": "My Race",
  "note": "Based on Race",
  "font": "sans",
  "colors": {
    "main": [1, 1, 1],
    "accent": [1, 0.48, 0]
  },
  "pieces": {
    "speed":  {"x": 0.12, "y": 0.15},
    "height": {"x": 0.85, "y": 0.9, "size": 1.2},
    "flip":   {"x": 0.5,  "y": 0.82}
  }
}
```

- **pieces**: which pieces the design has and where each sits by default. `x` and `y` run from 0 to 1 with the origin at the **bottom left** (Fusion's convention); `size` scales the piece (default 1). The pieces are `speed`, `height`, `gforce`, `horizon`, `path`, `sticks`, `battery` and `flip`.
- **colors**: any names you like, as red, green, blue from 0 to 1. The panel's **Main color** and **Accent** fields set `main` and `accent`.
- **font**: `mono`, `sans` or a font name installed on the computer.

### design.setting: the rules

`design.setting` holds Fusion tools in Fusion's own text format, the same text you get when you copy nodes in Fusion's node editor. You can paste a copied block straight in; SkyHUD unwraps the `{ Tools = ordered() { ... } }` around it.

1. **Every tool of a piece starts with the piece's name**: `Speed`, `Height`, `GForce`, `Horizon`, `Path`, `Sticks`, `Battery` or `Flip`. For example `SpeedArc`, `SpeedValue`, `SpeedMerge`.
2. **Each piece has one output tool named `<Piece>Out`** (`SpeedOut`, `PathOut`, …). Draw the piece **around the center of the frame** (0.5, 0.5); SkyHUD adds a Transform named after the piece (`Speed`) that moves it to its place and scales it. That's the Transform you drag on the Fusion page.
3. **Tools starting with `FX`** are effects that aren't placed, such as Race's edge glow. The output tool is `FXOut`, drawn under the pieces.
4. **Don't include** SkyHUD's own tools: `HUDData`, `HUDData2`, `HUDData3`, the piece Transforms (`Speed`, `Height`, …), `HUD`, `SkyHUDMerge*` or `MediaOut1`. Connect to the data tools by name; SkyHUD adds them.
5. **Pieces stay separate:** a tool may connect to (or read in an expression) only tools of its own piece, FX tools only FX tools, and anyone the data tools. A hidden piece is left out of the comp, so links between pieces would break.
6. **Data only:** SkyHUD reads `design.setting` strictly and writes the tools out again from what it read. It accepts names, numbers, double-quoted strings (escapes `\"`, `\\`, `\n`), `true`/`false`, tables, and the constructors `Input`, `Polyline`, `FuID`, `OperatorInfo` and `ordered()`. Tools may have `Inputs`, `DrawMode`, `ViewInfo`, `CtrlWZoom`, `CtrlWShown` and `NameSet`; inputs may have `Value`, `Expression`, `SourceOp` and `Source`. Comments, other string forms, function calls, repeated keys and `UserControls` are refused.
7. **Allowed tool types:** Text+ (`TextPlus`), Background, Ellipse, Rectangle, Polygon and B-Spline masks, Merge, Transform, Brightness Contrast, Color Corrector, Soft Glow, Blur, Channel Booleans and Dissolve.

### The data

SkyHUD writes one keyframe track per value into three data tools. Read them in expressions as `HUDData.NumberIn1` and so on. An "OK" input is 1 where the value is known and 0 where it isn't (no GPS lock, no flight log): show `--` there.

| Tool | Input | Value |
|---|---|---|
| `HUDData` | `NumberIn1` | speed, km/h or mph (see `{{speedunit}}`) |
| | `NumberIn2` | speed OK |
| | `NumberIn3` | height, m or ft (see `{{heightunit}}`), above takeoff or sea level as chosen in the panel |
| | `NumberIn4` | height OK |
| | `NumberIn5` | g-force, g |
| | `NumberIn6` | g-force OK |
| | `NumberIn7` | horizon roll, degrees, counterclockwise positive (use it for a Transform's Angle) |
| | `NumberIn8` | horizon OK |
| `HUDData2` | `NumberIn1` | throttle stick, -1 to 1 |
| | `NumberIn2` | yaw stick, -1 to 1 |
| | `NumberIn3` | pitch stick, -1 to 1 |
| | `NumberIn4` | roll stick, -1 to 1 |
| | `NumberIn5` | sticks OK |
| | `NumberIn6` | battery, % |
| | `NumberIn7` | battery, volts |
| | `NumberIn8` | battery OK |
| `HUDData3` | `NumberIn1` | pitch, degrees, nose up positive |
| | `NumberIn2` | pitch OK |
| | `NumberIn3` | flight path dot x, -0.5 to 0.5 |
| | `NumberIn4` | flight path dot y, -0.5 to 0.5 |
| | `NumberIn5` | flight path progress, 0 to 1 along the flight |
| | `NumberIn6` | flight path OK |
| | `NumberIn7` | 1 while upside down, else 0 |
| | `NumberIn8` | seconds since the last flip started (99 if none yet) |

An earlier value: `HUDData:GetValue("NumberIn1", time - 10)` is the speed 10 frames ago (use this form; `HUDData.NumberIn1[time - 10]` breaks the render).

G-force from a DJI Avata 2 video is estimated from speed changes. SkyHUD fills `{{glabel}}` with `G (EST)` then, and `G` otherwise; use it as your g-force label so estimates stay labelled.

### Placeholders

SkyHUD replaces these in `design.setting` before loading it:

| Placeholder | Becomes |
|---|---|
| `{{font}}` | the design's font (from design.json or the panel) |
| `{{mono}}`, `{{sans}}` | the system's monospace and sans-serif fonts |
| `{{main.r}}`, `{{main.g}}`, `{{main.b}}` | one channel of the color `main` (any color name works) |
| `{{main}}` | `Red = …, Green = …, Blue = …` |
| `{{aspect}}` | the frame's width divided by its height (Fusion's Y is measured in frame heights, so multiply vertical offsets by it to keep circles round) |
| `{{path}}` | the recording's flight path as Polyline points, centered on 0 and spanning -0.5 to 0.5; give the PolylineMask a `Size` to set the box size, and put the dot at `Point(0.5 + HUDData3.NumberIn3 * Size, 0.5 + HUDData3.NumberIn4 * Size * {{aspect}})` |
| `{{glabel}}` | `G` or `G (EST)` |
| `{{speedunit}}`, `{{SPEEDUNIT}}` | `km/h` or `mph` (`KM/H` or `MPH`), from the panel's **Units** |
| `{{heightunit}}`, `{{HEIGHTUNIT}}` | `m` or `ft` (`M` or `FT`) |

Label speed and height with the unit placeholders, never a fixed "km/h" or "m": SkyHUD only switches a design to mph and feet when it uses both, so its numbers and labels always agree. A design with fixed labels stays metric.

### Expressions

For safety, expressions may use only: numbers and strings, `+ - * / % ^`, comparisons, `and`, `or`, `not`, `time`, the data inputs (and `GetValue`), inputs of the design's own tools (`SpeedArc.Size`), `Text(...)`, `Point(...)`, `string.format`, and `math.abs`, `min`, `max`, `floor`, `ceil`, `sin`, `cos`, `tan`, `rad`, `deg`, `pi`, `sqrt`, `fmod`, `huge`. Joining strings with `..` isn't allowed; use `string.format`.

Examples from the built-in designs:

```lua
-- speed text with a fallback
Text(HUDData.NumberIn2 > 0.5 and string.format("SPEED\n%.0f {{speedunit}}", HUDData.NumberIn1) or "SPEED\n-- {{speedunit}}")
-- a speed arc: an EllipseMask's WriteLength grows with speed up to 60 (km/h or mph)
0.5 * math.min(1, HUDData.NumberIn1 / 60)
-- the flip badge shows only while upside down
Text(HUDData3.NumberIn7 > 0.5 and "FLIP!" or "")
-- a stick dot: Center of a small EllipseMask
Point(0.44 + HUDData2.NumberIn2 * 0.032, 0.5 + HUDData2.NumberIn1 * 0.032 * {{aspect}})
```

### Check, preview, use

```sh
skyhud design check ~/Library/Application\ Support/SkyHUD/designs/my-race
skyhud design preview ~/Library/Application\ Support/SkyHUD/designs/my-race -o ~/Desktop/my-race.comp
```

- **check** lists every problem with the tool and input it's in. A design that fails is never loaded; the panel uses Goggles instead and says why.
- **preview** writes a Fusion composition of your design driven by a made-up 30-second flight (`--seconds` to change it). In Resolve, open any Fusion composition on the Fusion page and use **File > Import > Fusion Composition…** (or open it in Fusion Studio) to see it move. Edit it there, copy your tools back into `design.setting`, and check again.

Then pick it under **Design** in the panel (reopen the panel to refresh the list) and press **Add HUD to all shots**.

### Sharing designs

A design is just its folder: zip it and share it. To use someone's design, unzip it into your designs folder and run `skyhud design check` on it first. SkyHUD refuses tools and expressions that could run code, but only use designs from people you trust.

### Privacy: the flight path

The flight path piece is drawn from relative meters, never GPS coordinates, but its shape can still show where you flew (a recognizable field, your street). Turn **Flight path** off in the panel for footage near your home, or leave the piece out of your design.
