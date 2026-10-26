package flow

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// CanvasWorkflowRunOptions controls a backend DAG execution run over a Canvas project.
type CanvasWorkflowRunOptions struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	ExecutionKey     string `json:"execution_key"`
	RefreshBindings  bool   `json:"refresh_bindings"`
	TargetNodeID     string `json:"target_node_id,omitempty"`
	SaveReport       bool   `json:"save_report,omitempty"`
	ReportTitle      string `json:"report_title,omitempty"`
}

var errCanvasWorkflowRevisionConflict = errors.New("canvas workflow revision conflict")

func (opts CanvasWorkflowRunOptions) validate() error {
	if opts.ExpectedRevision == nil || *opts.ExpectedRevision < 0 {
		return fmt.Errorf("%w: expected_revision is required and must be non-negative", ErrInvalid)
	}
	// Admission only: durable execution-key deduplication belongs to T12.
	if strings.TrimSpace(opts.ExecutionKey) == "" {
		return fmt.Errorf("%w: execution_key is required", ErrInvalid)
	}
	if opts.TargetNodeID != "" {
		return fmt.Errorf("%w: target_node_id is not supported", ErrInvalid)
	}
	return nil
}

func decodeCanvasWorkflowRunOptions(r io.Reader) (CanvasWorkflowRunOptions, error) {
	var opts CanvasWorkflowRunOptions
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&opts); err != nil {
		return opts, fmt.Errorf("%w: invalid workflow request JSON: %v", ErrInvalid, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return opts, fmt.Errorf("%w: expected one JSON object", ErrInvalid)
	}
	return opts, opts.validate()
}

// CanvasNodeSnapshot records per-node execution telemetry written back to node.metadata.lastRunSnapshot.
type CanvasNodeSnapshot struct {
	RunID       string `json:"runId"`
	ExecutedAt  string `json:"executedAt"`
	DurationMs  int64  `json:"durationMs"`
	InputCount  int    `json:"inputCount"`
	OutputCount int    `json:"outputCount"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	SampleItems []any  `json:"sampleItems,omitempty"`
}

// CanvasWorkflowRunResult summarizes a completed Canvas DAG run.
type CanvasWorkflowRunResult struct {
	RunID         string                        `json:"run_id"`
	CanvasID      string                        `json:"canvas_id"`
	Revision      int64                         `json:"revision"`
	ExecutedNodes int                           `json:"executed_nodes"`
	NodeSnapshots map[string]CanvasNodeSnapshot `json:"node_snapshots"`
	SavedReportID string                        `json:"saved_report_id,omitempty"`
}

type wfEvalResult struct {
	outputItems []any
	trueItems   []any
	falseItems  []any
	errText     string
	cycled      bool
}

// ExecuteCanvasWorkflow loads a user's Canvas project, evaluates all dataflow/modeling nodes
// (data-source, logic, skill, chart, markdown) in topological order, writes lastRunSnapshot
// and computed content back to each node, and increments the canvas revision.
// ExecuteCanvasWorkflow no longer bypasses admission, budget or skill policy.
// Call CanvasWorkflowManagedRunner.Run with a durable admitted execution.
func ExecuteCanvasWorkflow(ctx context.Context, db *gorm.DB, s Scope, userID, canvasID string, opts CanvasWorkflowRunOptions, now time.Time) (CanvasWorkflowRunResult, error) {
	if err := opts.validate(); err != nil {
		return CanvasWorkflowRunResult{}, err
	}
	return CanvasWorkflowRunResult{}, ErrApprovalRequired
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if m, ok := parent[key].(map[string]any); ok && m != nil {
		return m
	}
	m := map[string]any{}
	parent[key] = m
	return m
}

func asString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if math.Trunc(val) == val {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		if v == nil {
			return ""
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func wfEvalDataSource(db *gorm.DB, s Scope, _ map[string]any, meta map[string]any, refresh bool, nowISO string) ([]any, string) {
	ds := ensureMap(meta, "dataSource")
	format := strings.ToLower(strings.TrimSpace(asString(ds["format"])))
	if format == "" {
		format = "json"
	}
	rawText := strings.TrimSpace(asString(ds["rawText"]))
	if rawText == "" {
		rawText = strings.TrimSpace(asString(meta["content"]))
	}

	if sb, ok := ds["sourceBinding"].(map[string]any); ok && sb != nil {
		kind := strings.TrimSpace(asString(sb["kind"]))
		refID := strings.TrimSpace(asString(sb["refId"]))
		if (refresh || rawText == "") && s.TenantID != "" && s.WorkspaceID != "" {
			if kind == "topic" && refID != "" {
				if top, err := GetTopic(db, s, refID); err == nil {
					rows := []any{
						map[string]any{
							"id":       top.ID,
							"title":    top.Title,
							"question": top.Question,
							"notes":    top.Notes,
						},
					}
					if b, err := json.MarshalIndent(rows, "", "  "); err == nil {
						rawText = string(b)
						ds["rawText"] = rawText
						meta["content"] = rawText
						sb["lastSyncedAt"] = nowISO
					}
				}
			}
		}
	}

	items, err := wfParseRawItems(rawText, format, asString(ds["fileName"]))
	if err != nil {
		return nil, err.Error()
	}
	return items, ""
}

var wfTableDecimal = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?$`)

