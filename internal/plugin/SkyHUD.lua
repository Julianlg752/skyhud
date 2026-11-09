local SKYHUD_BIN = [==[skyhud]==]
-- SkyHUD for DaVinci Resolve: adds a data-driven HUD overlay above every
-- shot of the current timeline. `skyhud install` writes this file into
-- Resolve's Scripts folder and sets the line above to the skyhud binary.
--
-- How it works:
--   1. export the timeline as OTIO, which holds each shot's speed ramps;
--   2. `skyhud hud --jobs` writes one Fusion comp per shot;
--   3. each comp is loaded onto a long, empty "base" timeline clip placed on
--      the "SkyHUD" track above the shot, so the grade below never touches it.

SkyHUD = {}

local resolve = Resolve()
local IS_WIN = package.config:sub(1, 1) == "\\"
local SEP = IS_WIN and "\\" or "/"
local TRACK_NAME = "SkyHUD"
local BIN_NAME = "SkyHUD"
local PIECES = { "speed", "height", "gforce", "horizon", "path", "sticks", "battery", "flip" }
local TOOLS = { speed = "Speed", height = "Height", gforce = "GForce", horizon = "Horizon", path = "Path", sticks = "Sticks", battery = "Battery", flip = "Flip" }
local LABELS = { speed = "Speed", height = "Height", gforce = "G-force", horizon = "Horizon", path = "Flight path", sticks = "Stick positions", battery = "Battery", flip = "Flip badge" }
local FLIP_MARKER = "skyhud-flip"
local COLOR_SLOTS = 4 -- color pickers in the panel
local COLOR_LABELS = { main = "Main", accent = "Accent", accent2 = "Second accent", outline = "Outline" }

-- A transparent 16x16 PNG: the base timeline's only clip.
local BLANK_PNG = "\137\80\78\71\13\10\26\10\0\0\0\13\73\72\68\82\0\0\0\16\0\0\0\16\8\6\0\0\0\31\243\255\97\0\0\0\18\73\68\65\84\120\156\99\96\24\5\163\96\20\140\2\8\0\0\4\16\0\1\85\55\90\208\0\0\0\0\73\69\78\68\174\66\96\130"

---------------------------------------------------------------------------
-- Files and settings

local function workDir()
	local base
	if IS_WIN then
		-- ProgramData, not APPDATA: Lua sees paths in the ANSI code page, so a
		-- user name with accents would break the paths sent to skyhud.
		base = (os.getenv("PROGRAMDATA") or os.getenv("APPDATA") or ".") .. "\\SkyHUD"
	else
		base = (os.getenv("HOME") or ".") .. "/Library/Application Support/SkyHUD"
	end
	bmd.createdir(base)
	return base
end

local function safeName(s)
	return (tostring(s):gsub("[^%w%-_ ]", "_"))
end

local function settingsPath(project)
	local dir = workDir() .. SEP .. "projects"
	bmd.createdir(dir)
	return dir .. SEP .. safeName(project:GetName()) .. ".lua"
end

local function defaults()
	return {
		show = { speed = true, height = true, gforce = true, horizon = true, path = true, sticks = true, battery = true, flip = true },
		logs = "",
		height = "start",
		units = "metric",
		design = "goggles",
		scale = 1,
		layouts = {}, -- per design: piece -> { x, y, size } of its placing Transform
		colors = {}, -- per design: color name -> { r, g, b } picked in the panel
		fonts = {}, -- per design: "mono" or "sans" picked in the panel
		layoutVersion = 2,
		track = 1,
		nudge = {},
	}
end

function SkyHUD.loadSettings(project)
	local s = defaults()
	local saved = bmd.readfile(settingsPath(project))
	if type(saved) == "table" then
		for k, v in pairs(saved) do
			s[k] = v
		end
		if not saved.layoutVersion then -- older layouts measured Text+ sizes
			s.layout, s.layouts, s.layoutVersion = nil, {}, 2
		end
	end
	for _, p in ipairs(PIECES) do
		if s.show[p] == nil then s.show[p] = true end
	end
	return s
end

local function designsDir()
	local dir = workDir() .. SEP .. "designs"
	bmd.createdir(dir)
	return dir
end

function SkyHUD.saveSettings(project, s)
	bmd.writefile(settingsPath(project), s)
end

---------------------------------------------------------------------------
-- JSON for the jobs file

local function isArray(t)
	return type(t) == "table" and #t > 0
end

