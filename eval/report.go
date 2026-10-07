package taxeval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ResultRecord is one graded task outcome written to results/<model>-<ts>.jsonl.
type ResultRecord struct {
	ID             string    `json:"id"`
	FactKey        string    `json:"fact_key"`
	Template       string    `json:"template"`
	TaskType       string    `json:"task_type"`
	AnswerType     string    `json:"answer_type"`
	Question       string    `json:"question"`
	Gold           string    `json:"gold"`
	Model          string    `json:"model"`
	Backend        string    `json:"backend"`
	RawResponse    string    `json:"raw_response"`
	ExtractedGold  string    `json:"extracted_gold"`
	ExtractedModel string    `json:"extracted_model"`
	Missing        []string  `json:"missing"`
	Pass           bool      `json:"pass"`
	ScoreParts     float64   `json:"score_parts"`
	Graded         bool      `json:"graded"`
	Error          string    `json:"error"`
	LatencyMS      int64     `json:"latency_ms"`
	Usage          UsageInfo `json:"usage"`
}

// UsageInfo captures token usage where the backend reports it.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ResultWriter appends ResultRecords to a JSONL file.
type ResultWriter struct {
	f *os.File
	w *bufio.Writer
}

// NewResultWriter creates/truncates the output file.
func NewResultWriter(path string) (*ResultWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &ResultWriter{f: f, w: bufio.NewWriter(f)}, nil
}

// Write one record as a JSONL line.
func (rw *ResultWriter) Write(r ResultRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := rw.w.Write(b); err != nil {
		return err
	}
	return rw.w.WriteByte('\n')
}

// Close flushes and closes.
func (rw *ResultWriter) Close() error {
	if err := rw.w.Flush(); err != nil {
		rw.f.Close()
		return err
	}
	return rw.f.Close()
}

// ---------- aggregation ----------

type typeStat struct {
	n      int
	passed int
}

// BuildReport scans result files matching prefix and renders the baseline table.
func BuildReport(dir, outFile string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// model -> stats
	models := map[string]*modelStats{}
	var order []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
		for sc.Scan() {
			var r ResultRecord
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				continue
			}
			ms, ok := models[r.Model]
			if !ok {
				ms = &modelStats{
					backend: r.Backend,
					byAT:    map[string]*typeStat{},
					byTT:    map[string]*typeStat{},
					seenIDs: map[string]bool{},
				}
				models[r.Model] = ms
				order = append(order, r.Model)
			}
			// A model may have been run in more than one file (resume /
			// retry legs): keep the first response per task id, skip the rest.
			if ms.seenIDs[r.ID] {
				continue
			}
			ms.seenIDs[r.ID] = true
			ms.nTotal++
			ms.tokens += r.Usage.PromptTokens + r.Usage.CompletionTokens
			if r.Error != "" {
				ms.nErrors++
				continue
			}
			if !r.Graded {
				continue // rubric_pending
			}
			ms.nGraded++
			ts := func(m map[string]*typeStat, k string) *typeStat {
				if m[k] == nil {
					m[k] = &typeStat{}
				}
				return m[k]
			}
			if r.Pass {
				ms.nPass++
				ts(ms.byAT, r.AnswerType).passed++
				ts(ms.byTT, r.TaskType).passed++
			}
			ts(ms.byAT, r.AnswerType).n++
			ts(ms.byTT, r.TaskType).n++
		}
		f.Close()
	}

	sort.Strings(order)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Baseline scores — Tax-Model locked test set (%d tasks)\n\n", taskCount(dir)))
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04")))
	b.WriteString("Strict accuracy = all required entities present (multi_part: all parts). ")
	b.WriteString("Errors (API failures after retries) count as failures in acc_all. ")
	b.WriteString("`reasoned` tasks (rubric pending) are excluded from both denominators.\n\n")
	b.WriteString("| Model | Backend | Graded | Errors | acc (graded) | acc (all) | Tokens |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, m := range order {
		ms := models[m]
		accG := pct(ms.nPass, ms.nGraded)
		accAll := pct(ms.nPass, ms.nGraded+ms.nErrors)
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %s | %s | %d |\n",
			m, ms.backend, ms.nGraded, ms.nErrors, accG, accAll, ms.tokens))
	}

	b.WriteString("\n## Accuracy by answer_type\n\n")
	b.WriteString("| Model | " + strings.Join(atOrder, " | ") + " |\n")
	b.WriteString("|---|" + strings.Repeat("---|", len(atOrder)) + "\n")
	for _, m := range order {
		ms := models[m]
		cells := make([]string, len(atOrder))
		for i, at := range atOrder {
			if ts := ms.byAT[at]; ts != nil && ts.n > 0 {
				cells[i] = fmt.Sprintf("%d%% (%d/%d)", 100*ts.passed/ts.n, ts.passed, ts.n)
			} else {
				cells[i] = "—"
			}
		}
		b.WriteString("| " + m + " | " + strings.Join(cells, " | ") + " |\n")
	}

	b.WriteString("\n## Accuracy by task_type\n\n")
	ttList := collectKeys(models, order, func(ms *modelStats) map[string]*typeStat { return ms.byTT })
	b.WriteString("| Model | " + strings.Join(ttList, " | ") + " |\n")
	b.WriteString("|---|" + strings.Repeat("---|", len(ttList)) + "\n")
	for _, m := range order {
		ms := models[m]
		cells := make([]string, len(ttList))
		for i, tt := range ttList {
			if ts := ms.byTT[tt]; ts != nil && ts.n > 0 {
				cells[i] = fmt.Sprintf("%d%% (%d/%d)", 100*ts.passed/ts.n, ts.passed, ts.n)
			} else {
				cells[i] = "—"
			}
		}
		b.WriteString("| " + m + " | " + strings.Join(cells, " | ") + " |\n")
	}

	if outFile == "" {
		fmt.Print(b.String())
		return nil
	}
	return os.WriteFile(outFile, []byte(b.String()), 0o644)
}

