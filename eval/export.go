package taxeval

// Exported wrappers over the extraction/normalization logic, for reuse by
// downstream consumers (api/ P-L1 /v1/validate) per the licensing spec:
// citation parsing in /validate REUSES this normalization, never rewrites it.
// The functions are pure (regex over strings) and safe for concurrent use.

// ParseSectionRefs extracts normalized section references from free text
// ("s.194C", "Section 398(3)(a)(ii)", "s. 392(1)").
func ParseSectionRefs(s string) []SectionRef { return extractSectionRefs(s) }

// ParseForms extracts form tokens from free text ("Form 16", "Forms 138 and 143").
func ParseForms(s string) []string { return extractFormTokens(s) }

// ParsePaymentCodes extracts challan payment codes (1001..1092) from free text.
func ParsePaymentCodes(s string) []string { return extractCodes(s) }

// ParseActYears extracts Act years ("1961"/"2025") explicitly tied to the
// word "Act" — bare years in date/filing contexts are never matched.
func ParseActYears(s string) []string { return extractActYears(s) }

// ParseDates extracts normalized calendar dates (ISO, "15th June 2026", ...).
func ParseDates(s string) []Date { return extractDates(s) }

// ParseLeadingSection parses a bare leading section token ("536", "263(1)")
// from text that lacks an s./Section prefix.
func ParseLeadingSection(s string) (SectionRef, bool) { return parseLeadingSection(s) }
