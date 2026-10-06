package integration

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/mirusu400/aram-core/cheat"
	"github.com/mirusu400/aram-frontend/frontend"
)

const memoryPageSize = 32

var errMemorySessionChanged = errors.New("the game or search session changed; refreshed the current session")

type memoryToolState struct {
	fields     map[string]string
	offset     int
	selected   *frontend.MemoryResult
	scanRegion string
}

var memoryTypes = []string{"u8", "i8", "u16", "i16", "u32", "i32", "u64", "i64", "f32", "f64"}
var memoryComparisons = map[string]cheat.Comparison{
	"unknown": cheat.CompareUnknown, "equal": cheat.CompareEqual,
	"not_equal": cheat.CompareNotEqual, "greater": cheat.CompareGreater,
	"less": cheat.CompareLess, "changed": cheat.CompareChanged,
	"unchanged": cheat.CompareUnchanged, "increased": cheat.CompareIncreased,
	"decreased": cheat.CompareDecreased,
}

// Called only while operationMu is held, including publication under mu.
func (backend *Backend) invalidateMemoryTool() {
	backend.memorySession++
	backend.memoryTool = memoryToolState{}
	if backend.cheats != nil {
		backend.cheats.Engine().ResetScan()
	}
}

func memoryType(name string) (cheat.ValueType, error) {
	for index, candidate := range memoryTypes {
		if candidate == name {
			return cheat.ValueType(index), nil
		}
	}
	return 0, fmt.Errorf("invalid numeric type %q", name)
}

// Integers are decimal unless explicitly prefixed by 0x, with an optional sign.
func memoryInteger(text string) (string, int) {
	sign := ""
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+") {
		sign, text = text[:1], text[1:]
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(text), "0x") {
		base, text = 16, text[2:]
	}
	return sign + text, base
}

func parseMemoryValue(name, text string) (cheat.Value, error) {
	valueType, err := memoryType(name)
	if err != nil {
		return cheat.Value{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return cheat.Value{}, errors.New("a numeric value is required")
	}
	value := cheat.Value{Type: valueType}
	width := valueType.Size() * 8
	if name[0] == 'f' {
		number, parseErr := strconv.ParseFloat(text, width)
		if parseErr != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return cheat.Value{}, fmt.Errorf("%q is not a finite %s value", text, name)
		}
		if width == 32 {
			value.Bits = uint64(math.Float32bits(float32(number)))
		} else {
			value.Bits = math.Float64bits(number)
		}
	} else {
		digits, base := memoryInteger(text)
		if name[0] == 'i' {
			number, parseErr := strconv.ParseInt(digits, base, width)
			if parseErr != nil {
				return cheat.Value{}, fmt.Errorf("%q is outside the %s range", text, name)
			}
			value.Bits = uint64(number)
			if width < 64 {
				value.Bits &= (uint64(1) << width) - 1
			}
		} else {
			digits = strings.TrimPrefix(digits, "+")
			number, parseErr := strconv.ParseUint(digits, base, width)
			if parseErr != nil {
				return cheat.Value{}, fmt.Errorf("%q is outside the %s range", text, name)
			}
			value.Bits = number
		}
	}
	return value, value.Validate()
}

func formatMemoryValue(value cheat.Value) string {
	name := memoryTypes[int(value.Type)]
	width := value.Type.Size() * 8
	if name[0] == 'f' {
		number := math.Float64frombits(value.Bits)
		if width == 32 {
			number = float64(math.Float32frombits(uint32(value.Bits)))
		}
		return strconv.FormatFloat(number, 'g', -1, width)
	}
	if name[0] == 'i' {
		return strconv.FormatInt(int64(value.Bits<<(64-width))>>(64-width), 10)
	}
	return strconv.FormatUint(value.Bits, 10)
}

func parseMemoryAddress(text string) (uint32, error) {
	digits, base := memoryInteger(strings.TrimSpace(text))
	number, err := strconv.ParseUint(strings.TrimPrefix(digits, "+"), base, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid 32-bit guest address %q", text)
	}
	return uint32(number), nil
}

func (backend *Backend) memorySnapshot(ctx context.Context) (frontend.ToolSnapshot, error) {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return frontend.ToolSnapshot{}, err
	}
	return backend.memorySnapshotLocked("")
}

