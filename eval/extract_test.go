package taxeval

import (
	"strings"
	"testing"
)

func TestExtractCodes(t *testing.T) {
	in := "Payment code 1001; also 1092 and year 2026 and 1005."
	got := extractCodes(in)
	want := []string{"1001", "1092", "1005"}
	if len(got) != len(want) {
		t.Fatalf("codes: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("codes: got %v want %v", got, want)
		}
	}
}

func TestExtractSectionRefs(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"under s.392(1) of the Act", "392(1)"},
		{"Section 398(3)(a)(ii) applies", "398(3)(a)(ii)"},
		{"s.194A read with", "194A"},
		{"s.393 of the new Act", "393"},
		{"per section 206AA", "206AA"},
	}
	for _, c := range cases {
		refs := extractSectionRefs(c.in)
		found := false
		for _, r := range refs {
			if r.String() == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("section %q: got %v want %s", c.in, refs, c.want)
		}
	}
}

func TestSectionMatches(t *testing.T) {
	gold := extractSectionRefs("s.392(1)")
	out := extractSectionRefs("TDS under section 392 of the 2025 Act")
	if !sectionMatches(gold[0], out) {
		t.Error("gold 392(1) should match bare 392")
	}
	outDiff := extractSectionRefs("s.392(2)")
	if len(gold) > 0 && sectionMatches(gold[0], outDiff) {
		t.Error("gold 392(1) must NOT match 392(2)")
	}
}

func TestExtractRatesAndSlab(t *testing.T) {
	if r := extractRates("deduct at 10% or 2%"); len(r) != 2 || r[0] != 10 || r[1] != 2 {
		t.Errorf("rates: got %v", r)
	}
	if !isSlab("Slab rates (regime-dependent)") {
		t.Error("slab not detected")
	}
	if isSlab("at 20%") {
		t.Error("false slab")
	}
}

func TestExtractAmounts(t *testing.T) {
	got := extractAmounts("Rs 2,100,000 (at 10%) threshold Rs 50,000")
	if len(got) != 2 || got[0] != 2100000 || got[1] != 50000 {
		t.Errorf("amounts: got %v", got)
	}
}

func TestExtractDates(t *testing.T) {
	cases := []struct {
		in   string
		want Date
	}{
		{"due 2027-05-31 under", Date{2027, 5, 31}},
		{"by 31 May 2027", Date{2027, 5, 31}},
		{"by 7 June 2026", Date{2026, 6, 7}},
		{"on May 31, 2027", Date{2027, 5, 31}},
		{"by 15th August 2026", Date{2026, 8, 15}},
	}
	for _, c := range cases {
		dates := extractDates(c.in)
		found := false
		for _, d := range dates {
			if d == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("date %q: got %v want %v", c.in, dates, c.want)
		}
	}
}

func TestExtractFormTokens(t *testing.T) {
	if got := extractFormTokens("Form 138 (salary) or form 10A"); len(got) != 2 || got[0] != "138" || got[1] != "10A" {
		t.Errorf("forms: got %v", got)
	}
	if firstBareNumber("104") != "104" {
		t.Error("bare form number failed")
	}
}

func TestExtractActYears(t *testing.T) {
	got := extractActYears("Income-tax Act 2025 applies, not the Income-tax Act, 1961")
	has2025, has1961 := false, false
	for _, y := range got {
		if y == "2025" {
			has2025 = true
		}
		if y == "1961" {
			has1961 = true
		}
	}
	if !has2025 || !has1961 {
		t.Errorf("acts: got %v", got)
	}
}

func TestExtractDays(t *testing.T) {
	if got := extractDays("within 15 days of"); len(got) != 1 || got[0] != 15 {
		t.Errorf("days: got %v", got)
	}
}

