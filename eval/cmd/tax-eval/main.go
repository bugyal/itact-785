package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	taxeval "taxeval"
)

// backend describes one endpoint preset.
type backend struct {
	BaseURL       string
	APIKeyEnv     string
	Claude        bool // local claude CLI subprocess
	Codex         bool // local codex CLI subprocess
	Anthropic     bool // Anthropic /v1/messages wire protocol
	AnthropicBase string
}

var backends = map[string]backend{
	"zai":        {BaseURL: "https://api.z.ai/api/paas/v4", APIKeyEnv: "ZAI_API_KEY"},
	"zai-a":      {Anthropic: true, AnthropicBase: "https://api.z.ai/api/anthropic/v1", APIKeyEnv: "ZAI_API_KEY"},
	"openrouter": {BaseURL: "https://openrouter.ai/api/v1", APIKeyEnv: "OPENROUTER_API_KEY"},
	"ollama":     {BaseURL: "https://ollama.com/v1", APIKeyEnv: "OLLAMA_API_KEY"},
	"sarvam":     {BaseURL: "https://api.sarvam.ai/v1", APIKeyEnv: "SARVAM_API_KEY"},
	"claude":     {Claude: true},
	"codex":      {Codex: true},
	"custom":     {}, // requires -base-url and -api-key-env
}

const promptPreamble = "Answer the following Indian tax compliance question precisely and concisely.\n\n"

