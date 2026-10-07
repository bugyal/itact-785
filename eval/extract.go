package taxeval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ---------- payment codes (1001..1092) ----------

var codeRe = regexp.MustCompile(`\b(10[0-9]{2})\b`)

func extractCodes(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range codeRe.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// ---------- section references ----------

// sectionRe captures e.g. "s.392(1)", "s. 392(1)", "Section 398(3)(a)(ii)",
// "s.194A", "s.393". The space after "s." is optional (gold data uses both
// forms). Sub-section parts accept multi-char roman numerals (ii, iii, iv).
var sectionRe = regexp.MustCompile(`(?i)(?:s\s*\.\s*|section\s+|sec\.\s*)(\d{1,3}[A-Z]{0,2})((?:\([ivxlcdm]+\)|\([a-z]{1,4}\)|\(\d+\))*)`)

// SectionRef is a normalized section reference.
type SectionRef struct {
	Num      string   // "392", "194A"
	SubParts []string // ["1"] or ["3","a","ii"]; empty for bare refs
}

func (r SectionRef) String() string {
	if len(r.SubParts) == 0 {
		return r.Num
	}
	return r.Num + "(" + strings.Join(r.SubParts, ")(") + ")"
}

func parseSubParts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parts []string
	for _, m := range regexp.MustCompile(`\(([^)]+)\)`).FindAllStringSubmatch(raw, -1) {
		parts = append(parts, strings.ToLower(m[1]))
	}
	return parts
}

func extractSectionRefs(s string) []SectionRef {
	var out []SectionRef
	seen := map[string]bool{}
	for _, m := range sectionRe.FindAllStringSubmatch(s, -1) {
		ref := SectionRef{Num: m[1], SubParts: parseSubParts(m[2])}
		key := ref.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, ref)
		}
	}
	return out
}

// sectionMatches reports whether the gold reference is satisfied by the model
// output: exact subsection match, OR the output cites the same bare section
// number (gold 392(1) is satisfied by "s.392"; gold 392 is satisfied only by 392).
func sectionMatches(gold SectionRef, out []SectionRef) bool {
	for _, o := range out {
		if !strings.EqualFold(o.Num, gold.Num) {
			continue
		}
		if len(gold.SubParts) == 0 {
			return true
		}
		if len(o.SubParts) == 0 {
			return true // bare cite of the right section
		}
		if strings.EqualFold(o.String(), gold.String()) {
			return true
		}
	}
	return false
}

// parseLeadingSection parses a bare leading section token like "536",
// "263(1)", "132 / 132A" (first token only), "393(2) [Table ...]" from gold
// text that lacks an s./Section prefix. Returns false when the text does not
// start with a section number.
func parseLeadingSection(s string) (SectionRef, bool) {
	m := leadingSectionRe.FindStringSubmatch(s)
	if m == nil {
		return SectionRef{}, false
	}
	return SectionRef{Num: m[1], SubParts: parseSubParts(m[2])}, true
}

var leadingSectionRe = regexp.MustCompile(`^\s*(\d{1,3}[A-Z]{0,2})\s*((?:\([ivxlcdm]+\)|\([a-z]{1,4}\)|\(\d+\))*)`)

// ---------- rates ----------

var rateRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
var slabRe = regexp.MustCompile(`(?i)slab\s+rates?\b`)

func extractRates(s string) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, m := range rateRe.FindAllStringSubmatch(s, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err == nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func isSlab(s string) bool { return slabRe.MatchString(s) }

// ---------- amounts ----------

var amountRe = regexp.MustCompile(`(?:Rs\.?|INR|₹)\s*([\d,]+(?:\.\d+)?)`)

func extractAmounts(s string) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, m := range amountRe.FindAllStringSubmatch(s, -1) {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err == nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// ---------- dates ----------

var dateReISO = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)

var longMonth = map[string]int{
	"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
	"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
}

var dateReLong = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]+)\s+(\d{4})\b`)
var dateReLongRev = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})(?:st|nd|rd|th)?\s*,?\s+(\d{4})\b`)

// Date is a normalized calendar date.
type Date struct{ Y, M, D int }

func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Y, d.M, d.D) }