func (backend *Backend) memorySnapshotLocked(status string) (frontend.ToolSnapshot, error) {
	library, unavailable := backend.cheatLibrary()
	snapshot := frontend.ToolSnapshot{Title: "Memory Search", Session: backend.memorySession}
	if library == nil {
		snapshot.Lines = []string{"Memory search is unavailable: " + emptyFallback(unavailable, "open a supported game first")}
		return snapshot, nil
	}
	engine := library.Engine()
	state := &backend.memoryTool
	if state.fields == nil {
		state.fields = map[string]string{"type": "u32", "comparison": "equal", "value": "", "region": ""}
	}
	model := &frontend.MemorySnapshot{PageSize: memoryPageSize, Status: status}
	summary, err := engine.ScanSummary()
	if err != nil && !errors.Is(err, cheat.ErrScanNotStarted) {
		return snapshot, err
	}
	// A page read failure is reported with the controls intact, so the user
	// can still page away or start a new search.
	var pageErr error
	if err == nil {
		model.Active, model.Type, model.Total = true, memoryTypes[int(summary.Type)], summary.Total
		if state.offset >= summary.Total {
			state.offset = max(0, ((summary.Total-1)/memoryPageSize)*memoryPageSize)
		}
		model.Offset = state.offset
		page, err := engine.ScanPage(state.offset, memoryPageSize)
		if err != nil {
			pageErr = err
			model.Status = "The current result page is not readable: " + err.Error()
		}
		for _, match := range page.Matches {
			model.Results = append(model.Results, memoryResult(engine, match))
		}
	}
	if state.selected != nil && model.Active {
		valueType, _ := memoryType(state.selected.Type)
		value, err := engine.Read(state.selected.Address, valueType)
		if err != nil {
			state.selected = nil
			if pageErr == nil {
				model.Status = "The selected address is no longer readable: " + err.Error()
			}
		} else {
			selected := memoryResult(engine, cheat.Match{Address: state.selected.Address, Region: state.selected.Region, Value: value})
			state.selected = &selected
			model.Selected = &selected
		}
	} else {
		state.selected = nil
	}
	typeOptions := make([]frontend.ToolFieldOption, len(memoryTypes))
	for index, name := range memoryTypes {
		typeOptions[index] = frontend.ToolFieldOption{Value: name, Label: name}
	}
	comparisonOptions := []frontend.ToolFieldOption{
		{Value: "equal", Label: "Exact value"}, {Value: "unknown", Label: "Unknown initial value"},
		{Value: "increased", Label: "Increased"}, {Value: "decreased", Label: "Decreased"},
		{Value: "changed", Label: "Changed"}, {Value: "unchanged", Label: "Unchanged"},
		{Value: "not_equal", Label: "Not equal"}, {Value: "greater", Label: "Greater than"}, {Value: "less", Label: "Less than"},
	}
	regionOptions := []frontend.ToolFieldOption{{Value: "", Label: "All searchable regions"}}
	for _, region := range engine.Regions() {
		if region.Scannable {
			regionOptions = append(regionOptions, frontend.ToolFieldOption{Value: region.Name, Label: fmt.Sprintf("%s (0x%08x, %d bytes)", region.Name, region.Start, region.Size)})
		}
	}
	snapshot.Fields = []frontend.ToolField{
		{ID: "type", Label: "Numeric type", Value: state.fields["type"], Options: typeOptions},
		{ID: "comparison", Label: "Comparison", Value: state.fields["comparison"], Options: comparisonOptions},
		{ID: "value", Label: "Search value", Value: state.fields["value"], Placeholder: "Decimal, 0x hex, or float"},
		{ID: "region", Label: "Memory region", Value: state.fields["region"], Options: regionOptions},
	}
	snapshot.Actions = []frontend.ToolAction{
		{ID: "scan", Label: "First scan", Enabled: true},
		{ID: "refine", Label: "Next scan", Enabled: model.Active},
		{ID: "reset", Label: "New search", Enabled: true},
	}
	snapshot.Memory = model
	return snapshot, pageErr
}

func memoryResult(engine *cheat.Engine, match cheat.Match) frontend.MemoryResult {
	result := frontend.MemoryResult{Address: match.Address, Region: match.Region, Value: formatMemoryValue(match.Value), Type: memoryTypes[int(match.Value.Type)], Expected: strconv.FormatUint(match.Value.Bits, 16)}
	for _, region := range engine.Regions() {
		if region.Name == match.Region {
			result.Writable = region.Writable
			break
		}
	}
	return result
}

