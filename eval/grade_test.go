package taxeval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

const testSplitPath = "../data/splits/test.jsonl"

func loadTest(t *testing.T) []Task {
	t.Helper()
	tasks, err := LoadTasks(testSplitPath)
	if err != nil {
		t.Fatalf("load test split: %v", err)
	}
	// The P1.5 gap-closing card may regenerate the splits concurrently; the
	// count is whatever the current file holds. Lock verification (elsewhere)
	// guards integrity; here we only require a non-empty split.
	if len(tasks) == 0 {
		t.Fatalf("test split is empty")
	}
	return tasks
}

// TestGoldSelfExtraction: grading gold against itself must PASS for every
// non-reasoned task, and gold entity extraction must never come up empty.
func TestGoldSelfExtraction(t *testing.T) {
	tasks := loadTest(t)
	fails := 0
	nonNumericSection := 0
	for _, task := range tasks {
		g := Grade(task, task.Answer)
		if task.AnswerType == "reasoned" {
			if g.Graded {
				t.Errorf("reasoned task %s unexpectedly graded", task.ID)
			}
			continue
		}
		// Four section-map golds carry no numeric entity at all ("-",
		// "(credits preserved)", "(old Act continues)"); the entity grader
		// legitimately cannot score them and the report excludes them.
		if !g.Graded && strings.Contains(g.Reason, "no section ref in gold") && !strings.ContainsAny(task.Answer, "0123456789") {
			nonNumericSection++
			continue
		}
		if !g.Graded {
			t.Errorf("task %s (%s) ungradable: %s", task.ID, task.AnswerType, g.Reason)
			fails++
			continue
		}
		if !g.Pass {
			fails++
			if fails <= 10 {
				t.Errorf("task %s (%s): gold does not self-pass\n  gold=%q\n  missing=%v\n  extract_gold=%s",
					task.ID, task.AnswerType, task.Answer, g.MissingGold, g.ExtractGold)
			}
		}
	}
	if fails > 0 {
		t.Errorf("%d tasks failed self-extraction", fails)
	}
	t.Logf("self-extraction: %d tasks OK, %d non-numeric section golds excluded", len(tasks)-fails-nonNumericSection, nonNumericSection)
}

