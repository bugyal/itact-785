package taxeval

import (
	"fmt"
	"regexp"
	"strings"
)

// GradeResult is the outcome of grading one model answer against gold.
type GradeResult struct {
	Pass         bool     // strict pass (multi_part: all parts)
	ScoreParts   float64  // fraction of parts passed (1.0 for single-part types)
	Graded       bool     // false => rubric_pending or ungradable; excluded from accuracy
	Reason       string   // why not graded, or ""
	MissingGold  []string // human-readable description of required entities not found
	ExtractGold  string   // summary of gold entities (debugging)
	ExtractModel string   // summary of model entities (debugging)
}

// yesNoRe extracts the leading yes/no polarity. Models routinely open with a
// preamble ("**Short answer:** Yes, TDS applies — ..."), so after stripping
// markdown emphasis the first standalone yes/no token at the very start, or
// right after a "Short answer:/Answer:" label, counts (review fix #3).
var yesNoRe = regexp.MustCompile(`(?i)^(?:[\W_]*(?:short\s+answer|answer))?\W*(yes|no)\b`)

// extractPolarity finds the first yes/no token in s for yes_no_reasoned
// grading: markdown stripped, then match at the start or after a
// "Short answer:/Answer:" label; if still not found, fall back to the first
// word-boundary yes/no in the first 160 chars (skipping headings).
func extractPolarity(s string) string {
	stripped := emphasisRe.ReplaceAllString(s, " ")
	m := yesNoRe.FindStringSubmatch(stripped)
	if m == nil {
		if len(stripped) > 160 {
			stripped = stripped[:160]
		}
		m = firstYesNoRe.FindStringSubmatch(stripped)
	}
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

var firstYesNoRe = regexp.MustCompile(`(?i)\b(yes|no)\b`)

// goldPartMarkerRe matches the leading "(i)" roman-numeral markers used by
// the dataset golds ("(i) Payment code 1001; (ii) s.392(1) ...").
var goldPartMarkerRe = regexp.MustCompile(`\((i{1,3}|iv|v)\)`)

// Grade compares a model answer to the gold answer according to the task's
// answer_type. Grading is entity extraction based: every verifiable entity in
// gold must appear in the model output.
func Grade(t Task, modelAnswer string) GradeResult {
	gold := t.Answer
	at := t.AnswerType

	switch at {
	case "reasoned":
		// Rubric grader stub: never scored automatically.
		return GradeResult{Graded: false, Reason: "rubric_pending"}

	case "code":
		want := extractCodes(gold)
		if len(want) == 0 {
			return ungradable(at, "no code entity in gold")
		}
		out := extractCodes(modelAnswer)
		var missing []string
		for _, w := range want {
			if !containsStr(out, w) {
				missing = append(missing, "code "+w)
			}
		}
		return finish(at, gold, modelAnswer, missing, len(want))

	case "section":
		wantRefs := extractSectionRefs(gold)
		if len(wantRefs) == 0 {
			// B1/B2 section-map gold is often a bare "536" or "263(1)" with no
			// s. prefix (and occasionally "-" for no-equivalent). Fall back to
			// parsing a leading section token from the gold text.
			if ref, ok := parseLeadingSection(gold); ok {
				wantRefs = []SectionRef{ref}
			} else {
				return ungradable(at, "no section ref in gold: "+gold)
			}
		}
		return gradeRefs(wantRefs, at, gold, modelAnswer)

	case "nature_and_section":
		wantRefs := extractSectionRefs(gold)
		if len(wantRefs) == 0 {
			return ungradable(at, "no section ref in gold")
		}
		return gradeRefs(wantRefs, at, gold, modelAnswer)

	case "form":
		want := extractFormTokens(gold)
		if len(want) == 0 {
			// Bare gold like "104", "10A", "15G", or multi "134 / 135",
			// "3CA / 3CB / 3CD", plus quarterly names ("24Q", "26AS").
			want = extractBareFormTokens(gold)
			if len(want) == 0 {
				return ungradable(at, "no form token in gold: "+gold)
			}
		}
		out := extractFormTokens(modelAnswer)
		outAll := append(append([]string{}, out...), extractBareFormTokens(modelAnswer)...)
		var missing []string
		for _, w := range want {
			if !containsStr(outAll, w) {
				missing = append(missing, "form "+w)
			}
		}
		return finish(at, gold, modelAnswer, missing, len(want))

	case "rate":
		if isSlab(gold) {
			if isSlab(modelAnswer) {
				return GradeResult{Pass: true, ScoreParts: 1, Graded: true,
					ExtractGold: "slab", ExtractModel: "slab"}
			}
			return GradeResult{Pass: false, ScoreParts: 0, Graded: true,
				MissingGold: []string{"slab rates"}, ExtractGold: "slab",
				ExtractModel: entitiesSummary(modelAnswer)}
		}
		want := extractRates(gold)
		if len(want) == 0 {
			return ungradable(at, "no rate in gold")
		}
		out := extractRates(modelAnswer)
		var missing []string
		for _, w := range want {
			if !containsFloat(out, w) {
				missing = append(missing, fmt.Sprintf("rate %g%%", w))
			}
		}
		return finish(at, gold, modelAnswer, missing, len(want))

	case "threshold", "amount":
		wantAmt := extractAmounts(gold)
		if len(wantAmt) == 0 {
			// Threshold golds like "No standard threshold" / "Net winnings"
			// carry no Rs amount. Require the gold's key phrase verbatim
			// (case-insensitive) instead — still an objective check.
			if phrase, ok := nonNumericThresholdPhrase(gold); ok {
				if strings.Contains(strings.ToLower(modelAnswer), phrase) {
					return GradeResult{Pass: true, ScoreParts: 1, Graded: true,
						ExtractGold: phrase, ExtractModel: entitiesSummary(modelAnswer)}
				}
				return GradeResult{Pass: false, ScoreParts: 0, Graded: true,
					MissingGold: []string{"phrase " + phrase},
					ExtractGold: phrase, ExtractModel: entitiesSummary(modelAnswer)}
			}
			return ungradable(at, "no amount in gold: "+gold)
		}
		outAmt := extractAmounts(modelAnswer)
		var missing []string
		for _, w := range wantAmt {
			if !containsFloat(outAmt, w) {
				missing = append(missing, fmt.Sprintf("amount Rs %g", w))
			}
		}
		n := len(wantAmt)
		if at == "amount" {
			// amount also requires the rate
			wantRates := extractRates(gold)
			outRates := extractRates(modelAnswer)
			for _, w := range wantRates {
				if !containsFloat(outRates, w) {
					missing = append(missing, fmt.Sprintf("rate %g%%", w))
				}
			}
			n += len(wantRates)
		}
		return finish(at, gold, modelAnswer, missing, n)

	case "date", "form_and_date", "date_and_pct":
		wantDates := extractDates(gold)
		if len(wantDates) == 0 {
			return ungradable(at, "no date in gold")
		}
		outDates := extractDates(modelAnswer)
		var missing []string
		n := len(wantDates)
		for _, w := range wantDates {
			found := false
			for _, o := range outDates {
				if o == w {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, "date "+w.String())
			}
		}
		if at == "form_and_date" {
			wantF := extractFormTokens(gold)
			if len(wantF) == 0 {
				if b := firstBareNumber(gold); b != "" {
					wantF = []string{b}
				}
			}
			outF := extractFormTokens(modelAnswer)
			for _, w := range wantF {
				if !containsStr(outF, w) {
					missing = append(missing, "form "+w)
				}
			}
			n += len(wantF)
		}
		if at == "date_and_pct" {
			wantR := extractRates(gold)
			outR := extractRates(modelAnswer)
			for _, w := range wantR {
				if !containsFloat(outR, w) {
					missing = append(missing, fmt.Sprintf("rate %g%%", w))
				}
			}
			n += len(wantR)
		}
		return finish(at, gold, modelAnswer, missing, n)

	case "days":
		want := extractDays(gold)
		if len(want) == 0 {
			return ungradable(at, "no day count in gold")
		}
		out := extractDays(modelAnswer)
		var missing []string
		for _, w := range want {
			if !containsInt(out, w) {
				missing = append(missing, fmt.Sprintf("%d days", w))
			}
		}
		return finish(at, gold, modelAnswer, missing, len(want))

	case "act_selection", "act_and_section":
		wantActs := extractActYears(gold)
		if len(wantActs) == 0 {
			return ungradable(at, "no act year in gold")
		}
		outActs := extractActYears(modelAnswer)
		var missing []string
		n := len(wantActs)
		for _, w := range wantActs {
			if !containsStr(outActs, w) {
				missing = append(missing, "Act "+w)
			}
		}
		if at == "act_and_section" {
			wantRefs := extractSectionRefs(gold)
			outRefs := extractSectionRefs(modelAnswer)
			for _, w := range wantRefs {
				if !sectionMatches(w, outRefs) {
					missing = append(missing, "section "+w.String())
				}
			}
			n += len(wantRefs)
		}
		return finish(at, gold, modelAnswer, missing, n)

	case "section_and_code":
		wantRefs := extractSectionRefs(gold)
		wantCodes := extractCodes(gold)
		if len(wantRefs) == 0 || len(wantCodes) == 0 {
			return ungradable(at, "missing section or code in gold")
		}
		outRefs := extractSectionRefs(modelAnswer)
		outCodes := extractCodes(modelAnswer)
		var missing []string
		n := 0
		for _, w := range wantRefs {
			if !sectionMatches(w, outRefs) {
				missing = append(missing, "section "+w.String())
			}
			n++
		}
		for _, w := range wantCodes {
			if !containsStr(outCodes, w) {
				missing = append(missing, "code "+w)
			}
			n++
		}
		return finish(at, gold, modelAnswer, missing, n)

	case "yes_no_reasoned":
		pol := extractPolarity(gold)
		if pol == "" {
			return ungradable(at, "no yes/no polarity in gold")
		}
		wantPol := pol
		outPol := extractPolarity(modelAnswer)
		if outPol == "" || outPol != wantPol {
			return GradeResult{Pass: false, ScoreParts: 0, Graded: true,
				MissingGold:  []string{"polarity " + wantPol},
				ExtractGold:  "polarity=" + wantPol + " " + entitiesSummary(gold),
				ExtractModel: entitiesSummary(modelAnswer)}
		}
		// Also require the gold threshold amount to appear (reasoning anchor).
		wantAmt := extractAmounts(gold)
		outAmt := extractAmounts(modelAnswer)
		var missing []string
		for _, w := range wantAmt {
			if !containsFloat(outAmt, w) {
				missing = append(missing, fmt.Sprintf("amount Rs %g", w))
			}
		}
		return finish(at, gold, modelAnswer, missing, 1+len(wantAmt))

	case "multi_part":
		return gradeMultiPart(t, modelAnswer)

	default:
		return ungradable(at, "unknown answer_type")
	}
}

// gradeMultiPart grades each gold part against the best-matching region of the
// model answer. Gold and model may enumerate with different marker styles
// ((i)/(ii), (a)/(b), 1./2., bold headers); if no markers are found the
// fallback ladder is: (a) position alignment on any enumerated structure,
// (b) part 1 graded against the full answer, (c) whole-answer grading when the
// gold's parts carry no extractable entities (review fix #2).
func gradeMultiPart(t Task, modelAnswer string) GradeResult {
	goldParts := splitGoldParts(t.Answer)
	if len(goldParts) == 0 {
		return ungradable("multi_part", "no part markers in gold")
	}

	// Try structured alignment (position-based) first.
	outParts := splitModelParts(modelAnswer)
	var missing []string
	passed := 0
	for gi, gp := range goldParts {
		var chunk string
		switch {
		case gi < len(outParts):
			chunk = outParts[gi]
		case len(outParts) > 0:
			// model produced fewer parts than gold: remaining gold parts are
			// searched in the rest of the answer (from the last model part on)
			chunk = strings.Join(outParts[minInt(gi, len(outParts)-1):], " ")
		default:
			// no enumerable structure in the model answer: grade against the
			// whole answer (review fix #2 fallback)
			chunk = modelAnswer
		}
		// The whole answer is always an admissible search window: a part the
		// position alignment misses may still be answered elsewhere (lettered
		// vs roman markers, prose wrap-up). A pass in the wider window keeps
		// the part; a fail reports the specific missing entity.
		ok, miss := gradePart(gp, chunk)
		if !ok {
			ok, miss = gradePart(gp, modelAnswer)
		}
		if ok {
			passed++
		} else {
			missing = append(missing, fmt.Sprintf("part %d: %s", gi+1, miss))
		}
	}
	return GradeResult{
		Pass:         passed == len(goldParts),
		ScoreParts:   float64(passed) / float64(len(goldParts)),
		Graded:       true,
		Reason:       "",
		MissingGold:  missing,
		ExtractGold:  fmt.Sprintf("%d parts", len(goldParts)),
		ExtractModel: fmt.Sprintf("%d parts", len(outParts)),
	}
}

// emphasisRe strips markdown emphasis and heading hashes so "**(i) Payment
// code:**" and "# Heading" texts normalize before marker scanning.
var emphasisRe = regexp.MustCompile(`\*\*|__|(^|\n)#{1,6}\s*`)

// partLabelRe matches any enumeration label a model may use at a line start:
// roman (i)/(ii), lettered (a)/(b) or "a)" or "a.", digits "1." / "1)" /
// "**1.**" — the label must be followed by a capital letter or bold marker.
var partLabelRe = regexp.MustCompile(`(?im)^\s*(?:\*\*)?\(?([ivx]{1,5}|[a-j]|0?[1-9]|10)\)?\.?\)?(?:\*\*)?\s*[A-Z*]`)

// splitGoldParts splits a gold answer on its (i)/(ii)/... markers. The markers
// in the data are monotone with repeats ("i, ii, i, iii, iv" when the part
// text quotes "Sl. No. 1(i)"), so a marker ONLY starts a new part when it
// continues the expected sequence (next index, or restart after iv/v).
// Returns the text following each marker.
func splitGoldParts(s string) []string {
	locs := goldPartMarkerRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return nil
	}
	seq := []string{"i", "ii", "iii", "iv", "v"}
	want := 0
	var heads []int
	for _, loc := range locs {
		marker := strings.ToLower(s[loc[2]:loc[3]])
		if want < len(seq) && marker == seq[want] {
			heads = append(heads, loc[0])
			want++
		}
	}
	if len(heads) == 0 {
		return nil
	}
	var parts []string
	for i, h := range heads {
		end := len(s)
		if i+1 < len(heads) {
			end = heads[i+1]
		}
		parts = append(parts, s[h:end])
	}
	return parts
}