var atOrder = []string{
	"code", "section", "section_and_code", "nature_and_section", "form",
	"rate", "threshold", "amount", "date", "days", "date_and_pct",
	"act_selection", "act_and_section", "form_and_date", "yes_no_reasoned",
	"multi_part",
}

// modelStats accumulates one model's outcomes across result files.
type modelStats struct {
	backend string
	nTotal  int
	nGraded int
	nErrors int
	nPass   int
	byAT    map[string]*typeStat
	byTT    map[string]*typeStat
	tokens  int
	seenIDs map[string]bool // task ids already counted (resume dedupe)
}

func collectKeys(models map[string]*modelStats, order []string, sel func(*modelStats) map[string]*typeStat) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range order {
		for k := range sel(models[m]) {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func pct(n, d int) string {
	if d == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}

// LoadAnsweredIDs reads a prior partial result file and returns the set of
// task ids that already have a model response (raw_response != ""). Records
// with empty raw_response are error records — retry them on resume.
func LoadAnsweredIDs(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		var r ResultRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue // torn/partial line from a killed writer
		}
		if r.ID != "" && r.RawResponse != "" {
			out[r.ID] = true
		}
	}
	return out, sc.Err()
}

// VerifyResultsIntegrity is the post-run gate for review fix #1: the file must
// hold exactly `expected` records, every line must parse as JSON, every id
// must be non-empty and unique. Any torn line (the run1 corruption mode) or
// lost task fails the run instead of silently skewing the baseline.
func VerifyResultsIntegrity(path string, expected int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	ids := map[string]bool{}
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(line) == 0 {
			return fmt.Errorf("line %d: empty", lineNo)
		}
		var r ResultRecord
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("line %d: unparseable JSON (torn write?): %v", lineNo, err)
		}
		if r.ID == "" {
			return fmt.Errorf("line %d: record has empty id", lineNo)
		}
		if ids[r.ID] {
			return fmt.Errorf("line %d: duplicate id %s", lineNo, r.ID)
		}
		ids[r.ID] = true
		n++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if n != expected {
		return fmt.Errorf("record count %d != dispatched tasks %d (lost records)", n, expected)
	}
	return nil
}

// taskCount is the distinct task-id count across all result files (the test-set
// size the sweep covered).
func taskCount(dir string) int {
	seen := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
		for sc.Scan() {
			var r ResultRecord
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID != "" {
				seen[r.ID] = true
			}
		}
		f.Close()
	}
	return len(seen)
}