func wfTableScalar(raw string) any {
	if raw != strings.TrimSpace(raw) {
		return raw
	}
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}
	if wfTableDecimal.MatchString(raw) {
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsInf(value, 0) && (math.Trunc(value) != value || math.Abs(value) <= 9007199254740991) {
			return value
		}
	}
	return raw
}

func wfJSONItem(value any) any {
	if value == nil {
		return ""
	}
	if array, ok := value.([]any); ok {
		encoded, _ := json.Marshal(array)
		return map[string]any{"items": array, "text": string(encoded)}
	}
	return value
}

func wfParseRawItems(rawText, format string, fileName ...string) ([]any, error) {
	text := strings.TrimPrefix(rawText, "\uFEFF")
	if strings.TrimSpace(text) == "" {
		return []any{}, nil
	}
	name := ""
	if len(fileName) > 0 {
		name = strings.ToLower(strings.TrimSpace(fileName[0]))
	}
	if format == "json" && strings.HasSuffix(name, ".jsonl") {
		out := []any{}
		for index, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var value any
			if err := json.Unmarshal([]byte(line), &value); err != nil {
				return nil, fmt.Errorf("JSONL 第 %d 行解析失败: %w", index+1, err)
			}
			out = append(out, wfJSONItem(value))
		}
		return out, nil
	}
	if format == "json" || format == "intel" {
		var value any
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			return nil, fmt.Errorf("JSON 解析失败: %w", err)
		}
		var rows []any
		switch parsed := value.(type) {
		case []any:
			rows = parsed
		case map[string]any:
			for _, key := range []string{"items", "rows", "data", "records", "list", "documents", "results"} {
				if array, ok := parsed[key].([]any); ok {
					rows = array
					break
				}
			}
			if rows == nil {
				rows = []any{parsed}
			}
		default:
			return nil, errors.New("JSON 必须是对象或数组")
		}
		out := make([]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, wfJSONItem(row))
		}
		return out, nil
	}
	if format == "csv" {
		reader := csv.NewReader(strings.NewReader(text))
		if strings.HasSuffix(name, ".tsv") {
			reader.Comma = '\t'
		}
		records, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("CSV/TSV 解析失败: %w", err)
		}
		if len(records) == 0 {
			return []any{}, nil
		}
		headers := records[0]
		seen := map[string]bool{}
		for index, header := range headers {
			key := strings.TrimSpace(header)
			if key == "" || seen[key] {
				return nil, errors.New("CSV/TSV 表头为空或重复")
			}
			headers[index] = key
			seen[key] = true
		}
		out := make([]any, 0, len(records)-1)
		for _, cells := range records[1:] {
			row := map[string]any{}
			for index, header := range headers {
				row[header] = wfTableScalar(cells[index])
			}
			out = append(out, row)
		}
		return out, nil
	}
	if format != "txt" && format != "markdown" {
		return nil, errors.New("unsupported data source format")
	}
	out := []any{}
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func wfLogicOperator(node map[string]any) string {
	// Read-only: never materialize an empty logic map into a node's metadata —
	// the managed commit serializes the graph doc and a reader-side artifact
	// would change the canvas (and monitor plan config hashes).
	meta, _ := node["metadata"].(map[string]any)
	logic, _ := meta["logic"].(map[string]any)
	op := strings.TrimSpace(asString(logic["operator"]))
	if op == "" {
		return "if_else"
	}
	return op
}

func wfIsDualInput(op string) bool {
	return op == "intersect" || op == "union" || op == "difference" || op == "join"
}

func wfReadField(item any, fieldPath string) any {
	clean := strings.TrimSpace(fieldPath)
	if clean == "" {
		if m, ok := item.(map[string]any); ok {
			for _, k := range []string{"text", "content", "title", "name", "prompt", "value", "id"} {
				if v, exists := m[k]; exists && v != nil && strings.TrimSpace(asString(v)) != "" {
					return v
				}
			}
		}
		return item
	}
	if m, ok := item.(map[string]any); ok {
		if value, exists := m[clean]; exists {
			return value
		}
	}
	cur := item
	for _, seg := range strings.Split(clean, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok || m == nil {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

func wfItemIdentity(item any, matchKey string) string {
	if strings.TrimSpace(matchKey) != "" {
		return strings.TrimSpace(asString(wfReadField(item, matchKey)))
	}
	return strings.TrimSpace(asString(item))
}

var wfComputeDecimal = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)

func wfComputeNumber(raw any, field string) (float64, error) {
	var n float64
	switch v := raw.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case string:
		if !wfComputeDecimal.MatchString(strings.TrimSpace(v)) {
			return 0, fmt.Errorf("missing or invalid numeric field: %s", field)
		}
		var err error
		n, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric field: %s", field)
		}
	default:
		return 0, fmt.Errorf("missing or invalid numeric field: %s", field)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("non-finite numeric field: %s", field)
	}
	return n, nil
}

func wfComputeField(item any, field string) (any, error) {
	for _, part := range strings.Split(field, ".") {
		if strings.TrimSpace(part) == "" {
			return nil, fmt.Errorf("invalid field: %s", field)
		}
	}
	if _, ok := item.(map[string]any); !ok {
		return nil, fmt.Errorf("missing field: %s", field)
	}
	return wfReadField(item, field), nil
}