func TestSplitParts(t *testing.T) {
	in := "(i) Payment code 1001; (ii) s.392(1) of the Income-tax Act 2025; (iii) 10%; (iv) deposit by 7 June 2026."
	parts := splitGoldParts(in)
	if len(parts) != 4 {
		t.Fatalf("parts: got %d: %v", len(parts), parts)
	}
	if !strings.Contains(parts[0], "1001") || !strings.Contains(parts[3], "7 June 2026") {
		t.Errorf("parts content wrong: %v", parts)
	}
}

// A quoted "Sl. No. 1(i)" inside part (ii)'s text must not split the gold a
// second time at (i): the sequence guard keeps 4 parts.
func TestSplitPartsQuotedMarkers(t *testing.T) {
	in := "(i) Payment code 1005; (ii) s.393(1) [Table: Sl. No. 1(i)] of the Income-tax Act 2025; (iii) 2%; (iv) deposit by 7 June 2026."
	parts := splitGoldParts(in)
	if len(parts) != 4 {
		t.Fatalf("parts: got %d: %v", len(parts), parts)
	}
	if !strings.Contains(parts[1], "Sl. No. 1(i)") {
		t.Errorf("part 2 lost its quoted marker: %q", parts[1])
	}
}

func TestYesNoPolarity(t *testing.T) {
	for _, s := range []string{
		"Yes - the payment of Rs 90,000 exceeds",
		"No - the payment is below",
		"**Short answer:** Yes, TDS applies — the payment (Rs 90,000) exceeds the Rs 50,000 threshold.",
		"# TDS on Dividend Payment — Rs 6,000 to Resident\n\n**No, tax is not deductible at source.**",
		"yes, because the amount crosses the limit",
	} {
		if extractPolarity(s) == "" {
			t.Errorf("polarity not found in %q", s)
		}
	}
	if extractPolarity("**Short answer:** Yes, TDS applies") != "yes" {
		t.Error("preamble yes failed")
	}
	if extractPolarity("**No, tax is not deductible at source.**") != "no" {
		t.Error("bold no failed")
	}
	if p := extractPolarity("The payment of Rs 12,000 ... notify maybe"); p != "" && p != "no" {
		t.Errorf("unexpected polarity %q", p)
	}
}

func TestActYearBareYearRejected(t *testing.T) {
	// review fix #4: bare years in date/filing context must NOT match
	if got := extractActYears("filed on 2025-04-01 for the previous year"); len(got) != 0 {
		t.Errorf("bare 2025 wrongly matched: %v", got)
	}
	if got := extractActYears("the filing date 2025 was fine"); len(got) != 0 {
		t.Errorf("bare 'filing date 2025' wrongly matched: %v", got)
	}
	// anchored mentions must match
	for _, s := range []string{
		"under the Income-tax Act 2025",
		"the Income-tax Act, 2025 applies",
		"governed by the 1961 Act",
		"Act of 1961 continues",
		"the new Act (2025) covers this",
	} {
		if len(extractActYears(s)) == 0 {
			t.Errorf("anchored act year not found in %q", s)
		}
	}
}

func TestFormPlural(t *testing.T) {
	// review fix #5: "Forms 138 and 143" must yield both tokens
	got := extractFormTokens("file Form 10A and Forms 138 and 143")
	want := map[string]bool{"10A": false, "138": false, "143": false}
	for _, g := range got {
		if _, ok := want[g]; ok {
			want[g] = true
		}
	}
	for tok, seen := range want {
		if !seen {
			t.Errorf("form token %s missing from %v", tok, got)
		}
	}
}

// A model listing forms across a conjunction boundary without repeating
// "Form" still yields both ("Form 26QB and 26QC").
func TestFormConjunction(t *testing.T) {
	got := extractFormTokens("submit Form 26QB and 26QC, plus Form 16")
	want := map[string]bool{"26QB": false, "26QC": false, "16": false}
	for _, g := range got {
		if _, ok := want[g]; ok {
			want[g] = true
		}
	}
	for tok, seen := range want {
		if !seen {
			t.Errorf("form token %s missing from %v", tok, got)
		}
	}
}