// splitModelParts splits a model answer into enumerated parts. It tries, in
// order: gold-style (i)/(ii) markers with the same sequence guard, lettered
// or numbered labels at line starts, and returns nil when neither structure
// is present (prose answer → whole-answer fallback windows).
func splitModelParts(s string) []string {
	if parts := splitGoldParts(s); len(parts) >= 2 {
		return parts
	}
	// lettered/numbered labels at line starts: collect distinct-label heads in
	// order; only use them when there are at least 2 and labels progress
	type label struct {
		text string
		pos  int
	}
	var labels []label
	heads := partLabelRe.FindAllStringSubmatchIndex(s, -1)
	prev := ""
	for _, loc := range heads {
		lab := strings.ToLower(strings.Trim(s[loc[2]:loc[3]], ".)"))
		if lab == prev {
			continue // repeated label (e.g. "Sl. No. 1(i)" quoting) — not a new part
		}
		labels = append(labels, label{text: lab, pos: loc[0]})
		prev = lab
	}
	if len(labels) >= 2 {
		var parts []string
		for i, lb := range labels {
			end := len(s)
			if i+1 < len(labels) {
				end = labels[i+1].pos
			}
			parts = append(parts, s[lb.pos:end])
		}
		return parts
	}
	return nil
}

// minInt returns the smaller of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// gradePart grades one gold part against one model chunk by sniffing the
// entity types present in the gold part text.
func gradePart(goldPart, modelChunk string) (bool, string) {
	// order matters: check most specific first
	if codes := extractCodes(goldPart); len(codes) > 0 {
		out := extractCodes(modelChunk)
		for _, w := range codes {
			if !containsStr(out, w) {
				return false, "code " + w
			}
		}
		return true, ""
	}
	if refs := extractSectionRefs(goldPart); len(refs) > 0 {
		out := extractSectionRefs(modelChunk)
		for _, w := range refs {
			if !sectionMatches(w, out) {
				return false, "section " + w.String()
			}
		}
		return true, ""
	}
	if isSlab(goldPart) {
		if isSlab(modelChunk) {
			return true, ""
		}
		return false, "slab rates"
	}
	if rates := extractRates(goldPart); len(rates) > 0 {
		out := extractRates(modelChunk)
		for _, w := range rates {
			if !containsFloat(out, w) {
				return false, fmt.Sprintf("rate %g%%", w)
			}
		}
		return true, ""
	}
	if dates := extractDates(goldPart); len(dates) > 0 {
		out := extractDates(modelChunk)
		for _, w := range dates {
			found := false
			for _, o := range out {
				if o == w {
					found = true
					break
				}
			}
			if !found {
				return false, "date " + w.String()
			}
		}
		return true, ""
	}
	// no entity found in this gold part — treat as satisfied (do not punish)
	return true, ""
}