func (backend *Backend) executeMemoryAction(ctx context.Context, request frontend.ToolRequest) (frontend.ToolSnapshot, error) {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return frontend.ToolSnapshot{}, err
	}
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		return frontend.ToolSnapshot{}, fmt.Errorf("memory search unavailable: %s", emptyFallback(unavailable, "no game loaded"))
	}
	respond := func(status string, actionErr error) (frontend.ToolSnapshot, error) {
		snapshot, err := backend.memorySnapshotLocked(status)
		return snapshot, errors.Join(actionErr, err)
	}
	if request.Session != backend.memorySession {
		return respond(errMemorySessionChanged.Error(), errMemorySessionChanged)
	}
	engine, state := library.Engine(), &backend.memoryTool
	if state.fields == nil {
		state.fields = map[string]string{"type": "u32", "comparison": "equal", "region": "", "value": ""}
	}
	for _, name := range []string{"type", "comparison", "region", "value"} {
		if value, ok := request.Fields[name]; ok {
			state.fields[name] = value
		}
	}
	status := ""
	var actionErr error
	switch request.Action {
	case "scan", "refine":
		valueType, err := memoryType(state.fields["type"])
		if err != nil {
			return respond("", err)
		}
		comparison, ok := memoryComparisons[state.fields["comparison"]]
		if !ok {
			return respond("", fmt.Errorf("invalid comparison %q", state.fields["comparison"]))
		}
		var target *cheat.Value
		if comparison >= cheat.CompareEqual && comparison <= cheat.CompareLess {
			value, err := parseMemoryValue(state.fields["type"], state.fields["value"])
			if err != nil {
				return respond("", err)
			}
			target = &value
		}
		if request.Action == "scan" {
			var regions []string
			if name := state.fields["region"]; name != "" {
				valid := false
				for _, region := range engine.Regions() {
					if region.Name == name && region.Scannable {
						valid = true
					}
				}
				if !valid {
					return respond("", fmt.Errorf("region %q is not searchable", name))
				}
				regions = []string{name}
			}
			_, actionErr = engine.StartScan(cheat.ScanRequest{Type: valueType, Comparison: comparison, Value: target, Regions: regions})
			if actionErr == nil {
				backend.memorySession++
				state.scanRegion = state.fields["region"]
				state.offset, state.selected = 0, nil
			}
		} else {
			summary, err := engine.ScanSummary()
			if err != nil {
				return respond("", err)
			}
			if summary.Type != valueType {
				return respond("", errors.New("numeric type changed; start a first scan"))
			}
			if state.fields["region"] != state.scanRegion {
				return respond("", errors.New("memory region changed; start a first scan"))
			}
			_, actionErr = engine.RefineScan(cheat.NextScanRequest{Comparison: comparison, Value: target})
			if actionErr == nil {
				backend.memorySession++
				state.offset, state.selected = 0, nil
			}
		}
		if actionErr == nil {
			status = "Scan complete"
		}
	case "reset":
		engine.ResetScan()
		backend.memorySession++
		state.offset, state.selected = 0, nil
		status = "Search cleared"
	case "previous":
		state.offset = max(0, state.offset-memoryPageSize)
	case "next":
		state.offset += memoryPageSize
	case "refresh":
	case "select":
		address, err := parseMemoryAddress(request.Fields["address"])
		if err != nil {
			return respond("", err)
		}
		page, err := engine.ScanPage(state.offset, memoryPageSize)
		if err != nil {
			return respond("", err)
		}
		found := false
		for _, match := range page.Matches {
			if match.Address == address {
				result := memoryResult(engine, match)
				state.selected = &result
				found = true
				break
			}
		}
		if !found {
			actionErr = errors.New("select an address from the current result page")
		}
	case "write":
		if state.selected == nil {
			return respond("", errors.New("select a result before writing"))
		}
		address, err := parseMemoryAddress(request.Fields["address"])
		if err != nil || address != state.selected.Address {
			return respond("", errors.New("the selected address changed; select the result again"))
		}
		value, err := parseMemoryValue(state.selected.Type, request.Fields["new_value"])
		if err != nil {
			return respond("", err)
		}
		expectedBits, err := strconv.ParseUint(request.Fields["expected"], 16, 64)
		expected := cheat.Value{Type: value.Type, Bits: expectedBits}
		if err != nil || expected.Validate() != nil {
			return respond("", errors.New("the displayed value token is invalid; refresh the selected address"))
		}
		// Re-read immediately before Engine.Write. The engine also checks the
		// exact expected bytes under its execution lock.
		current, err := engine.Read(address, value.Type)
		if err != nil {
			return respond("", err)
		}
		if current != expected {
			return respond("Value changed; review the refreshed value before applying again", cheat.ErrUnexpectedOriginal)
		}
		actionErr = engine.Write(address, value, &expected)
		if actionErr == nil {
			status = fmt.Sprintf("Wrote %s to 0x%08x", formatMemoryValue(value), address)
		}
	default:
		actionErr = fmt.Errorf("unknown memory action %q", request.Action)
	}
	return respond(status, actionErr)
}