func wfValidateFactorSource(item any) error {
	raw := wfReadField(item, "excretion_factor_source")
	source, ok := raw.(map[string]any)
	if !ok {
		return errors.New("excretion_factor_source requires version and evidence citations")
	}
	version, ok := source["version"].(string)
	citations, listOK := source["citations"].([]any)
	if !ok || strings.TrimSpace(version) == "" || !listOK || len(citations) == 0 {
		return errors.New("invalid excretion_factor_source")
	}
	for _, raw := range citations {
		citation, ok := raw.(map[string]any)
		if !ok {
			return errors.New("invalid excretion_factor_source citation")
		}
		id, ok := citation["evidence_id"].(string)
		revision, err := wfComputeNumber(citation["revision"], "revision")
		_, isString := citation["revision"].(string)
		if !ok || strings.TrimSpace(id) == "" || err != nil || isString || revision <= 0 || math.Trunc(revision) != revision || revision > 9007199254740991 {
			return errors.New("invalid excretion_factor_source citation")
		}
	}
	return nil
}

func wfFiniteCompute(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errors.New("non-finite compute result")
	}
	return nil
}

func wfRoundCompute(value float64) float64 {
	if math.Abs(value) > math.MaxFloat64/10000 {
		return value
	}
	// Match JavaScript Math.round, including negative half-way values.
	scaled := value * 10000
	floor := math.Floor(scaled)
	if scaled-floor >= 0.5 {
		floor++
	}
	return floor / 10000
}

func wfEvalArithmetic(item any, expr string) (float64, string) {
	clean := strings.TrimSpace(expr)
	if clean == "wbe_consumption_mg_1000p" {
		clean = "concentration_ng_l * flow_m3_day * excretion_factor / population"
	}
	type tok struct {
		kind string // "num", "ident", "op"
		num  float64
		text string
	}
	var tokens []tok
	runes := []rune(clean)
	for i := 0; i < len(runes); {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '(' || ch == ')' {
			tokens = append(tokens, tok{kind: "op", text: string(ch)})
			i++
			continue
		}
		if unicode.IsDigit(ch) || (ch == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])) {
			start := i
			i++
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			n, err := strconv.ParseFloat(string(runes[start:i]), 64)
			if err != nil {
				return 0, "无效数值"
			}
			tokens = append(tokens, tok{kind: "num", num: n})
			continue
		}
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '.') {
				i++
			}
			tokens = append(tokens, tok{kind: "ident", text: string(runes[start:i])})
			continue
		}
		return 0, "表达式包含非法字符"
	}
	if len(tokens) == 0 {
		return 0, "表达式为空"
	}

	pos := 0
	var parseAddSub func() (float64, error)
	var parsePrimary func() (float64, error)
	parsePrimary = func() (float64, error) {
		if pos >= len(tokens) {
			return 0, errors.New("表达式意外结束")
		}
		t := tokens[pos]
		if t.kind == "op" && t.text == "+" {
			pos++
			return parsePrimary()
		}
		if t.kind == "op" && t.text == "-" {
			pos++
			v, err := parsePrimary()
			return -v, err
		}
		if t.kind == "op" && t.text == "(" {
			pos++
			v, err := parseAddSub()
			if err != nil {
				return 0, err
			}
			if pos >= len(tokens) || tokens[pos].text != ")" {
				return 0, errors.New("缺少右括号")
			}
			pos++
			return v, nil
		}
		if t.kind == "num" {
			pos++
			return t.num, nil
		}
		if t.kind == "ident" {
			pos++
			if t.text == "excretion_factor" {
				if err := wfValidateFactorSource(item); err != nil {
					return 0, err
				}
			}
			raw, err := wfComputeField(item, t.text)
			if err != nil {
				return 0, err
			}
			return wfComputeNumber(raw, t.text)
		}
		return 0, errors.New("无效算子")
	}
	parseMulDiv := func() (float64, error) {
		left, err := parsePrimary()
		if err != nil {
			return 0, err
		}
		for pos < len(tokens) && tokens[pos].kind == "op" && (tokens[pos].text == "*" || tokens[pos].text == "/") {
			op := tokens[pos].text
			pos++
			right, err := parsePrimary()
			if err != nil {
				return 0, err
			}
			if op == "*" {
				left *= right
			} else if right == 0 {
				return 0, errors.New("division by zero")
			} else {
				left /= right
			}
			if err := wfFiniteCompute(left); err != nil {
				return 0, err
			}
		}
		return left, nil
	}
	parseAddSub = func() (float64, error) {
		left, err := parseMulDiv()
		if err != nil {
			return 0, err
		}
		for pos < len(tokens) && tokens[pos].kind == "op" && (tokens[pos].text == "+" || tokens[pos].text == "-") {
			op := tokens[pos].text
			pos++
			right, err := parseMulDiv()
			if err != nil {
				return 0, err
			}
			if op == "+" {
				left += right
			} else {
				left -= right
			}
			if err := wfFiniteCompute(left); err != nil {
				return 0, err
			}
		}
		return left, nil
	}
	val, err := parseAddSub()
	if err != nil {
		return 0, err.Error()
	}
	if pos != len(tokens) {
		return 0, "unexpected trailing expression tokens"
	}
	if err := wfFiniteCompute(val); err != nil {
		return 0, err.Error()
	}
	return wfRoundCompute(val), ""
}

