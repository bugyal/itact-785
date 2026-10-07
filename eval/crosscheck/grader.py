"""Faithful Python port of the Tax-Model Go grader (eval/extract.go + eval/grade.go).

Used by the inspect-ai cross-check (P2.5) so that scores produced by a standard
eval tool are comparable to the Go harness. Parity against the Go grader is
verified record-by-record by parity_check.py over the archived sweep results.

Port rules:
- Go RE2 patterns map 1:1 to Python `re` for the patterns used here.
- Go FindAllStringSubmatch order == Python finditer order (leftmost-first).
- go_fmt_float reproduces Go's %v float formatting (10.0 -> "10", 2.5 -> "2.5").
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass, field
from typing import List, Optional, Sequence, Tuple

# ---------------- payment codes (1001..1092) ----------------

CODE_RE = re.compile(r"\b(10[0-9]{2})\b", re.ASCII)


def extract_codes(s: str) -> List[str]:
    seen = set()
    out = []
    for m in CODE_RE.finditer(s):
        if m.group(1) not in seen:
            seen.add(m.group(1))
            out.append(m.group(1))
    return out


# ---------------- section references ----------------

SECTION_RE = re.compile(
    r"(?:s\s*\.\s*|section\s+|sec\.\s*)(\d{1,3}[A-Z]{0,2})"
    r"((?:\([ivxlcdm]+\)|\([a-z]{1,4}\)|\(\d+\))*)",
    re.IGNORECASE | re.ASCII,
)


@dataclass
class SectionRef:
    num: str
    sub_parts: List[str] = field(default_factory=list)

    def string(self) -> str:
        if not self.sub_parts:
            return self.num
        return self.num + "(" + ")(".join(self.sub_parts) + ")"


def parse_sub_parts(raw: str) -> List[str]:
    raw = raw.strip()
    if raw == "":
        return []
    return [m.group(1).lower() for m in re.finditer(r"\(([^)]+)\)", raw)]


def extract_section_refs(s: str) -> List[SectionRef]:
    out = []
    seen = set()
    for m in SECTION_RE.finditer(s):
        ref = SectionRef(num=m.group(1), sub_parts=parse_sub_parts(m.group(2)))
        key = ref.string()
        if key not in seen:
            seen.add(key)
            out.append(ref)
    return out


def section_matches(gold: SectionRef, out: List[SectionRef]) -> bool:
    """Gold reference satisfied: exact subsection match, OR bare cite of the
    same section number (gold 392(1) satisfied by "s.392"; gold 392 only by 392)."""
    for o in out:
        if o.num.lower() != gold.num.lower():
            continue
        if not gold.sub_parts:
            return True
        if not o.sub_parts:
            return True  # bare cite of the right section
        if o.string().lower() == gold.string().lower():
            return True
    return False


LEADING_SECTION_RE = re.compile(
    r"^\s*(\d{1,3}[A-Z]{0,2})\s*((?:\([ivxlcdm]+\)|\([a-z]{1,4}\)|\(\d+\))*)", re.ASCII
)


def parse_leading_section(s: str) -> Optional[SectionRef]:
    m = LEADING_SECTION_RE.match(s)
    if m is None:
        return None
    return SectionRef(num=m.group(1), sub_parts=parse_sub_parts(m.group(2)))


# ---------------- rates ----------------

RATE_RE = re.compile(r"(\d+(?:\.\d+)?)\s*%", re.ASCII)
SLAB_RE = re.compile(r"slab\s+rates?\b", re.IGNORECASE | re.ASCII)


def extract_rates(s: str) -> List[float]:
    seen = set()
    out = []
    for m in RATE_RE.finditer(s):
        v = float(m.group(1))
        if v not in seen:
            seen.add(v)
            out.append(v)
    return out


def is_slab(s: str) -> bool:
    return SLAB_RE.search(s) is not None


def go_fmt_float(v: float) -> str:
    """Go %v float formatting: integral values print without decimals."""
    if v == int(v) and abs(v) < 1e15:
        return str(int(v))
    return repr(v)


# ---------------- amounts ----------------

AMOUNT_RE = re.compile(r"(?:Rs\.?|INR|\u20b9)\s*([\d,]+(?:\.\d+)?)", re.ASCII)


def extract_amounts(s: str) -> List[float]:
    seen = set()
    out = []
    for m in AMOUNT_RE.finditer(s):
        try:
            v = float(m.group(1).replace(",", ""))
        except ValueError:
            continue  # mirrors Go: ParseFloat error skips the match
        if v not in seen:
            seen.add(v)
            out.append(v)
    return out


# ---------------- dates ----------------

DATE_RE_ISO = re.compile(r"\b(\d{4})-(\d{2})-(\d{2})\b", re.ASCII)

LONG_MONTH = {
    "january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
    "july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
}

DATE_RE_LONG = re.compile(r"\b(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]+)\s+(\d{4})\b", re.IGNORECASE | re.ASCII)
DATE_RE_LONG_REV = re.compile(r"\b([A-Za-z]+)\s+(\d{1,2})(?:st|nd|rd|th)?\s*,?\s+(\d{4})\b", re.IGNORECASE | re.ASCII)


@dataclass
class Date:
    y: int
    m: int
    d: int

    def string(self) -> str:
        return "%04d-%02d-%02d" % (self.y, self.m, self.d)


def extract_dates(s: str) -> List[Date]:
    seen = set()
    out = []

    def add(d: Date):
        if 1 <= d.m <= 12 and 1 <= d.d <= 31 and 1900 <= d.y <= 2100 and (d.y, d.m, d.d) not in seen:
            seen.add((d.y, d.m, d.d))
            out.append(d)

    for m in DATE_RE_ISO.finditer(s):
        add(Date(int(m.group(1)), int(m.group(2)), int(m.group(3))))
    for m in DATE_RE_LONG.finditer(s):
        mo = LONG_MONTH.get(m.group(2).lower())
        if mo is not None:
            add(Date(int(m.group(3)), mo, int(m.group(1))))
    for m in DATE_RE_LONG_REV.finditer(s):
        mo = LONG_MONTH.get(m.group(1).lower())
        if mo is not None:
            add(Date(int(m.group(3)), mo, int(m.group(2))))
    return out


# ---------------- form tokens ----------------

FORM_RE = re.compile(r"\bForms?\s+(\d{1,3}[A-Z]{0,2})\b", re.IGNORECASE | re.ASCII)
BARE_FORM_RE = re.compile(r"\b(\d{1,3}[A-Z]?)\b", re.ASCII)
BARE_FORM_TOKEN_RE = re.compile(r"\b(\d{1,3}[A-Z]{0,2}|[A-Z]{1,4}\d{0,2})\b", re.ASCII)


def extract_form_tokens(s: str) -> List[str]:
    seen = set()
    out = []
    for m in FORM_RE.finditer(s):
        if m.group(1) not in seen:
            seen.add(m.group(1))
            out.append(m.group(1))
    return out


def extract_bare_form_tokens(s: str) -> List[str]:
    """Form tokens from bare gold like "134 / 135" or "3CA / 3CB / 3CD"."""
    seen = set()
    out = []
    for part in s.split("/"):
        part = part.strip()
        m = BARE_FORM_TOKEN_RE.search(part.upper())
        if m is None:
            continue
        tok = m.group(1)
        has_digit = any(c.isdigit() for c in tok)
        all_caps = tok == tok.upper() and len(tok) >= 2
        if (has_digit or all_caps) and tok not in seen:
            seen.add(tok)
            out.append(tok)
    return out


def first_bare_number(s: str) -> str:
    m = BARE_FORM_RE.search(s)
    return m.group(1) if m else ""


# ---------------- act years ----------------

ACT_RE = re.compile(
    r"(?:act\b[^.?:;]{0,30}?\b(19[66]1|2025))|(?:(19[66]1|2025)\s*,?\s+act\b)|(?:\bact\s+of\s+(19[66]1|2025)\b)",
    re.IGNORECASE | re.ASCII,
)


def extract_act_years(s: str) -> List[str]:
    """Act years ("1961"/"2025") explicitly tied to the word "Act". A bare
    year in a filing-date context is NOT an Act selection."""
    seen = set()
    out = []
    for m in ACT_RE.finditer(s):
        yr = m.group(1) or m.group(2) or m.group(3)
        if yr == "1966":  # regex typo safety, mirrors Go port
            yr = "1961"
        if yr not in seen:
            seen.add(yr)
            out.append(yr)
    return out


# ---------------- days ----------------

DAYS_RE = re.compile(r"(\d+)\s*days?\b", re.ASCII)


def extract_days(s: str) -> List[int]:
    seen = set()
    out = []
    for m in DAYS_RE.finditer(s):
        v = int(m.group(1))
        if v not in seen:
            seen.add(v)
            out.append(v)
    return out


# ---------------- polarity (yes_no_reasoned) ----------------

EMPHASIS_RE = re.compile(r"\*\*|__|(^|\n)#{1,6}\s*", re.ASCII)
YES_NO_RE = re.compile(r"^(?:[\W_]*(?:short\s+answer|answer))?\W*(yes|no)\b", re.IGNORECASE | re.ASCII)
FIRST_YES_NO_RE = re.compile(r"\b(yes|no)\b", re.IGNORECASE | re.ASCII)


def extract_polarity(s: str) -> str:
    stripped = EMPHASIS_RE.sub(" ", s)
    m = YES_NO_RE.match(stripped)
    if m is None:
        if len(stripped) > 160:
            stripped = stripped[:160]
        m = FIRST_YES_NO_RE.search(stripped)
    if m is None:
        return ""
    return m.group(1).lower()


# ---------------- result type ----------------


@dataclass
class GradeResult:
    passed: bool
    score_parts: float
    graded: bool
    reason: str = ""
    missing_gold: List[str] = field(default_factory=list)
    extract_gold: str = ""
    extract_model: str = ""


def ungradable(at: str, why: str) -> GradeResult:
    return GradeResult(passed=False, score_parts=0.0, graded=False, reason="ungradable:%s:%s" % (at, why))


def contains_str(lst: List[str], s: str) -> bool:
    return s in lst


def contains_float(lst: Sequence[float], v: float) -> bool:
    return v in lst


def non_numeric_threshold_phrase(gold: str) -> Tuple[str, bool]:
    """Key phrase for threshold golds stating a rule ("No standard threshold").
    ok=False when the gold carries digits (a normal amount)."""
    if any(c.isdigit() for c in gold):
        return "", False
    p = gold.strip().lower()
    if p == "":
        return "", False
    return p, True


def entities_summary(s: str) -> str:
    """Mirrors Go entitiesSummary incl. fmt.Sprint slice formatting ([a b c])."""
    bits = []
    c = extract_codes(s)
    if c:
        bits.append("codes:" + "[" + " ".join(c) + "]")
    r = extract_section_refs(s)
    if r:
        bits.append("sections:" + "[" + " ".join(x.string() for x in r) + "]")
    if is_slab(s):
        bits.append("slab")
    rr = extract_rates(s)
    if rr:
        bits.append("rates:" + "[" + " ".join(go_fmt_float(x) for x in rr) + "]")
    a = extract_amounts(s)
    if a:
        bits.append("amounts:" + "[" + " ".join(go_fmt_float(x) for x in a) + "]")
    d = extract_dates(s)
    if d:
        bits.append("dates:" + "[" + " ".join(x.string() for x in d) + "]")
    f = extract_form_tokens(s)
    if f:
        bits.append("forms:" + "[" + " ".join(f) + "]")
    y = extract_act_years(s)
    if y:
        bits.append("acts:" + "[" + " ".join(y) + "]")
    dd = extract_days(s)
    if dd:
        bits.append("days:" + "[" + " ".join(str(x) for x in dd) + "]")
    if not bits:
        return "(no entities)"
    return " ".join(bits)


def finish(at: str, gold: str, model_answer: str, missing: List[str], n_required: int) -> GradeResult:
    if n_required == 0:
        return ungradable(at, "no required entities")
    g = entities_summary(gold)
    m = entities_summary(model_answer)
    if not missing:
        return GradeResult(passed=True, score_parts=1.0, graded=True, extract_gold=g, extract_model=m)
    return GradeResult(passed=False, score_parts=0.0, graded=True,
                       missing_gold=missing, extract_gold=g, extract_model=m)


def grade_refs(want: List[SectionRef], at: str, gold: str, model_answer: str) -> GradeResult:
    out = extract_section_refs(model_answer)
    if not out:
        ref = parse_leading_section(model_answer)
        if ref is not None:
            out.append(ref)
    missing = []
    for w in want:
        if not section_matches(w, out):
            missing.append("section " + w.string())
    return finish(at, gold, model_answer, missing, len(want))


# ---------------- multi-part ----------------

GOLD_PART_MARKER_RE = re.compile(r"\((i{1,3}|iv|v)\)")
PART_LABEL_RE = re.compile(r"^\s*(?:\*\*)?\(?([ivx]{1,5}|[a-j]|0?[1-9]|10)\)?\.?\)?(?:\*\*)?\s*[A-Z*]",
                           re.IGNORECASE | re.MULTILINE | re.ASCII)


def split_gold_parts(s: str) -> List[str]:
    """Split gold on (i)/(ii)/... markers; a marker only starts a new part when
    it continues the expected sequence (monotone-with-repeats guard)."""
    locs = [(m.start(), m.group(1)) for m in GOLD_PART_MARKER_RE.finditer(s)]
    if not locs:
        return []
    seq = ["i", "ii", "iii", "iv", "v"]
    want = 0
    heads = []
    for pos, marker in locs:
        if want < len(seq) and marker.lower() == seq[want]:
            heads.append(pos)
            want += 1
    if not heads:
        return []
    parts = []
    for i, h in enumerate(heads):
        end = len(s) if i + 1 >= len(heads) else heads[i + 1]
        parts.append(s[h:end])
    return parts


def split_model_parts(s: str) -> List[str]:
    parts = split_gold_parts(s)
    if len(parts) >= 2:
        return parts
    labels = []  # (text, pos)
    prev = None
    for m in PART_LABEL_RE.finditer(s):
        lab = m.group(1).lower().strip(".)")
        if lab == prev:
            continue
        labels.append((lab, m.start()))
        prev = lab
    if len(labels) >= 2:
        parts = []
        for i, (lab, pos) in enumerate(labels):
            end = len(s) if i + 1 >= len(labels) else labels[i + 1][1]
            parts.append(s[pos:end])
        return parts
    return []


def grade_part(gold_part: str, model_chunk: str) -> Tuple[bool, str]:
    """Grade one gold part against one model chunk by entity-type sniffing."""
    codes = extract_codes(gold_part)
    if codes:
        out = extract_codes(model_chunk)
        for w in codes:
            if w not in out:
                return False, "code " + w
        return True, ""
    refs = extract_section_refs(gold_part)
    if refs:
        out = extract_section_refs(model_chunk)
        for w in refs:
            if not section_matches(w, out):
                return False, "section " + w.string()
        return True, ""
    if is_slab(gold_part):
        return (True, "") if is_slab(model_chunk) else (False, "slab rates")
    rates = extract_rates(gold_part)
    if rates:
        out = extract_rates(model_chunk)
        for w in rates:
            if w not in out:
                return False, "rate %s%%" % go_fmt_float(w)
        return True, ""
    dates = extract_dates(gold_part)
    if dates:
        out = extract_dates(model_chunk)
        out_set = {(o.y, o.m, o.d) for o in out}
        for w in dates:
            if (w.y, w.m, w.d) not in out_set:
                return False, "date " + w.string()
        return True, ""
    # no entity found in this gold part — satisfied (do not punish)
    return True, ""


def grade_multi_part(t: "Task", model_answer: str) -> GradeResult:
    gold_parts = split_gold_parts(t.answer)
    if not gold_parts:
        return ungradable("multi_part", "no part markers in gold")
    out_parts = split_model_parts(model_answer)
    missing = []
    passed = 0
    for gi, gp in enumerate(gold_parts):
        if gi < len(out_parts):
            chunk = out_parts[gi]
        else:
            start = min(gi, len(out_parts) - 1)
            chunk = " ".join(out_parts[start:]) if out_parts else ""
        ok, miss = grade_part(gp, chunk)
        if not ok:
            ok, miss = grade_part(gp, model_answer)
        if ok:
            passed += 1
        else:
            missing.append("part %d: %s" % (gi + 1, miss))
    return GradeResult(
        passed=(passed == len(gold_parts)),
        score_parts=passed / len(gold_parts),
        graded=True,
        missing_gold=missing,
        extract_gold="%d parts" % len(gold_parts),
        extract_model="%d parts" % len(out_parts),
    )


# ---------------- task + main Grade entry point ----------------


@dataclass
class Task:
    task_type: str
    table: str
    fact_key: str
    question: str
    answer: str
    answer_type: str
    template: str
    id: str = ""

    @classmethod
    def from_json(cls, obj: dict) -> "Task":
        t = cls(
            task_type=obj.get("task_type", ""),
            table=obj.get("table", ""),
            fact_key=obj.get("fact_key", ""),
            question=obj.get("question", ""),
            answer=obj.get("answer", ""),
            answer_type=obj.get("answer_type", ""),
            template=obj.get("template", ""),
        )
        t.id = task_id(t.question)
        return t


def task_id(q: str) -> str:
    """First 8 hex chars of sha256(question) — matches Go TaskID."""
    return hashlib.sha256(q.encode("utf-8")).hexdigest()[:8]


def verify_lock(split_path: str, lock_path: str) -> Tuple[str, Optional[str]]:
    """Returns (actual_sha, error_or_None). Mirrors Go VerifyLock."""
    try:
        lock = json.load(open(lock_path))
    except Exception as e:
        return "", "read/parse lock file: %s" % e
    sha = lock.get("sha256_questions", "")
    if not sha:
        return "", 'lock file has no sha256_questions field'
    qs = []
    with open(split_path) as f:
        for line in f:
            line = line.strip()
            if line:
                qs.append(Task.from_json(json.loads(line)).question)
    qs.sort()
    joined = "\n".join(qs)
    actual = hashlib.sha256(joined.encode("utf-8")).hexdigest()
    if actual != sha:
        return actual, "LOCK MISMATCH: %s != locked %s" % (actual, sha)
    return actual, None


def grade(t: Task, model_answer: str) -> GradeResult:
    gold = t.answer
    at = t.answer_type

    if at == "reasoned":
        return GradeResult(passed=False, score_parts=0.0, graded=False, reason="rubric_pending")

    if at == "code":
        want = extract_codes(gold)
        if not want:
            return ungradable(at, "no code entity in gold")
        out = extract_codes(model_answer)
        missing = ["code " + w for w in want if not contains_str(out, w)]
        return finish(at, gold, model_answer, missing, len(want))

    if at == "section":
        want_refs = extract_section_refs(gold)
        if not want_refs:
            ref = parse_leading_section(gold)
            if ref is not None:
                want_refs = [ref]
            else:
                return ungradable(at, "no section ref in gold: " + gold)
        return grade_refs(want_refs, at, gold, model_answer)

    if at == "nature_and_section":
        want_refs = extract_section_refs(gold)
        if not want_refs:
            return ungradable(at, "no section ref in gold")
        return grade_refs(want_refs, at, gold, model_answer)

    if at == "form":
        want = extract_form_tokens(gold)
        if not want:
            want = extract_bare_form_tokens(gold)
            if not want:
                return ungradable(at, "no form token in gold: " + gold)
        out = extract_form_tokens(model_answer)
        out_all = out + extract_bare_form_tokens(model_answer)
        missing = ["form " + w for w in want if not contains_str(out_all, w)]
        return finish(at, gold, model_answer, missing, len(want))

    if at == "rate":
        if is_slab(gold):
            if is_slab(model_answer):
                return GradeResult(passed=True, score_parts=1.0, graded=True,
                                   extract_gold="slab", extract_model="slab")
            return GradeResult(passed=False, score_parts=0.0, graded=True,
                               missing_gold=["slab rates"], extract_gold="slab",
                               extract_model=entities_summary(model_answer))
        want = extract_rates(gold)
        if not want:
            return ungradable(at, "no rate in gold")
        out = extract_rates(model_answer)
        missing = ["rate %s%%" % go_fmt_float(w) for w in want if not contains_float(out, w)]
        return finish(at, gold, model_answer, missing, len(want))

    if at in ("threshold", "amount"):
        want_amt = extract_amounts(gold)
        if not want_amt:
            phrase, ok = non_numeric_threshold_phrase(gold)
            if ok:
                if phrase in model_answer.lower():
                    return GradeResult(passed=True, score_parts=1.0, graded=True,
                                       extract_gold=phrase, extract_model=entities_summary(model_answer))
                return GradeResult(passed=False, score_parts=0.0, graded=True,
                                   missing_gold=["phrase " + phrase],
                                   extract_gold=phrase, extract_model=entities_summary(model_answer))
            return ungradable(at, "no amount in gold: " + gold)
        out_amt = extract_amounts(model_answer)
        missing = ["amount Rs %s" % go_fmt_float(w) for w in want_amt if not contains_float(out_amt, w)]
        n = len(want_amt)
        if at == "amount":
            want_rates = extract_rates(gold)
            out_rates = extract_rates(model_answer)
            missing += ["rate %s%%" % go_fmt_float(w) for w in want_rates if not contains_float(out_rates, w)]
            n += len(want_rates)
        return finish(at, gold, model_answer, missing, n)

    if at in ("date", "form_and_date", "date_and_pct"):
        want_dates = extract_dates(gold)
        if not want_dates:
            return ungradable(at, "no date in gold")
        out_dates = extract_dates(model_answer)
        out_set = {(o.y, o.m, o.d) for o in out_dates}
        missing = ["date " + w.string() for w in want_dates if (w.y, w.m, w.d) not in out_set]
        n = len(want_dates)
        if at == "form_and_date":
            want_f = extract_form_tokens(gold)
            if not want_f:
                b = first_bare_number(gold)
                if b:
                    want_f = [b]
            out_f = extract_form_tokens(model_answer)
            missing += ["form " + w for w in want_f if not contains_str(out_f, w)]
            n += len(want_f)
        if at == "date_and_pct":
            want_r = extract_rates(gold)
            out_r = extract_rates(model_answer)
            missing += ["rate %s%%" % go_fmt_float(w) for w in want_r if not contains_float(out_r, w)]
            n += len(want_r)
        return finish(at, gold, model_answer, missing, n)

    if at == "days":
        want = extract_days(gold)
        if not want:
            return ungradable(at, "no day count in gold")
        out = extract_days(model_answer)
        missing = ["%d days" % w for w in want if not contains_float(out, w)]
        return finish(at, gold, model_answer, missing, len(want))

    if at in ("act_selection", "act_and_section"):
        want_acts = extract_act_years(gold)
        if not want_acts:
            return ungradable(at, "no act year in gold")
        out_acts = extract_act_years(model_answer)
        missing = ["Act " + w for w in want_acts if not contains_str(out_acts, w)]
        n = len(want_acts)
        if at == "act_and_section":
            want_refs = extract_section_refs(gold)
            out_refs = extract_section_refs(model_answer)
            missing += ["section " + w.string() for w in want_refs if not section_matches(w, out_refs)]
            n += len(want_refs)
        return finish(at, gold, model_answer, missing, n)

    if at == "section_and_code":
        want_refs = extract_section_refs(gold)
        want_codes = extract_codes(gold)
        if not want_refs or not want_codes:
            return ungradable(at, "missing section or code in gold")
        out_refs = extract_section_refs(model_answer)
        out_codes = extract_codes(model_answer)
        missing = []
        n = 0
        for w in want_refs:
            if not section_matches(w, out_refs):
                missing.append("section " + w.string())
            n += 1
        for w in want_codes:
            if not contains_str(out_codes, w):
                missing.append("code " + w)
            n += 1
        return finish(at, gold, model_answer, missing, n)

    if at == "yes_no_reasoned":
        pol = extract_polarity(gold)
        if pol == "":
            return ungradable(at, "no yes/no polarity in gold")
        want_pol = pol
        out_pol = extract_polarity(model_answer)
        if out_pol == "" or out_pol != want_pol:
            return GradeResult(passed=False, score_parts=0.0, graded=True,
                               missing_gold=["polarity " + want_pol],
                               extract_gold="polarity=" + want_pol + " " + entities_summary(gold),
                               extract_model=entities_summary(model_answer))
        want_amt = extract_amounts(gold)
        out_amt = extract_amounts(model_answer)
        missing = ["amount Rs %s" % go_fmt_float(w) for w in want_amt if not contains_float(out_amt, w)]
        return finish(at, gold, model_answer, missing, 1 + len(want_amt))

    if at == "multi_part":
        return grade_multi_part(t, model_answer)

    return ungradable(at, "unknown answer_type")


if __name__ == "__main__":
    import sys

    split = sys.argv[1] if len(sys.argv) > 1 else "data/splits/test.jsonl"
    tasks = []
    with open(split) as f:
        for line in f:
            line = line.strip()
            if line:
                tasks.append(Task.from_json(json.loads(line)))
    ok = bad = nonnum = ungrad = 0
    for t in tasks:
        g = grade(t, t.answer)
        if t.answer_type == "reasoned":
            continue
        if not g.graded and "no section ref in gold" in g.reason and not any(c.isdigit() for c in t.answer):
            nonnum += 1
            continue
        if not g.graded:
            ungrad += 1
            print("UNGRADABLE", t.id, t.answer_type, g.reason)
            continue
        if g.passed:
            ok += 1
        else:
            bad += 1
            if bad <= 10:
                print("SELF-FAIL", t.id, t.answer_type, repr(t.answer[:80]), g.missing_gold)
    print("self-extraction: %d ok, %d fail, %d non-numeric excluded, %d ungradable" % (ok, bad, nonnum, ungrad))