// TestLockVerification checks VerifyLock both accepts a lock matching the
// current file and rejects a tampered one.
func TestLockVerification(t *testing.T) {
	tasks, err := LoadTasks(testSplitPath)
	if err != nil {
		t.Fatal(err)
	}
	// Build a lock matching the CURRENT file (the concurrent P1.5 card may
	// regenerate data mid-run; we test the mechanism, not a fixed sha).
	qs := make([]string, len(tasks))
	for i, task := range tasks {
		qs[i] = task.Question
	}
	sortStrings(qs)
	matching := t.TempDir() + "/matching.locked.json"
	writeLock(t, matching, shaHex(strings.Join(qs, "\n")))
	if _, err := VerifyLock(testSplitPath, matching); err != nil {
		t.Fatalf("matching lock should verify: %v", err)
	}

	tmp := t.TempDir()
	bad := tmp + "/bad.locked.json"
	os.WriteFile(bad, []byte(`{"sha256_questions":"deadbeef"}`), 0o644)
	_, err = VerifyLock(testSplitPath, bad)
	if err == nil {
		t.Fatal("tampered lock should fail")
	}
	if !strings.Contains(err.Error(), "LOCK MISMATCH") {
		t.Fatalf("expected LOCK MISMATCH, got: %v", err)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func shaHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeLock(t *testing.T, path, sha string) {
	t.Helper()
	body := fmt.Sprintf(`{"locked_at":"test","n_test":0,"sha256_questions":%q,"policy":"test"}`, sha)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMutatedGoldFails: mutate one required entity in the gold answer; the
// grader must FAIL. Samples tasks deterministically across all answer types.
func TestMutatedGoldFails(t *testing.T) {
	tasks := loadTest(t)

	// deterministic sample: first task of each answer_type + every 37th task
	seen := map[string]bool{}
	var sample []Task
	for _, task := range tasks {
		if !seen[task.AnswerType] {
			seen[task.AnswerType] = true
			sample = append(sample, task)
		}
	}
	for i := 0; i < len(tasks); i += 37 {
		sample = append(sample, tasks[i])
	}

	skipped := 0
	for _, task := range sample {
		if task.AnswerType == "reasoned" {
			continue
		}
		mut := mutateGold(task.Answer, task.AnswerType)
		if mut == task.Answer {
			skipped++ // no entity available to mutate for this answer
			continue
		}
		mutTask := task
		mutTask.Answer = mut
		g := Grade(mutTask, task.Answer) // model says the TRUE answer, gold mutated
		if g.Graded && g.Pass {
			t.Errorf("task %s (%s): mutated gold %q wrongly PASSED against true answer %q",
				task.ID, task.AnswerType, mut, task.Answer)
		}
	}
	t.Logf("mutation test: %d samples, %d skipped (no mutable entity)", len(sample), skipped)
}

// mutateGold swaps one entity deterministically. Returns the input unchanged
// if nothing mutable is found.
func mutateGold(gold, at string) string {
	switch at {
	case "code":
		return regexp.MustCompile(`\b10\d{2}\b`).ReplaceAllStringFunc(gold, func(m string) string {
			// 1001 -> 1002 etc., never equal to itself
			n := int(m[3]-'0') + 1
			if n > 9 {
				n = 0
			}
			return m[:3] + string(rune('0'+n))
		})
	case "rate", "date_and_pct":
		return regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`).ReplaceAllStringFunc(gold, func(m string) string {
			// bump the first number by 1 (10% -> 11%, 20% -> 21%)
			var n int
			fmt.Sscanf(m, "%d", &n)
			return fmt.Sprintf("%d%%", n+1)
		})
	case "amount", "threshold", "yes_no_reasoned":
		return regexp.MustCompile(`Rs\.?\s*([\d,]+)`).ReplaceAllStringFunc(gold, func(m string) string {
			digits := regexp.MustCompile(`\d`).ReplaceAllStringFunc(m, func(d string) string {
				n := int(d[0] - '0')
				return fmt.Sprintf("%d", (n+1)%10)
			})
			return digits
		})
	case "date", "form_and_date":
		iso := regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
		if iso.MatchString(gold) {
			return iso.ReplaceAllStringFunc(gold, func(m string) string {
				var y, mo, d int
				fmt.Sscanf(m, "%d-%d-%d", &y, &mo, &d)
				return fmt.Sprintf("%04d-%02d-%02d", y, mo, d+1) // +1 day
			})
		}
		return bumpDayWord(gold)
	case "days":
		return regexp.MustCompile(`(\d+)\s*days?\b`).ReplaceAllStringFunc(gold, func(m string) string {
			var n int
			fmt.Sscanf(m, "%d", &n)
			return fmt.Sprintf("%d days", n+1)
		})
	case "act_selection", "act_and_section":
		if strings.Contains(gold, "2025") {
			return strings.Replace(gold, "2025", "1961", 1)
		}
		if strings.Contains(gold, "1961") {
			return strings.Replace(gold, "1961", "2025", 1)
		}
		return gold
	case "form":
		return regexp.MustCompile(`\b(\d{1,3})[A-Z]?\b`).ReplaceAllStringFunc(gold, func(m string) string {
			var n int
			fmt.Sscanf(m, "%d", &n)
			return fmt.Sprintf("%d", n+1)
		})
	case "section", "nature_and_section", "section_and_code":
		return regexp.MustCompile(`\b(\d{1,3}[A-Z]?)\b`).ReplaceAllStringFunc(gold, func(m string) string {
			var n int
			fmt.Sscanf(m, "%d", &n)
			return fmt.Sprintf("%d", n+1)
		})
	case "multi_part":
		// mutate the first code found (part (i) is always the payment code)
		if regexp.MustCompile(`\b10\d{2}\b`).MatchString(gold) {
			return mutateGold(gold, "code")
		}
		return mutateGold(gold, "date")
	default:
		return gold
	}
}

// bumpDayWord shifts a long-form date day number by +1 ("7 June" -> "8 June").
func bumpDayWord(s string) string {
	re := regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]+)\s+(\d{4})\b`)
	if re.MatchString(s) {
		return re.ReplaceAllStringFunc(s, func(m string) string {
			var d int
			var mon, y string
			fmt.Sscanf(m, "%d %s %s", &d, &mon, &y)
			return fmt.Sprintf("%d %s %s", d+1, mon, strings.TrimSuffix(y, ","))
		})
	}
	return s
}