func wfEvalPeriodComparison(items []any, logic map[string]any, raw any, outKey string) wfEvalResult {
	fail := func(err error) wfEvalResult { return wfEvalResult{errText: err.Error()} }
	config, ok := raw.(map[string]any)
	if !ok || len(config) != 5 || strings.TrimSpace(asString(logic["expression"])) != "" || strings.TrimSpace(asString(logic["fieldPath"])) != "" {
		return fail(errors.New("invalid or conflicting periodComparison"))
	}
	fields := map[string]string{}
	for _, name := range []string{"regionField", "substanceField", "periodField", "baselinePeriodField", "valueField"} {
		field, ok := config[name].(string)
		if !ok || strings.TrimSpace(field) == "" {
			return fail(errors.New("invalid periodComparison field"))
		}
		for _, part := range strings.Split(field, ".") {
			if strings.TrimSpace(part) == "" {
				return fail(errors.New("invalid periodComparison path"))
			}
		}
		fields[name] = field
	}
	key := func(item any, periodField string) (string, error) {
		parts := []any{}
		for _, field := range []string{fields["regionField"], fields["substanceField"], periodField} {
			raw, err := wfComputeField(item, field)
			if err != nil {
				return "", err
			}
			switch v := raw.(type) {
			case string:
				if strings.TrimSpace(v) == "" {
					return "", fmt.Errorf("missing comparison key: %s", field)
				}
				parts = append(parts, v)
			case float64, int, int64:
				n, err := wfComputeNumber(v, field)
				if err != nil {
					return "", err
				}
				if n == 0 {
					n = 0
				}
				parts = append(parts, n)
			default:
				return "", fmt.Errorf("invalid comparison key: %s", field)
			}
		}
		encoded, err := json.Marshal(parts)
		return string(encoded), err
	}
	index := map[string]float64{}
	statusKey := outKey + "_status"
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return fail(errors.New("comparison requires records"))
		}
		if _, exists := record[outKey]; exists {
			return fail(errors.New("duplicate compute outputField"))
		}
		if _, exists := record[statusKey]; exists {
			return fail(errors.New("duplicate compute outputField status"))
		}
		identity, err := key(item, fields["periodField"])
		if err != nil {
			return fail(err)
		}
		if _, exists := index[identity]; exists {
			return fail(errors.New("duplicate region/substance/period baseline"))
		}
		value, err := wfComputeNumber(wfReadField(item, fields["valueField"]), fields["valueField"])
		if err != nil {
			return fail(err)
		}
		index[identity] = value
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		baselineKey, err := key(item, fields["baselinePeriodField"])
		if err != nil {
			return fail(err)
		}
		baseline, exists := index[baselineKey]
		status := "ok"
		var result any
		if !exists {
			status = "missing_baseline"
		} else if baseline == 0 {
			status = "zero_baseline"
		} else {
			current, err := wfComputeNumber(wfReadField(item, fields["valueField"]), fields["valueField"])
			if err != nil {
				return fail(err)
			}
			difference := current - baseline
			if err := wfFiniteCompute(difference); err != nil {
				return fail(err)
			}
			percentage := difference / baseline * 100
			if err := wfFiniteCompute(percentage); err != nil {
				return fail(err)
			}
			result = wfRoundCompute(percentage)
		}
		cp := map[string]any{}
		for k, v := range item.(map[string]any) {
			cp[k] = v
		}
		cp[outKey] = result
		cp[statusKey] = status
		out = append(out, cp)
	}
	return wfEvalResult{outputItems: out, trueItems: out}
}

func wfEvalCondition(item any, comparator, fieldPath, compareValue string) (bool, string) {
	raw := wfReadField(item, fieldPath)
	actual := strings.TrimSpace(asString(raw))
	target := strings.TrimSpace(compareValue)
	switch comparator {
	case "is_empty":
		return actual == "" || actual == "[]" || actual == "null", ""
	case "not_empty":
		return actual != "" && actual != "[]" && actual != "null", ""
	case "contains", "":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(target)), ""
	case "not_contains":
		return !strings.Contains(strings.ToLower(actual), strings.ToLower(target)), ""
	case "equals":
		if na, errA := strconv.ParseFloat(actual, 64); errA == nil {
			if nb, errB := strconv.ParseFloat(target, 64); errB == nil {
				return na == nb, ""
			}
		}
		return actual == target, ""
	case "not_equals":
		m, _ := wfEvalCondition(item, "equals", fieldPath, compareValue)
		return !m, ""
	case "gt", "lt":
		if na, errA := strconv.ParseFloat(actual, 64); errA == nil {
			if nb, errB := strconv.ParseFloat(target, 64); errB == nil {
				if comparator == "gt" {
					return na > nb, ""
				}
				return na < nb, ""
			}
		}
		if comparator == "gt" {
			return actual > target, ""
		}
		return actual < target, ""
	case "regex":
		if target == "" {
			return actual != "", ""
		}
		re, err := regexp.Compile("(?i)" + target)
		if err != nil {
			return false, err.Error()
		}
		return re.MatchString(actual), ""
	default:
		return false, ""
	}
}