local function encode(v)
	local t = type(v)
	if t == "string" then
		return '"' .. v:gsub('[%c"\\]', function(c)
			if c == '"' then return '\\"' end
			if c == "\\" then return "\\\\" end
			return string.format("\\u%04x", c:byte())
		end) .. '"'
	elseif t == "number" then
		if v ~= v or v == math.huge or v == -math.huge then return "0" end
		return string.format("%.17g", v)
	elseif t == "boolean" then
		return tostring(v)
	elseif isArray(v) then
		local out = {}
		for _, x in ipairs(v) do out[#out + 1] = encode(x) end
		return "[" .. table.concat(out, ",") .. "]"
	elseif t == "table" then
		local out = {}
		for k, x in pairs(v) do out[#out + 1] = encode(tostring(k)) .. ":" .. encode(x) end
		return "{" .. table.concat(out, ",") .. "}"
	end
	return "null"
end

local function writeFile(path, data, mode)
	local f = io.open(path, mode or "w")
	if not f then return false end
	f:write(data)
	f:close()
	return true
end

---------------------------------------------------------------------------
-- Running skyhud

local function quote(s)
	-- Windows programs read \" as a quote, so trailing backslashes (a folder
	-- like D:\Clips\) are doubled to end the argument where it should.
	if IS_WIN then return '"' .. (s:gsub("(\\+)$", "%1%1")) .. '"' end
	return "'" .. (s:gsub("'", "'\\''")) .. "'"
end

local function run(args)
	local cmd = quote(SKYHUD_BIN)
	for _, a in ipairs(args) do cmd = cmd .. " " .. quote(a) end
	if IS_WIN then cmd = '"' .. cmd .. '"' end -- cmd.exe strips one outer pair
	local p = io.popen(cmd .. " 2>&1")
	if not p then return nil, "could not run skyhud" end
	local out = p:read("*a")
	p:close()
	return out
end

function SkyHUD.inspect(path)
	return run({ "inspect", path }) or "could not run skyhud"
end

-- keyStatus says whether a DJI key is stored (never the key itself).
function SkyHUD.keyStatus()
	return ((run({ "key", "status" }) or ""):gsub("%s+$", ""))
end

-- saveKey hands the key to skyhud on standard input, so it never appears in
-- a command line, and skyhud puts it in the system keychain.
function SkyHUD.saveKey(key)
	local cmd = quote(SKYHUD_BIN) .. " key set"
	if IS_WIN then cmd = '"' .. cmd .. '"' end
	local p = io.popen(cmd, "w")
	if not p then return "could not run skyhud" end
	p:write(key, "\n")
	p:close()
	return SkyHUD.keyStatus()
end

---------------------------------------------------------------------------
-- Timeline helpers

local EXACT_RATES = { ["23.976"] = 24000 / 1001, ["29.97"] = 30000 / 1001, ["47.952"] = 48000 / 1001, ["59.94"] = 60000 / 1001, ["119.88"] = 120000 / 1001 }

local function rate(s)
	s = tostring(s or "")
	local n = s:match("[%d%.]+")
	return EXACT_RATES[n] or tonumber(n)
end

-- Lists from the API carry an extra "__flags" entry, so walk them with ipairs.
local function list(t)
	local out = {}
	for _, v in ipairs(t or {}) do out[#out + 1] = v end
	return out
end

local function byStart(items)
	table.sort(items, function(a, b) return a:GetStart() < b:GetStart() end)
	return items
end

local function overlaps(a0, a1, b0, b1)
	return a0 < b1 and b0 < a1
end

local function context()
	local project = resolve:GetProjectManager():GetCurrentProject()
	local tl = project and project:GetCurrentTimeline()
	if not tl then return nil, "Open a timeline first." end
	return { project = project, tl = tl, mp = project:GetMediaPool() }
end

local function overlayTrack(tl, create)
	for i = 1, tl:GetTrackCount("video") do
		if tl:GetTrackName("video", i) == TRACK_NAME then return i end
	end
	if not create then return nil end
	tl:AddTrack("video")
	local n = tl:GetTrackCount("video")
	tl:SetTrackName("video", n, TRACK_NAME)
	return n
end

local function filePath(item)
	local mpi = item:GetMediaPoolItem()
	local p = mpi and mpi:GetClipProperty("File Path")
	if p and p ~= "" then return p, mpi end
	return nil
end

-- tcFrames counts the frames of a timecode; ";" marks drop-frame.
local function tcFrames(tc, fps)
	local h, m, sec, sep, f = tostring(tc or ""):match("^(%d+):(%d+):(%d+)([:;%.,])(%d+)$")
	if not h then return nil end
	h, m, sec, f = tonumber(h), tonumber(m), tonumber(sec), tonumber(f)
	local nominal = math.floor(fps + 0.5)
	local n = ((h * 60 + m) * 60 + sec) * nominal + f
	if sep == ";" or sep == "," then
		local minutes = h * 60 + m
		n = n - math.floor(nominal / 15 + 0.5) * (minutes - math.floor(minutes / 10))
	end
	return n
end

-- playhead returns the timeline frame under the playhead.
local function playhead(tl)
	local fps = rate(tl:GetSetting("timelineFrameRate"))
	local now, start = tcFrames(tl:GetCurrentTimecode(), fps), tcFrames(tl:GetStartTimecode(), fps)
	if now and start then return tl:GetStartFrame() + now - start end
	local cur = tl:GetCurrentVideoItem()
	return cur and cur:GetStart()
end

local function itemAt(items, frame)
	for _, it in ipairs(items) do
		if frame and it:GetStart() <= frame and frame < it:GetEnd() then return it end
	end
end

-- The shot items on the chosen track; "current" keeps the one under the playhead.
local function shotItems(c, track, scope)
	local items = byStart(list(c.tl:GetItemListInTrack("video", track)))
	if scope ~= "current" then return items end
	local it = itemAt(items, playhead(c.tl))
	return it and { it } or {}
end

local function findFolder(folder, name)
	for _, f in ipairs(folder:GetSubFolderList() or {}) do
		if f:GetName() == name then return f end
	end
end

local function findClip(folder, name)
	for _, clip in ipairs(folder:GetClipList() or {}) do
		if clip:GetName() == name then return clip end
	end
end

-- The media pool clip of the long, empty timeline that carries each HUD.
local function baseClip(c, fpsText, fps)
	local name = "SkyHUD base " .. fpsText
	local root = c.mp:GetRootFolder()
	local bin = findFolder(root, BIN_NAME) or c.mp:AddSubFolder(root, BIN_NAME)
	local clip = findClip(bin, name)
	if clip then return clip end

	local before = c.mp:GetCurrentFolder()
	c.mp:SetCurrentFolder(bin)
	local png = findClip(bin, "skyhud_blank.png")
	if not png then
		local path = workDir() .. SEP .. "skyhud_blank.png"
		writeFile(path, BLANK_PNG, "wb")
		png = (c.mp:ImportMedia({ path }) or {})[1]
	end
	local base = c.mp:CreateEmptyTimeline(name)
	if base and png then
		base:SetSetting("useCustomSettings", "1")
		base:SetSetting("timelineFrameRate", tostring(fpsText))
		local got = rate(base:GetSetting("timelineFrameRate"))
		if not got or math.abs(got - fps) > 0.001 then
			c.mp:DeleteTimelines({ base })
			if before then c.mp:SetCurrentFolder(before) end
			return nil, "Could not make a " .. tostring(fpsText) .. " fps base timeline for the HUD (it came out at " .. tostring(got) .. ")."
		end
		c.project:SetCurrentTimeline(base)
		c.mp:AppendToTimeline({ { mediaPoolItem = png, startFrame = 0, endFrame = 149, trackIndex = 1,
			recordFrame = base:GetStartFrame() + math.floor(fps * 3 * 3600) } })
		c.project:SetCurrentTimeline(c.tl)
	end
	if before then c.mp:SetCurrentFolder(before) end
	return findClip(bin, name)
end

---------------------------------------------------------------------------
-- Commands

-- fill builds the HUD for every shot ("all") or the shot under the playhead
-- ("current").
function SkyHUD.fill(scope)
	scope = scope or "all"
	local c, err = context()
	if not c then return { ok = 0, skipped = {}, message = err } end
	local s = SkyHUD.loadSettings(c.project)
	local fpsText = c.tl:GetSetting("timelineFrameRate")
	local fps = rate(fpsText)
	local track = tonumber(s.track) or 1
	local work = workDir()
	local result = { ok = 0, skipped = {}, notes = {} }

	local ov = overlayTrack(c.tl, false)
	if ov and ov <= track then
		return { ok = 0, skipped = {}, message = "The SkyHUD track must be above the shots' track (V" .. track .. ")." }
	end

	local shots, jobs = {}, {}
	for i, it in ipairs(shotItems(c, track, scope)) do
		local path, mpi = filePath(it)
		if not path then
			result.skipped[#result.skipped + 1] = it:GetName() .. ": not a file clip (compound clip, title or generator)"
		else
			local clipFps = rate(mpi:GetClipProperty("FPS")) or fps
			local sp = it:GetSpeed()
			local id = "s" .. i
			shots[id] = it
			jobs[#jobs + 1] = {
				id = id, file = path, track = track,
				recordStart = it:GetStart(), frames = it:GetDuration(),
				speed = (type(sp) == "table" and sp.Percentage) or 100,
				sourceStart = (it:GetSourceStartFrame() or 0) / clipFps,
				nudge = tonumber(s.nudge[path:match("[^/\\]+$")] or 0) or 0,
				out = work .. SEP .. "comps" .. SEP .. id .. ".comp",
			}
		end
	end
	if #jobs == 0 then
		result.message = "No clips with a file on video track " .. track .. "."
		return result
	end

	local otio = work .. SEP .. "timeline.otio"
	os.remove(otio)
	if not c.tl:Export(otio, resolve.EXPORT_OTIO, resolve.EXPORT_NONE) then otio = "" end
	local jobsPath = work .. SEP .. "jobs.json"
	local w = tonumber(c.tl:GetSetting("timelineResolutionWidth")) or 16
	local h = tonumber(c.tl:GetSetting("timelineResolutionHeight")) or 9
	os.remove(jobsPath) -- never let skyhud read an older jobs file
	if not writeFile(jobsPath, encode({ version = 1, fps = fps, aspect = w / h, otio = otio, shots = jobs,
		settings = { show = s.show, height = s.height, units = s.units, logs = s.logs or "", design = s.design, designs = designsDir(),
			scale = s.scale, layouts = s.layouts, layoutVersion = 2, designColors = s.colors, designFonts = s.fonts } })) then
		return { ok = 0, skipped = {}, message = "Could not write " .. jobsPath }
	end
	local out = run({ "hud", "--jobs", jobsPath }) or ""

	local base, baseErr = baseClip(c, fpsText, fps)
	if not base then return { ok = 0, skipped = {}, message = baseErr or "Could not create the SkyHUD base timeline." } end
	ov = overlayTrack(c.tl, true)
	local overlays = list(c.tl:GetItemListInTrack("video", ov))
	local answered = 0
	local startFrame = c.tl:GetStartFrame()
	if scope == "all" then -- SkyHUD's flip markers come back on every rebuild
		for _ = 1, 1000 do
			if not c.tl:DeleteMarkerByCustomData(FLIP_MARKER) then break end
		end
	end
	for line in out:gmatch("[^\r\n]+") do
		local status, id, note, flips = line:match("^(%a+)\t([^\t]*)\t([^\t]*)\t?(.*)$")
		local it = id and shots[id]
		if it then answered = answered + 1 end
		if it then
			-- An old HUD over this shot goes, whether or not a new one comes:
			-- it may show another clip's data.
			local a, b = it:GetStart(), it:GetEnd()
			local old = {}
			for _, o in ipairs(overlays) do
				if overlaps(a, b, o:GetStart(), o:GetEnd()) then old[#old + 1] = o end
			end
			if #old > 0 then c.tl:DeleteClips(old, false) end
		end
		if it and status == "ok" then
			local a = it:GetStart()
			local added = c.mp:AppendToTimeline({ { mediaPoolItem = base, startFrame = 0, endFrame = it:GetDuration(),
				trackIndex = ov, recordFrame = a, mediaType = 1 } })
			local hudItem = added and added[1]
			local placed = hudItem and hudItem:GetStart() == a and hudItem:GetDuration() == it:GetDuration()
			local comp = placed and hudItem:ImportFusionComp(work .. SEP .. "comps" .. SEP .. id .. ".comp")
			if comp then
				pcall(function() hudItem:SetName("SkyHUD") end)
				result.ok = result.ok + 1
				-- a red marker where each flip starts
				for at in (flips or ""):gmatch("[%d%.]+") do
					local f = a - startFrame + math.floor(tonumber(at) * fps + 0.5)
					-- replace an older flip marker there, never the user's own marker
					if c.tl:GetMarkerCustomData(f) == FLIP_MARKER then c.tl:DeleteMarkerAtFrame(f) end
					c.tl:AddMarker(f, "Red", "Flip", "Added by SkyHUD", 1, FLIP_MARKER)
				end
				if note ~= "" then result.notes[#result.notes + 1] = it:GetName() .. ": " .. note end
			else
				if hudItem then c.tl:DeleteClips({ hudItem }, false) end
				result.skipped[#result.skipped + 1] = it:GetName() .. ": could not place the HUD"
			end
			os.remove(work .. SEP .. "comps" .. SEP .. id .. ".comp")
		elseif it then
			result.skipped[#result.skipped + 1] = it:GetName() .. ": " .. note
		end
		overlays = list(c.tl:GetItemListInTrack("video", ov))
	end
	if answered == 0 then
		result.message = "skyhud gave no result:\n" .. out
		return result
	end

	if scope == "all" then -- drop overlays whose shot is gone
		local orphans = {}
		for _, o in ipairs(overlays) do
			local keep = false
			for _, it in ipairs(shotItems(c, track, "all")) do
				if overlaps(it:GetStart(), it:GetEnd(), o:GetStart(), o:GetEnd()) then keep = true end
			end
			if not keep then orphans[#orphans + 1] = o end
		end
		if #orphans > 0 then c.tl:DeleteClips(orphans, false) end
	end
	result.message = string.format("HUD added to %d shot(s).", result.ok)
	return result
end

-- useLayout copies piece positions from the HUD under the playhead to the
-- settings, then rebuilds every HUD.
function SkyHUD.useLayout()
	local c, err = context()
	if not c then return { ok = 0, skipped = {}, message = err } end
	local ov = overlayTrack(c.tl, false)
	local hudItem = ov and itemAt(list(c.tl:GetItemListInTrack("video", ov)), playhead(c.tl))
	local comp = hudItem and hudItem:GetFusionCompByIndex(1)
	if not comp then return { ok = 0, skipped = {}, message = "Put the playhead over a SkyHUD overlay first." } end
	local s = SkyHUD.loadSettings(c.project)
	local layout = s.layouts[s.design] or {}
	for _, p in ipairs(PIECES) do
		local tool = comp:FindTool(TOOLS[p])
		if tool then
			local center = tool:GetInput("Center")
			layout[p] = { x = center[1], y = center[2], size = tool:GetInput("Size") }
		end
	end
	s.layouts[s.design] = layout
	SkyHUD.saveSettings(c.project, s)
	return SkyHUD.fill("all")
end

-- hasOverlays reports whether the timeline has any SkyHUD overlay.
function SkyHUD.hasOverlays()
	local c = context()
	local ov = c and overlayTrack(c.tl, false)
	return ov and #list(c.tl:GetItemListInTrack("video", ov)) > 0 or false
end

-- remove deletes every HUD overlay on the timeline.
function SkyHUD.remove()
	local c, err = context()
	if not c then return { ok = 0, skipped = {}, message = err } end
	local ov = overlayTrack(c.tl, false)
	local items = ov and list(c.tl:GetItemListInTrack("video", ov)) or {}
	if #items > 0 then c.tl:DeleteClips(items, false) end
	return { ok = #items, skipped = {}, message = string.format("Removed %d HUD overlay(s).", #items) }
end

-- designs lists the designs skyhud knows: { id, name, builtin, pieces = {piece = true}, problems,
-- colors = { { name, { r, g, b } }, ... } (main first), font }.
function SkyHUD.designs()
	local out = {}
	for line in (run({ "design", "list", "--tsv", "--dir", designsDir() }) or ""):gmatch("[^\r\n]+") do
		local id, name, kind, pieces, problems, colors, font = line:match("^([^\t]*)\t([^\t]*)\t([^\t]*)\t([^\t]*)\t(%d+)\t([^\t]*)\t([^\t]*)$")
		if id then
			local set = {}
			for p in pieces:gmatch("[^,]+") do set[p] = true end
			local list = {}
			for key, hex in colors:gmatch("([^,=]+)=#(%x%x%x%x%x%x)") do
				list[#list + 1] = { key, { tonumber(hex:sub(1, 2), 16) / 255, tonumber(hex:sub(3, 4), 16) / 255, tonumber(hex:sub(5, 6), 16) / 255 } }
			end
			out[#out + 1] = { id = id, name = name, builtin = kind == "builtin", pieces = set, problems = tonumber(problems), colors = list, font = font }
		end
	end
	return out
end

-- saveDesign writes a copy of the current design with a new name and the
-- colors, font and layout picked in the panel; it becomes the chosen design.
function SkyHUD.saveDesign(name)
	local c, err = context()
	if not c then return { message = err } end
	local s = SkyHUD.loadSettings(c.project)
	local args = { "design", "save", "--from", s.design, "--name", name, "--dir", designsDir() }
	for key, rgb in pairs(s.colors[s.design] or {}) do
		args[#args + 1] = "--color"
		args[#args + 1] = string.format("%s=#%02x%02x%02x", key,
			math.floor(rgb[1] * 255 + 0.5), math.floor(rgb[2] * 255 + 0.5), math.floor(rgb[3] * 255 + 0.5))
	end
	local font = s.fonts[s.design]
	if font and font ~= "" then
		args[#args + 1] = "--font"
		args[#args + 1] = font
	end
	local layout = s.layouts[s.design]
	if layout and next(layout) then
		args[#args + 1] = "--layout"
		args[#args + 1] = encode(layout)
	end
	local out = (run(args) or ""):gsub("%s+$", "")
	if not out:match("^[%w%-]+$") then return { message = "Could not save the design:\n" .. out } end
	s.design = out
	SkyHUD.saveSettings(c.project, s)
	return { message = "Saved as design \"" .. name .. "\" in " .. designsDir() .. SEP .. out }
end

-- scan lists the videos of a folder, their data and their flight logs.
function SkyHUD.scan(folder)
	local c = context()
	local s = c and SkyHUD.loadSettings(c.project) or defaults()
	local args = { "scan", folder }
	if s.logs and s.logs ~= "" then
		args[#args + 1] = "--logs"
		args[#args + 1] = s.logs
	end
	return { message = run(args) or "could not run skyhud" }
end

---------------------------------------------------------------------------
-- Window

local HEIGHT_MODES = { { "start", "Above takeoff (start of recording)" }, { "sea", "Above sea level" } }
local UNITS = { { "metric", "km/h, m" }, { "imperial", "mph, ft" } }

local function currentShot(c, track)
	local it = shotItems(c, track, "current")[1]
	if not it then return nil end
	local path = filePath(it)
	return it, path
end

local function report(res)
	local lines = { res.message or res.error or "" }
	for _, n in ipairs(res.notes or {}) do lines[#lines + 1] = "note: " .. n end
	for _, n in ipairs(res.skipped or {}) do lines[#lines + 1] = "skipped " .. n end
	return table.concat(lines, "\n")
end

function SkyHUD.window()
	local c, err = context()
	if not c then
		print("SkyHUD: " .. err)
		return
	end
	local s = SkyHUD.loadSettings(c.project)
	local ui = fu.UIManager
	local disp = bmd.UIDispatcher(ui)
	local win = disp:AddWindow({
		ID = "SkyHUD",
		WindowTitle = "SkyHUD",
		Geometry = { 100, 60, 1040, 600 },
		ui:HGroup{
			ui:VGroup{ Weight = 1,
				ui:Label{ ID = "Shot", Text = "", WordWrap = true, Weight = 0 },
				ui:TextEdit{ ID = "Info", ReadOnly = true, Weight = 1 },
				ui:Button{ ID = "Refresh", Text = "Read the shot under the playhead", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:Label{ Text = "<b>Design</b>", Weight = 0 },
					ui:ComboBox{ ID = "Design" },
				},
				ui:HGroup{ Weight = 0,
					ui:Label{ Text = "HUD size", Weight = 0 },
					ui:Slider{ ID = "Scale", Minimum = 50, Maximum = 200 },
					ui:Label{ ID = "ScaleText", Text = "", Weight = 0 },
					ui:Label{ Text = "Units", Weight = 0 },
					ui:ComboBox{ ID = "Units", Weight = 0 },
				},
				ui:HGroup{ Weight = 0,
					ui:Label{ Text = "<b>Colors</b>", Weight = 0 },
					ui:Label{ Text = "Font", Weight = 0 },
					ui:ComboBox{ ID = "Font" },
					ui:Button{ ID = "ResetColors", Text = "Reset colors", Weight = 0 },
				},
				ui:HGroup{ Weight = 0,
					ui:VGroup{ ui:Label{ ID = "ColorName1", Text = "", Weight = 0 }, ui:ColorPicker{ ID = "Color1" } },
					ui:VGroup{ ui:Label{ ID = "ColorName2", Text = "", Weight = 0 }, ui:ColorPicker{ ID = "Color2" } },
					ui:VGroup{ ui:Label{ ID = "ColorName3", Text = "", Weight = 0 }, ui:ColorPicker{ ID = "Color3" } },
					ui:VGroup{ ui:Label{ ID = "ColorName4", Text = "", Weight = 0 }, ui:ColorPicker{ ID = "Color4" } },
				},
				ui:Label{ Text = "<b>Show</b>", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:CheckBox{ ID = "speed", Text = "Speed" },
					ui:CheckBox{ ID = "height", Text = "Height" },
					ui:CheckBox{ ID = "gforce", Text = "G-force" },
					ui:CheckBox{ ID = "horizon", Text = "Horizon" },
				},
				ui:HGroup{ Weight = 0,
					ui:CheckBox{ ID = "path", Text = "Flight path" },
					ui:CheckBox{ ID = "sticks", Text = "Stick positions" },
					ui:CheckBox{ ID = "battery", Text = "Battery" },
					ui:CheckBox{ ID = "flip", Text = "Flip badge" },
				},
				ui:Label{ Text = "The flight path's shape can show where you flew.", Weight = 0 },
			},
			ui:VGroup{ Weight = 1,
				ui:Label{ Text = "Sticks and battery come from DJI flight logs (DJI Avata 2).", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:Label{ ID = "Logs", Text = "", WordWrap = true },
					ui:Button{ ID = "ChooseLogs", Text = "Choose logs folder…", Weight = 0 },
				},
				ui:HGroup{ Weight = 0,
					ui:LineEdit{ ID = "Key", EchoMode = "Password", PlaceholderText = "DJI API key" },
					ui:Button{ ID = "SaveKey", Text = "Save key", Weight = 0 },
				},
				ui:Label{ ID = "KeyStatus", Text = "", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:Label{ Text = "Height", Weight = 0 },
					ui:ComboBox{ ID = "HeightMode" },
				},
				ui:Label{ Text = "GPS height is rough (about ±10 m).", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:Label{ Text = "Shots on video track", Weight = 0 },
					ui:SpinBox{ ID = "Track", Minimum = 1, Maximum = 20, Value = tonumber(s.track) or 1 },
				},
				ui:HGroup{ Weight = 0,
					ui:Label{ ID = "NudgeLabel", Text = "Sync nudge for this file (s)", Weight = 0 },
					ui:DoubleSpinBox{ ID = "Nudge", Minimum = -30, Maximum = 30, SingleStep = 0.05, Decimals = 2 },
				},
				ui:Button{ ID = "FillAll", Text = "Add HUD to all shots", Weight = 0 },
				ui:Button{ ID = "FillCurrent", Text = "Update this shot", Weight = 0 },
				ui:Button{ ID = "UseLayout", Text = "Use this shot's layout for all", Weight = 0 },
				ui:Label{ Text = "Move pieces on the Fusion page of any SkyHUD overlay,", Weight = 0 },
				ui:Label{ Text = "put the playhead on it, then press the button above.", Weight = 0 },
				ui:Button{ ID = "Remove", Text = "Remove all HUDs", Weight = 0 },
				ui:Label{ Text = "<b>Save as new design</b> (no Fusion needed)", Weight = 0 },
				ui:HGroup{ Weight = 0,
					ui:LineEdit{ ID = "NewName", PlaceholderText = "Name (keeps the colors, font, size and layout)" },
					ui:Button{ ID = "SaveDesign", Text = "Save", Weight = 0 },
				},
				ui:Button{ ID = "Scan", Text = "Scan a flight folder…", Weight = 0 },
				ui:TextEdit{ ID = "Status", ReadOnly = true, Weight = 1 },
			},
		},
	})
	local items = win:GetItems()
	local designs = {}
	local current
	local loading = false -- set while the panel fills in pickers itself
	local function showColors()
		loading = true
		local picked = s.colors[s.design] or {}
		local list = current and current.colors or {}
		for i = 1, COLOR_SLOTS do
			local entry = list[i]
			items["ColorName" .. i].Hidden = not entry
			items["Color" .. i].Hidden = not entry
			if entry then
				local rgb = picked[entry[1]] or entry[2]
				items["ColorName" .. i].Text = COLOR_LABELS[entry[1]] or entry[1]
				items["Color" .. i].Color = { R = rgb[1], G = rgb[2], B = rgb[3], A = 1 }
			end
		end
		local font = s.fonts[s.design] or ""
		items.Font.CurrentIndex = font == "mono" and 1 or font == "sans" and 2 or 0
		loading = false
	end
	local function showDesigns()
		designs = SkyHUD.designs()
		loading = true
		items.Design:Clear()
		current = nil
		for i, d in ipairs(designs) do
			local label = d.name .. (d.builtin and "" or " (yours)") .. (d.problems > 0 and " - has problems" or "")
			items.Design:AddItem(label)
			if d.id == s.design then current = d; items.Design.CurrentIndex = i - 1 end
		end
		for _, p in ipairs(PIECES) do
			local has = not current or current.pieces[p]
			items[p].Enabled = has and true or false
			items[p].Text = LABELS[p] .. (has and "" or " (not in this design)")
			items[p].Checked = has and s.show[p] ~= false
		end
		loading = false
		showColors()
	end
	for _, f in ipairs({ "Design's font", "Mono", "Sans" }) do items.Font:AddItem(f) end
	showDesigns()
	items.Scale.Value = math.floor((tonumber(s.scale) or 1) * 100 + 0.5)
	items.ScaleText.Text = items.Scale.Value .. " %"
	for i, m in ipairs(HEIGHT_MODES) do
		items.HeightMode:AddItem(m[2])
		if s.height == m[1] then items.HeightMode.CurrentIndex = i - 1 end
	end
	for i, u in ipairs(UNITS) do
		items.Units:AddItem(u[2])
		if s.units == u[1] then items.Units.CurrentIndex = i - 1 end
	end

	local function showLogs()
		items.Logs.Text = (s.logs and s.logs ~= "") and ("Logs: " .. s.logs) or "Logs: not set"
	end
	showLogs()
	items.KeyStatus.Text = SkyHUD.keyStatus()

	local fileName
	local function refresh()
		local it, path = currentShot(c, tonumber(s.track) or 1)
		if not it then
			fileName = nil
			items.Shot.Text = "No clip under the playhead on video track " .. (s.track or 1) .. "."
			items.Info.PlainText = ""
			return
		end
		fileName = path and path:match("[^/\\]+$")
		items.Shot.Text = "<b>" .. it:GetName() .. "</b>"
		items.Info.PlainText = path and SkyHUD.inspect(path) or "This clip has no file."
		items.Nudge.Value = tonumber(fileName and s.nudge[fileName] or 0) or 0
	end

	local function save() SkyHUD.saveSettings(c.project, s) end
	local function act(f, arg)
		items.Status.PlainText = "Working..."
		local ok, res = pcall(f, arg)
		items.Status.PlainText = ok and report(res) or tostring(res)
		s = SkyHUD.loadSettings(c.project)
	end

	for _, p in ipairs(PIECES) do
		win.On[p].Clicked = function() s.show[p] = items[p].Checked; save() end
	end
	-- A color or font change rebuilds the HUDs already on the timeline, half a
	-- second after the last change (a picker sends many while dragging).
	local recolor = ui:Timer{ ID = "Recolor", Interval = 500, SingleShot = true }
	local function colorsChanged()
		save()
		if SkyHUD.hasOverlays() then
			items.Status.PlainText = "Updating colors..."
			recolor:Start()
		end
	end
	for i = 1, COLOR_SLOTS do
		win.On["Color" .. i].ColorChanged = function()
			local entry = current and current.colors[i]
			if loading or not entry then return end
			local col = items["Color" .. i].Color
			local was = (s.colors[s.design] or {})[entry[1]] or entry[2]
			-- the picker also reports colors the panel set itself: only act on a real change
			if not col or math.abs(col.R - was[1]) + math.abs(col.G - was[2]) + math.abs(col.B - was[3]) < 0.002 then return end
			s.colors[s.design] = s.colors[s.design] or {}
			s.colors[s.design][entry[1]] = { col.R, col.G, col.B }
			colorsChanged()
		end
	end
	function win.On.Font.CurrentIndexChanged()
		local font = ({ "", "mono", "sans" })[items.Font.CurrentIndex + 1] or ""
		if loading or font == (s.fonts[s.design] or "") then return end
		s.fonts[s.design] = font
		colorsChanged()
	end
	function win.On.ResetColors.Clicked()
		s.colors[s.design], s.fonts[s.design] = nil, nil
		showColors()
		colorsChanged()
	end
	function win.On.Design.CurrentIndexChanged()
		if loading then return end
		local d = designs[items.Design.CurrentIndex + 1]
		if d and d.id ~= s.design then
			s.design = d.id
			save()
			showDesigns()
		end
	end
	function win.On.Scale.ValueChanged()
		s.scale = items.Scale.Value / 100
		items.ScaleText.Text = items.Scale.Value .. " %"
		save()
	end
	function win.On.SaveDesign.Clicked()
		local name = items.NewName.Text
		if not name or not name:match("%S") then
			items.Status.PlainText = "Give the new design a name first."
			return
		end
		local res = SkyHUD.saveDesign(name)
		items.Status.PlainText = res.message
		s = SkyHUD.loadSettings(c.project)
		showDesigns()
	end
	function win.On.Scan.Clicked()
		local dir = fu:RequestDir()
		if dir then
			items.Status.PlainText = "Scanning..."
			items.Status.PlainText = SkyHUD.scan(dir).message
		end
	end
	function win.On.HeightMode.CurrentIndexChanged()
		local m = HEIGHT_MODES[items.HeightMode.CurrentIndex + 1]
		if m then s.height = m[1]; save() end
	end
	function win.On.Units.CurrentIndexChanged()
		local u = UNITS[items.Units.CurrentIndex + 1]
		if u then s.units = u[1]; save() end
	end
	function win.On.Track.ValueChanged() s.track = items.Track.Value; save() end
	function win.On.Nudge.ValueChanged()
		if fileName then s.nudge[fileName] = items.Nudge.Value; save() end
	end
	function win.On.Refresh.Clicked() refresh() end
	function win.On.ChooseLogs.Clicked()
		local dir = fu:RequestDir(s.logs ~= "" and s.logs or nil)
		if dir then
			s.logs = dir
			save()
			showLogs()
		end
	end
	function win.On.SaveKey.Clicked()
		local key = items.Key.Text
		items.Key.Text = ""
		if key and key:match("%S") then items.KeyStatus.Text = SkyHUD.saveKey(key) end
	end
	function win.On.FillAll.Clicked() act(SkyHUD.fill, "all") end
	function win.On.FillCurrent.Clicked() act(SkyHUD.fill, "current") end
	function win.On.UseLayout.Clicked() act(SkyHUD.useLayout) end
	function win.On.Remove.Clicked() act(SkyHUD.remove) end
	function win.On.SkyHUD.Close() disp:ExitLoop() end

	function disp.On.Timeout(ev)
		if ev.who == "Recolor" then
			act(SkyHUD.fill, "all")
		elseif ev.who == "AutoClose" then
			disp:ExitLoop()
		end
	end
	local autoClose = SKYHUD_AUTOCLOSE and ui:Timer{ ID = "AutoClose", Interval = SKYHUD_AUTOCLOSE * 1000, SingleShot = true }
	if autoClose then autoClose:Start() end
	win:Show()
	refresh()
	disp:RunLoop()
	win:Hide()
end

---------------------------------------------------------------------------
-- Entry

if SKYHUD_HEADLESS then
	local ok, res = pcall(SkyHUD[SKYHUD_HEADLESS], SKYHUD_ARG)
	if not ok then res = { error = tostring(res) } end
	if SKYHUD_RESULT then bmd.writefile(SKYHUD_RESULT, res) end
	return
end

if SkyHUD.window then SkyHUD.window() end