// ---------- helpers ----------

func gradeRefs(want []SectionRef, at, gold, modelAnswer string) GradeResult {
	out := extractSectionRefs(modelAnswer)
	// Bare-answer outputs (e.g. the model replies just "263(1)" with no s.
	// prefix) yield no prefix-based refs; fall back to leading-token parse.
	if len(out) == 0 {
		if ref, ok := parseLeadingSection(modelAnswer); ok {
			out = append(out, ref)
		}
	}
	var missing []string
	for _, w := range want {
		if !sectionMatches(w, out) {
			missing = append(missing, "section "+w.String())
		}
	}
	return finish(at, gold, modelAnswer, missing, len(want))
}

func finish(at, gold, modelAnswer string, missing []string, nRequired int) GradeResult {
	if nRequired == 0 {
		return ungradable(at, "no required entities")
	}
	g := entitiesSummary(gold)
	m := entitiesSummary(modelAnswer)
	if len(missing) == 0 {
		return GradeResult{Pass: true, ScoreParts: 1, Graded: true, ExtractGold: g, ExtractModel: m}
	}
	return GradeResult{Pass: false, ScoreParts: 0, Graded: true,
		MissingGold: missing, ExtractGold: g, ExtractModel: m}
}

func ungradable(at, why string) GradeResult {
	return GradeResult{Graded: false, Reason: "ungradable:" + at + ":" + why}
}