func wfAnalyticMissing(value any) bool {
	if value == nil {
		return true
	}
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) == ""
}

func wfAnalyticKeys(raw any, name string) ([]string, error) {
	var list []any
	switch value := raw.(type) {
	case []any:
		list = value
	case []string:
		for _, key := range value {
			list = append(list, key)
		}
	default:
		return nil, fmt.Errorf("%s must be a field array", name)
	}
	keys := []string{}
	seen := map[string]bool{}
	for _, value := range list {
		key, ok := value.(string)
		if !ok || strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate %s key", name)
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys, nil
}

func wfAnalyticIdentity(item any, keys []string, complete bool) (string, error) {
	values := []any{}
	for _, key := range keys {
		value := wfReadField(item, key)
		if complete && wfAnalyticMissing(value) {
			return "", nil
		}
		kind := "missing"
		switch v := value.(type) {
		case nil:
		case string:
			kind = "string"
		case bool:
			kind = "boolean"
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return "", errors.New("non-finite key")
			}
			if v == 0 {
				value = float64(0)
			}
			kind = "number"
		case int, int64:
			kind = "number"
		default:
			return "", errors.New("keys must be scalar")
		}
		values = append(values, []any{kind, value})
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

func wfCleanProvenance(value any) any {
	switch v := value.(type) {
	case []any:
		out := []any{}
		seen := map[string]bool{}
		for _, item := range v {
			if item == nil {
				continue
			}
			clean := wfCleanProvenance(item)
			encoded, _ := json.Marshal(clean)
			key := string(encoded)
			if !seen[key] {
				seen[key] = true
				out = append(out, clean)
			}
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			out[key] = wfCleanProvenance(child)
		}
		return out
	default:
		return value
	}
}

func wfMergeProvenanceValue(left, right any) any {
	left = wfCleanProvenance(left)
	right = wfCleanProvenance(right)
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	if string(a) == string(b) {
		return left
	}
	lm, lok := left.(map[string]any)
	rm, rok := right.(map[string]any)
	if lok && rok {
		out := map[string]any{}
		for k, v := range lm {
			out[k] = v
		}
		for k, v := range rm {
			out[k] = wfMergeProvenanceValue(out[k], v)
		}
		return out
	}
	values := []any{}
	if arr, ok := left.([]any); ok {
		values = append(values, arr...)
	} else {
		values = append(values, left)
	}
	if arr, ok := right.([]any); ok {
		values = append(values, arr...)
	} else {
		values = append(values, right)
	}
	out := []any{}
	seen := map[string]bool{}
	for _, v := range values {
		encoded, _ := json.Marshal(v)
		key := string(encoded)
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}

func wfMergeProvenance(out map[string]any, rows []any) {
	for _, item := range rows {
		if row, ok := item.(map[string]any); ok {
			for _, key := range []string{"citations", "evidence", "metadata"} {
				if value, exists := row[key]; exists && value != nil {
					out[key] = wfMergeProvenanceValue(out[key], value)
				}
			}
		}
	}
}

func wfEvalAggregate(items []any, logic map[string]any) wfEvalResult {
	fail := func(err error) wfEvalResult { return wfEvalResult{errText: err.Error()} }
	rawGroups, exists := logic["groupBy"]
	if !exists {
		rawGroups = []any{}
	}
	fields, err := wfAnalyticKeys(rawGroups, "groupBy")
	if err != nil {
		return fail(err)
	}
	aliases := map[string]bool{"citations": true, "evidence": true, "metadata": true}
	for _, key := range fields {
		if aliases[key] {
			return fail(errors.New("groupBy conflicts with provenance"))
		}
		aliases[key] = true
	}
	rawMetrics, exists := logic["metrics"]
	if !exists {
		rawMetrics = []any{map[string]any{"field": "", "fn": "count", "as": "count"}}
	}
	metrics, ok := rawMetrics.([]any)
	if !ok || len(metrics) == 0 {
		return fail(errors.New("metrics must be non-empty array"))
	}
	type metric struct{ field, fn, alias string }
	specs := []metric{}
	for _, raw := range metrics {
		m, ok := raw.(map[string]any)
		if !ok {
			return fail(errors.New("invalid metric"))
		}
		field, fok := m["field"].(string)
		fn, _ := m["fn"].(string)
		alias, aok := m["as"].(string)
		valid := fn == "count" || fn == "sum" || fn == "avg" || fn == "min" || fn == "max" || fn == "p95"
		if !valid || !fok || field != strings.TrimSpace(field) || (fn != "count" && field == "") || !aok || strings.TrimSpace(alias) == "" || alias != strings.TrimSpace(alias) || aliases[alias] {
			return fail(errors.New("invalid metric function, field or duplicate alias"))
		}
		aliases[alias] = true
		specs = append(specs, metric{field, fn, alias})
	}
	order := []string{}
	groups := map[string][]any{}
	if len(fields) == 0 {
		order = append(order, "[]")
		groups["[]"] = []any{}
	}
	for _, item := range items {
		key, err := wfAnalyticIdentity(item, fields, false)
		if err != nil {
			return fail(err)
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], item)
	}
	out := []any{}
	for _, key := range order {
		rows := groups[key]
		row := map[string]any{}
		for _, field := range fields {
			row[field] = wfReadField(rows[0], field)
		}
		for _, spec := range specs {
			count := 0
			nums := []float64{}
			for _, item := range rows {
				var raw any
				if spec.fn == "count" {
					count++
					continue
				}
				raw = wfReadField(item, spec.field)
				if wfAnalyticMissing(raw) {
					continue
				}
				count++
				if spec.fn != "count" {
					n, err := wfComputeNumber(raw, spec.field)
					if err != nil {
						return fail(err)
					}
					nums = append(nums, n)
				}
			}
			var value any
			if spec.fn == "count" {
				value = count
			} else if len(nums) > 0 {
				sort.Float64s(nums)
				n := 0.0
				switch spec.fn {
				case "min":
					n = nums[0]
				case "max":
					n = nums[len(nums)-1]
				case "p95":
					n = nums[int(math.Ceil(float64(len(nums))*0.95))-1]
				default:
					for _, v := range nums {
						n += v
						if math.IsNaN(n) || math.IsInf(n, 0) {
							return fail(errors.New("non-finite aggregate result"))
						}
					}
					if spec.fn == "avg" {
						n /= float64(len(nums))
					}
				}
				value = wfRoundCompute(n)
			}
			row[spec.alias] = value
		}
		wfMergeProvenance(row, rows)
		out = append(out, row)
	}
	return wfEvalResult{outputItems: out, trueItems: out}
}

func wfAnalyticPaths(record map[string]any, prefix string) []string {
	paths := []string{}
	for key, value := range record {
		path := prefix + key
		paths = append(paths, path)
		if child, ok := value.(map[string]any); ok {
			paths = append(paths, wfAnalyticPaths(child, path+".")...)
		}
	}
	return paths
}

func wfEvalJoin(a, b []any, logic map[string]any) wfEvalResult {
	fail := func(err error) wfEvalResult { return wfEvalResult{errText: err.Error()} }
	raw, exists := logic["joinKeys"]
	if !exists {
		raw = []any{map[string]any{"left": "id", "right": "id"}}
	}
	pairs, ok := raw.([]any)
	if !ok || len(pairs) == 0 {
		return fail(errors.New("joinKeys must be non-empty array"))
	}
	leftRaw, rightRaw := []any{}, []any{}
	for _, pair := range pairs {
		p, ok := pair.(map[string]any)
		if !ok {
			return fail(errors.New("invalid joinKeys"))
		}
		leftRaw = append(leftRaw, p["left"])
		rightRaw = append(rightRaw, p["right"])
	}
	leftKeys, err := wfAnalyticKeys(leftRaw, "joinKeys.left")
	if err != nil {
		return fail(err)
	}
	rightKeys, err := wfAnalyticKeys(rightRaw, "joinKeys.right")
	if err != nil {
		return fail(err)
	}
	mode := "inner"
	if raw, exists := logic["joinMode"]; exists {
		mode, ok = raw.(string)
		if !ok {
			return fail(errors.New("invalid joinMode"))
		}
	}
	if mode != "inner" && mode != "left" {
		return fail(errors.New("invalid joinMode"))
	}
	record := func(item any, side string) map[string]any {
		if m, ok := item.(map[string]any); ok {
			return m
		}
		return map[string]any{side + "Value": item}
	}
	leftFields, used, rightFields := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range a {
		for _, k := range wfAnalyticPaths(record(item, "left"), "") {
			leftFields[k] = true
			used[k] = true
		}
	}
	for _, item := range b {
		for k := range record(item, "right") {
			rightFields[k] = true
		}
		for _, k := range wfAnalyticPaths(record(item, "right"), "") {
			used[k] = true
		}
	}
	names := map[string]string{}
	ordered := []string{}
	for k := range rightFields {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	for _, k := range ordered {
		name := k
		if leftFields[k] {
			for {
				name = "right." + name
				if !used[name] {
					break
				}
			}
		}
		names[k] = name
		used[name] = true
	}
	index := map[string][]map[string]any{}
	for _, item := range b {
		key, err := wfAnalyticIdentity(item, rightKeys, true)
		if err != nil {
			return fail(err)
		}
		if key != "" {
			index[key] = append(index[key], record(item, "right"))
		}
	}
	out := []any{}
	for _, item := range a {
		key, err := wfAnalyticIdentity(item, leftKeys, true)
		if err != nil {
			return fail(err)
		}
		left := record(item, "left")
		matches := index[key]
		if len(matches) > 0 {
			for _, right := range matches {
				row := map[string]any{}
				for k, v := range left {
					row[k] = v
				}
				for k, v := range right {
					row[names[k]] = v
				}
				for _, k := range []string{"citations", "evidence", "metadata"} {
					delete(row, k)
				}
				wfMergeProvenance(row, []any{left, right})
				out = append(out, row)
			}
		} else if mode == "left" {
			row := map[string]any{}
			for k, v := range left {
				row[k] = v
			}
			out = append(out, row)
		}
	}
	return wfEvalResult{outputItems: out, trueItems: out}
}

func wfEvalSortLimit(items []any, logic map[string]any) wfEvalResult {
	fail := func(err error) wfEvalResult { return wfEvalResult{errText: err.Error()} }
	field := firstNonEmpty(asString(logic["sortField"]), asString(logic["fieldPath"]), "value")
	if strings.TrimSpace(field) == "" || field != strings.TrimSpace(field) {
		return fail(errors.New("invalid sortField"))
	}
	if raw, exists := logic["sortField"]; exists {
		if value, ok := raw.(string); !ok || value != strings.TrimSpace(value) {
			return fail(errors.New("invalid sortField"))
		}
	}
	direction := "desc"
	if raw, exists := logic["sortOrder"]; exists {
		var ok bool
		direction, ok = raw.(string)
		if !ok {
			return fail(errors.New("invalid sortOrder"))
		}
	}
	if direction != "asc" && direction != "desc" {
		return fail(errors.New("invalid sortOrder"))
	}
	limit := 10
	if raw, exists := logic["limit"]; exists {
		n, err := wfComputeNumber(raw, "limit")
		_, stringValue := raw.(string)
		if err != nil || stringValue || n <= 0 || math.Trunc(n) != n || n > 9007199254740991 {
			return fail(errors.New("invalid limit"))
		}
		limit = int(n)
	}
	raw, exists := logic["dedupeKeys"]
	if !exists {
		raw = []any{}
	}
	keys, err := wfAnalyticKeys(raw, "dedupeKeys")
	if err != nil {
		return fail(err)
	}
	type entry struct {
		item     any
		value    any
		num      float64
		numeric  bool
		label    string
		identity string
	}
	entries := []entry{}
	for _, item := range items {
		value := wfReadField(item, field)
		if _, ok := item.(map[string]any); !ok {
			value = item
		}
		e := entry{item: item, value: value}
		if !wfAnalyticMissing(value) {
			switch v := value.(type) {
			case string:
				e.label = "string:" + v
			case bool:
				e.label = "boolean:" + strconv.FormatBool(v)
			case float64:
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fail(errors.New("non-finite sort value"))
				}
			case int, int64:
			default:
				return fail(errors.New("sort values must be scalar"))
			}
			if n, err := wfComputeNumber(value, field); err == nil {
				e.num = n
				e.numeric = true
			}
		}
		if len(keys) > 0 {
			e.identity, err = wfAnalyticIdentity(item, keys, true)
			if err != nil {
				return fail(err)
			}
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		ma, mb := wfAnalyticMissing(a.value), wfAnalyticMissing(b.value)
		if ma || mb {
			return !ma && mb
		}
		cmp := 0
		if a.numeric && b.numeric {
			if a.num < b.num {
				cmp = -1
			} else if a.num > b.num {
				cmp = 1
			}
		} else if a.numeric != b.numeric {
			if a.numeric {
				cmp = -1
			} else {
				cmp = 1
			}
		} else {
			cmp = strings.Compare(a.label, b.label)
		}
		if direction == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	out := []any{}
	seen := map[string]int{}
	for _, e := range entries {
		if previous, exists := seen[e.identity]; e.identity != "" && exists {
			if winner, ok := out[previous].(map[string]any); ok {
				merged := map[string]any{}
				for key, value := range winner {
					merged[key] = value
				}
				wfMergeProvenance(merged, []any{e.item})
				out[previous] = merged
			}
			continue
		}
		if e.identity != "" {
			seen[e.identity] = len(out)
		}
		out = append(out, e.item)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return wfEvalResult{outputItems: out, trueItems: out}
}

func wfEvalLogicNode(node map[string]any, inputA, inputB []any) wfEvalResult {
	// Read-only metadata access (see wfLogicOperator): evaluation must not
	// mutate the graph document it was handed.
	meta, _ := node["metadata"].(map[string]any)
	logic, _ := meta["logic"].(map[string]any)
	op := wfLogicOperator(node)
	if raw, exists := logic["operator"]; exists {
		value, ok := raw.(string)
		if !ok {
			return wfEvalResult{errText: "invalid operator"}
		}
		switch value {
		case "if_else", "intersect", "union", "difference", "filter", "json_extract", "compute", "aggregate", "join", "sort_limit":
		default:
			return wfEvalResult{errText: "unknown operator"}
		}
	}
	if raw, exists := logic["limit"]; op == "sort_limit" && exists {
		var n float64
		switch value := raw.(type) {
		case float64:
			n = value
		case int:
			n = float64(value)
		case int64:
			n = float64(value)
		default:
			return wfEvalResult{errText: "sort_limit limit must be a positive integer"}
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || math.Trunc(n) != n || n > 9007199254740991 {
			return wfEvalResult{errText: "sort_limit limit must be a positive integer"}
		}
	}
	fieldPath := strings.TrimSpace(asString(logic["fieldPath"]))
	comparator := strings.TrimSpace(asString(logic["comparator"]))
	compareValue := strings.TrimSpace(asString(logic["compareValue"]))
	matchKey := strings.TrimSpace(asString(logic["matchKey"]))
	allInputs := append(append([]any{}, inputA...), inputB...)

	switch op {
	case "intersect", "union", "difference":
		keysB := map[string]bool{}
		for _, b := range inputB {
			if k := wfItemIdentity(b, matchKey); k != "" {
				keysB[k] = true
			}
		}
		seen := map[string]bool{}
		var out []any
		if op == "intersect" {
			for _, a := range inputA {
				k := wfItemIdentity(a, matchKey)
				if k != "" && keysB[k] && !seen[k] {
					seen[k] = true
					out = append(out, a)
				}
			}
		} else if op == "difference" {
			for _, a := range inputA {
				k := wfItemIdentity(a, matchKey)
				if k != "" && !keysB[k] && !seen[k] {
					seen[k] = true
					out = append(out, a)
				}
			}
		} else {
			for _, item := range allInputs {
				k := wfItemIdentity(item, matchKey)
				if k != "" && !seen[k] {
					seen[k] = true
					out = append(out, item)
				}
			}
		}
		return wfEvalResult{outputItems: out, trueItems: out}

	case "join":
		return wfEvalJoin(inputA, inputB, logic)

	case "compute":
		outKey := firstNonEmpty(strings.TrimSpace(asString(logic["outputField"])), "computed_value")
		for _, item := range allInputs {
			if record, ok := item.(map[string]any); ok {
				if _, exists := record[outKey]; exists {
					return wfEvalResult{errText: "duplicate compute outputField: " + outKey}
				}
			} else if outKey == "value" {
				return wfEvalResult{errText: "duplicate compute outputField: value"}
			}
		}
		if comparison, exists := logic["periodComparison"]; exists {
			return wfEvalPeriodComparison(allInputs, logic, comparison, outKey)
		}
		expr := firstNonEmpty(strings.TrimSpace(asString(logic["expression"])), fieldPath)
		if expr == "" {
			return wfEvalResult{errText: "compute expression must be explicit"}
		}
		var out []any
		for _, item := range allInputs {
			val, errStr := wfEvalArithmetic(item, expr)
			if errStr != "" {
				return wfEvalResult{errText: errStr}
			}
			cp := map[string]any{}
			if m, ok := item.(map[string]any); ok {
				for k, v := range m {
					cp[k] = v
				}
			} else {
				cp["value"] = item
			}
			cp[outKey] = val
			out = append(out, cp)
		}
		return wfEvalResult{outputItems: out, trueItems: out}

	case "aggregate":
		return wfEvalAggregate(allInputs, logic)

	case "sort_limit":
		return wfEvalSortLimit(allInputs, logic)

	case "if_else":
		var tItems, fItems []any
		var firstErr string
		for _, item := range allInputs {
			matched, errStr := wfEvalCondition(item, comparator, fieldPath, compareValue)
			if errStr != "" && firstErr == "" {
				firstErr = errStr
			}
			if matched {
				tItems = append(tItems, item)
			} else {
				fItems = append(fItems, item)
			}
		}
		return wfEvalResult{outputItems: tItems, trueItems: tItems, falseItems: fItems, errText: firstErr}

	case "filter":
		var out []any
		var firstErr string
		for _, item := range allInputs {
			matched, errStr := wfEvalCondition(item, comparator, fieldPath, compareValue)
			if errStr != "" && firstErr == "" {
				firstErr = errStr
			}
			if matched {
				out = append(out, item)
			}
		}
		return wfEvalResult{outputItems: out, trueItems: out, errText: firstErr}

	default: // json_extract
		var out []any
		for _, item := range allInputs {
			val := wfReadField(item, fieldPath)
			if val == nil || asString(val) == "" {
				continue
			}
			if arr, ok := val.([]any); ok {
				out = append(out, arr...)
			} else {
				out = append(out, val)
			}
		}
		return wfEvalResult{outputItems: out, trueItems: out}
	}
}

func wfFormatSkillNode(node, meta map[string]any, inputA []any) string {
	skillName := strings.TrimSpace(asString(meta["skillName"]))
	skillDesc := strings.TrimSpace(asString(meta["skillDescription"]))
	content := strings.TrimSpace(asString(meta["content"]))
	upText := wfFormatItems(inputA)
	var parts []string
	if skillName != "" {
		parts = append(parts, fmt.Sprintf("【技能：%s】", skillName))
	} else if t := strings.TrimSpace(asString(node["title"])); t != "" {
		parts = append(parts, fmt.Sprintf("【技能：%s】", t))
	}
	if skillDesc != "" {
		parts = append(parts, "用途："+skillDesc)
	}
	if content != "" {
		parts = append(parts, "执行模板：\n"+content)
	}
	if upText != "" {
		parts = append(parts, "【上游输入数据】\n"+upText)
	}
	return strings.Join(parts, "\n\n")
}

func wfFormatItems(items []any) string {
	if len(items) == 0 {
		return ""
	}
	allScalar := true
	for _, item := range items {
		switch item.(type) {
		case map[string]any, []any:
			allScalar = false
		}
	}
	if allScalar {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, asString(item))
		}
		return strings.Join(lines, "\n")
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// REST execution is registered by the application admission/approval adapter.
func registerCanvasWorkflowRoutes(_ *gin.RouterGroup, _ *gorm.DB, _ func(*gin.Context) (Scope, *model.User, bool)) {
}