func main() {
	var (
		split       = flag.String("split", "test", "split name under data/splits/")
		repoRoot    = flag.String("repo", "..", "tax-model repo root (holds data/)")
		backendName = flag.String("backend", "", "zai|ollama|sarvam|claude|custom")
		model       = flag.String("model", "", "model id sent to the endpoint")
		baseURL     = flag.String("base-url", "", "override OpenAI-compatible base URL")
		apiKeyEnv   = flag.String("api-key-env", "", "env var holding the bearer key")
		concurrency = flag.Int("concurrency", 4, "parallel requests")
		limit       = flag.Int("limit", 0, "only first N tasks (0 = all)")
		shuffle     = flag.Bool("shuffle", false, "deterministically shuffle tasks (seed 42) before limit — use with -limit for a uniform random sample")
		maxTokens   = flag.Int("max-tokens", 1024, "max completion tokens")
		timeoutS    = flag.Int("timeout", 120, "per-request timeout seconds")
		outDir      = flag.String("out", "results", "output directory for JSONL + reports")
		report      = flag.String("report", "", "aggregate all results in -out into this md file and exit")
		allowUnlock = flag.Bool("allow-unlocked", false, "run even if test-set sha256 does not match the lock")
		resumeFrom  = flag.String("resume-from", "", "jsonl from a prior partial run of the SAME model: skip tasks that already have a model response")
	)
	flag.Parse()

	if *report != "" {
		if err := taxeval.BuildReport(*outDir, *report); err != nil {
			fatal(err)
		}
		fmt.Println("report written:", *report)
		return
	}

	if *backendName == "" {
		fatal(fmt.Errorf("-backend required (zai|ollama|sarvam|claude|custom)"))
	}
	b, ok := backends[*backendName]
	if !ok {
		fatal(fmt.Errorf("unknown backend %q", *backendName))
	}
	if *baseURL != "" {
		b.BaseURL = *baseURL // allow overriding any backend's endpoint (e.g. local ollama for the P3 base baseline)
	}
	if b.BaseURL == "" && !b.Claude && !b.Codex && !b.Anthropic {
		fatal(fmt.Errorf("backend %q needs -base-url", *backendName))
	}
	if *model == "" && !b.Claude && !b.Codex {
		fatal(fmt.Errorf("-model required"))
	}

	// ---- load + lock check ----
	splitPath := filepath.Join(*repoRoot, "data", "splits", *split+".jsonl")
	lockPath := filepath.Join(*repoRoot, "data", "splits", "test.locked.json")
	done := map[string]bool{}
	if *split != "test" {
		fmt.Println("NOTE: non-test split; skipping lock verification")
	} else {
		actual, err := taxeval.VerifyLock(splitPath, lockPath)
		if err != nil && !*allowUnlock {
			fatal(fmt.Errorf("lock check: %w (actual=%s; pass -allow-unlocked to override)", err, actual))
		}
		fmt.Println("lock OK:", actual[:16])
	}

	// ---- resume: collect ids that already have a model response ----
	if *resumeFrom != "" {
		for _, path := range strings.Split(*resumeFrom, ",") {
			answered, lerr := taxeval.LoadAnsweredIDs(strings.TrimSpace(path))
			fatalIf(lerr)
			for id := range answered {
				done[id] = true
			}
			fmt.Printf("resume: %d tasks already answered in %s (total unique: %d)\n",
				len(answered), filepath.Base(strings.TrimSpace(path)), len(done))
		}
	}

	tasks, err := taxeval.LoadTasks(splitPath)
	fatalIf(err)
	if *shuffle {
		taxeval.ShuffleStable(tasks, 42)
	}
	if *limit > 0 && *limit < len(tasks) {
		tasks = tasks[:*limit]
	}
	fmt.Printf("loaded %d tasks from %s\n", len(tasks), splitPath)

	// ---- resolve API key ----
	var apiKey string
	if !b.Claude && !b.Codex {
		apiKey = os.Getenv(b.APIKeyEnv)
		if apiKey == "" && *apiKeyEnv != "" {
			apiKey = os.Getenv(*apiKeyEnv)
		}
		// Local endpoints (127.0.0.1/localhost) don't need a key; ollama and
		// llama.cpp ignore the Authorization header entirely.
		if apiKey == "" && !strings.Contains(b.BaseURL, "127.0.0.1") && !strings.Contains(b.BaseURL, "localhost") {
			fatal(fmt.Errorf("API key env %s is empty", b.APIKeyEnv))
		}
		if apiKey == "" {
			apiKey = "local"
		}
	}

	// ---- output file ----
	tag := *backendName
	if *model != "" {
		tag = *model
	}
	tag = strings.NewReplacer("/", "_", ":", "_").Replace(tag)
	outPath := filepath.Join(*outDir, fmt.Sprintf("%s-%s.jsonl", tag, time.Now().Format("20060102-150405")))
	w, err := taxeval.NewResultWriter(outPath)
	fatalIf(err)
	// Closed explicitly after wg.Wait() (before the integrity gate); the
	// deferred Close is a no-op safety net for early fatal paths.
	defer w.Close()

	// ---- run ----
	ctx := context.Background()
	jobs := make(chan taxeval.Task)
	recs := make(chan taxeval.ResultRecord) // single-writer channel (review fix #1)
	var wg sync.WaitGroup
	var mu sync.Mutex
	nDone, nPass, nErr := 0, 0, 0
	start := time.Now()

	// writer goroutine: the ONLY writer to the results file. Concurrent
	// bufio writes from workers tore lines in the run1 sweep (2 corrupted
	// records + 1 lost task id), so all writes now funnel through this
	// goroutine; the first write error aborts the run.
	writeErr := make(chan error, 1)
	go func() {
		for rec := range recs {
			if err := w.Write(rec); err != nil {
				writeErr <- err
				return
			}
		}
		writeErr <- nil
	}()

	workers := *concurrency
	if (b.Claude || b.Codex) && workers > 4 {
		workers = 4
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				var rec taxeval.ResultRecord
				rec.ID = t.ID
				rec.FactKey = t.FactKey
				rec.Template = t.Template
				rec.TaskType = t.TaskType
				rec.AnswerType = t.AnswerType
				rec.Question = t.Question
				rec.Gold = t.Answer
				rec.Model = *model
				rec.Backend = *backendName

				t0 := time.Now()
				var content string
				var callErr error
				switch {
				case b.Claude:
					var resp taxeval.Response
					resp, callErr = taxeval.CallClaude(ctx, t.Question, *model, 300)
					if callErr == nil {
						content = resp.Content
					}
				case b.Codex:
					var resp taxeval.Response
					resp, callErr = taxeval.CallCodex(ctx, t.Question, *model, *timeoutS)
					if callErr == nil {
						content = resp.Content
					}
				case b.Anthropic:
					var resp taxeval.Response
					resp, callErr = taxeval.CallAnthropic(ctx, taxeval.Request{
						BaseURL:        b.AnthropicBase,
						APIKey:         apiKey,
						Model:          *model,
						Question:       t.Question,
						MaxTokens:      *maxTokens,
						TimeoutS:       *timeoutS,
						PromptPreamble: promptPreamble,
					})
					if callErr == nil {
						content = resp.Content
						rec.Usage = taxeval.UsageInfo{PromptTokens: resp.PromptTokens, CompletionTokens: resp.CompletionTokens}
					}
				default:
					resp, err := taxeval.CallOpenAI(ctx, taxeval.Request{
						BaseURL:        b.BaseURL,
						APIKey:         apiKey,
						Model:          *model,
						Question:       t.Question,
						MaxTokens:      *maxTokens,
						TimeoutS:       *timeoutS,
						PromptPreamble: promptPreamble,
					})
					callErr = err
					if err == nil {
						content = resp.Content
						rec.Usage = taxeval.UsageInfo{PromptTokens: resp.PromptTokens, CompletionTokens: resp.CompletionTokens}
					}
				}
				rec.LatencyMS = time.Since(t0).Milliseconds()

				if callErr != nil {
					rec.Error = callErr.Error()
					rec.Pass = false
					rec.Graded = false
					mu.Lock()
					nErr++
					mu.Unlock()
				} else {
					rec.RawResponse = content
					g := taxeval.Grade(t, content)
					rec.Pass = g.Pass
					rec.ScoreParts = g.ScoreParts
					rec.Graded = g.Graded
					rec.Missing = g.MissingGold
					rec.ExtractedGold = g.ExtractGold
					rec.ExtractedModel = g.ExtractModel
					if g.Reason != "" {
						rec.Error = g.Reason
					}
					mu.Lock()
					if g.Graded && g.Pass {
						nPass++
					}
					mu.Unlock()
				}
				// Single-writer channel (review fix #1): the only goroutine
				// touching the bufio.Writer is the dedicated writer above.
				recs <- rec
				mu.Lock()
				nDone++
				done := nDone
				pass, errs := nPass, nErr
				mu.Unlock()
				if done%25 == 0 || done == len(tasks) {
					fmt.Printf("[%s] %d/%d done, %d pass, %d err, elapsed %s\n",
						tag, done, len(tasks), pass, errs, time.Since(start).Round(time.Second))
				}
			}
		}()
	}
	for _, t := range tasks {
		if done[t.ID] {
			continue
		}
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	close(recs)
	if werr := <-writeErr; werr != nil {
		fatal(fmt.Errorf("results writer failed: %w", werr))
	}
	// Flush + close BEFORE the integrity gate: the gate reads the file from
	// disk, and the tail of the run is still in the bufio buffer until Close.
	if err := w.Close(); err != nil {
		fatal(fmt.Errorf("close results file: %w", err))
	}

	// ---- results-integrity gate (review fix #1) ----
	// Every dispatched task must have exactly one parseable record; torn or
	// missing lines fail the run instead of silently skewing the baseline.
	expected := len(tasks) - len(done)
	if werr := taxeval.VerifyResultsIntegrity(outPath, expected); werr != nil {
		fatal(fmt.Errorf("RESULTS INTEGRITY FAILED: %w", werr))
	}
	fmt.Printf("integrity OK: %d records, all parseable, ids unique\n", expected)

	fmt.Printf("\nDONE %s: %d tasks, %d pass, %d errors -> %s\n", tag, len(tasks), nPass, nErr, outPath)

	// print a compact summary
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "n_total\t%d\nn_pass\t%d\nn_err\t%d\nout\t%s\n", len(tasks), nPass, nErr, outPath)
	tw.Flush()
}

func fatalIf(err error) {
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