func extractDates(s string) []Date {
	seen := map[Date]bool{}
	var out []Date
	add := func(d Date) {
		if d.M >= 1 && d.M <= 12 && d.D >= 1 && d.D <= 31 && d.Y >= 1900 && d.Y <= 2100 && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, m := range dateReISO.FindAllStringSubmatch(s, -1) {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		add(Date{y, mo, d})
	}
	for _, m := range dateReLong.FindAllStringSubmatch(s, -1) {
		if mo, ok := longMonth[strings.ToLower(m[2])]; ok {
			d, _ := strconv.Atoi(m[1])
			y, _ := strconv.Atoi(m[3])
			add(Date{y, mo, d})
		}
	}
	for _, m := range dateReLongRev.FindAllStringSubmatch(s, -1) {
		if mo, ok := longMonth[strings.ToLower(m[1])]; ok {
			d, _ := strconv.Atoi(m[2])
			y, _ := strconv.Atoi(m[3])
			add(Date{y, mo, d})
		}
	}
	return out
}

// ---------- form tokens ----------

// formRe matches "Form 10A" and, with the optional plural s (review fix #5),
// "Forms 138 and 143". The token accepts up to two trailing letters.
var formRe = regexp.MustCompile(`(?i)\bForms?\s+(\d{1,3}[A-Z]{0,2})\b`)
var bareFormRe = regexp.MustCompile(`\b(\d{1,3}[A-Z]?)\b`)

func extractFormTokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range formRe.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	// Conjunction lists: "Form 26QB and 26QC", "Forms 138 and 143" — the
	// second token has no "Form" prefix. Bridge "and|,|&" gaps from a matched
	// form token to the next bare token.
	for _, m := range formListRe.FindAllStringSubmatch(s, -1) {
		if !seen[m[2]] {
			seen[m[2]] = true
			out = append(out, m[2])
		}
	}
	return out
}

// formListRe captures "Form <tok> (and|,|&) <tok>" pairs. The follow token
// must look like a form token (1-3 digits + up to 2 letters) and the match is
// case-insensitive on "Form".
var formListRe = regexp.MustCompile(`(?i)\bForms?\s+(\d{1,3}[A-Z]{0,2})\s*(?:,|and|&)\s+(\d{1,3}[A-Z]{0,2})\b`)

// bareFormTokenRe matches alphanumeric form tokens: "104", "10A", "15G",
// "24Q", "26AS", "3CA", "16B". Digits optional if letters present ("AS").
var bareFormTokenRe = regexp.MustCompile(`\b(\d{1,3}[A-Z]{0,2}|[A-Z]{1,4}\d{0,2})\b`)

// extractBareFormTokens pulls form tokens from bare gold like "134 / 135" or
// "3CA / 3CB / 3CD". Splits on "/" and strips whitespace. Returns tokens that
// contain at least one digit or are all-caps letter codes (26AS, 15G).
func extractBareFormTokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(s, "/") {
		part = strings.TrimSpace(part)
		m := bareFormTokenRe.FindStringSubmatch(strings.ToUpper(part))
		if m == nil {
			continue
		}
		tok := m[1]
		hasDigit := strings.ContainsAny(tok, "0123456789")
		allCaps := tok == strings.ToUpper(tok) && len(tok) >= 2
		if (hasDigit || allCaps) && !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

// firstBareNumber returns the first standalone 1-3 digit token (used when the
// gold form answer is a bare "104" with no "Form" prefix).
func firstBareNumber(s string) string {
	m := bareFormRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// ---------- act years ----------

// actRe finds Act-year mentions (review fix #4). A bare year like "2025" in a
// date or filing context must NOT count — the year must be anchored to the
// "Act" noun, before or after: "Income-tax Act 2025", "Act of 1961",
// "1961 Act", "new Act (2025)". Bare years never match.
var actRe = regexp.MustCompile(`(?i)(?:act\b[^.?:;]{0,30}?\b(19[66]1|2025))|(?:(19[66]1|2025)\s*,?\s+act\b)|(?:\bact\s+of\s+(19[66]1|2025)\b)`)

// extractActYears returns the Act years ("1961"/"2025") explicitly tied to the
// word "Act" in either direction. A model citing a filing date "2025" alone
// is not credited with an Act selection.
func extractActYears(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range actRe.FindAllStringSubmatch(s, -1) {
		yr := m[1]
		if yr == "" {
			yr = m[2]
		}
		if yr == "" {
			yr = m[3]
		}
		if yr == "1966" { // regex typo safety: "19[66]1" cannot yield 1966
			yr = "1961"
		}
		if !seen[yr] {
			seen[yr] = true
			out = append(out, yr)
		}
	}
	return out
}

// ---------- days ----------

var daysRe = regexp.MustCompile(`(\d+)\s*days?\b`)

func extractDays(s string) []int {
	seen := map[int]bool{}
	var out []int
	for _, m := range daysRe.FindAllStringSubmatch(s, -1) {
		v, err := strconv.Atoi(m[1])
		if err == nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