// nonNumericThresholdPhrase returns the lowercase key phrase for threshold
// golds that state a rule instead of a number ("No standard threshold",
// "Net winnings"). ok=false when the gold has digits (a normal amount).
func nonNumericThresholdPhrase(gold string) (string, bool) {
	if strings.ContainsAny(gold, "0123456789") {
		return "", false
	}
	p := strings.ToLower(strings.TrimSpace(gold))
	if p == "" {
		return "", false
	}
	return p, true
}

func entitiesSummary(s string) string {
	var bits []string
	if c := extractCodes(s); len(c) > 0 {
		bits = append(bits, "codes:"+fmt.Sprint(c))
	}
	if r := extractSectionRefs(s); len(r) > 0 {
		var strs []string
		for _, x := range r {
			strs = append(strs, x.String())
		}
		bits = append(bits, "sections:"+fmt.Sprint(strs))
	}
	if isSlab(s) {
		bits = append(bits, "slab")
	}
	if r := extractRates(s); len(r) > 0 {
		bits = append(bits, "rates:"+fmt.Sprint(r))
	}
	if a := extractAmounts(s); len(a) > 0 {
		bits = append(bits, "amounts:"+fmt.Sprint(a))
	}
	if d := extractDates(s); len(d) > 0 {
		var strs []string
		for _, x := range d {
			strs = append(strs, x.String())
		}
		bits = append(bits, "dates:"+fmt.Sprint(strs))
	}
	if f := extractFormTokens(s); len(f) > 0 {
		bits = append(bits, "forms:"+fmt.Sprint(f))
	}
	if y := extractActYears(s); len(y) > 0 {
		bits = append(bits, "acts:"+fmt.Sprint(y))
	}
	if d := extractDays(s); len(d) > 0 {
		bits = append(bits, "days:"+fmt.Sprint(d))
	}
	if len(bits) == 0 {
		return "(no entities)"
	}
	return strings.Join(bits, " ")
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func containsFloat(list []float64, v float64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
