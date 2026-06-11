#!/usr/bin/env python3
"""
detect-structural.py — Detect structural AI writing patterns in markdown files.

Measures:
- Sentence length variance (burstiness)
- Paragraph length uniformity
- Parallelism (repeated syntactic openings)
- List-to-prose ratio
- Colon density
- Dash density
- Sentence opening diversity

Usage:
    python3 detect-structural.py <file-or-dir> [file-or-dir ...] [--json] [--threshold=strict]

Accepts: single file, multiple files, directories (scans *.md recursively).
Exit codes: 0 = clean, 1 = issues found, 2 = usage error
"""

import sys
import re
import json
import statistics
from pathlib import Path
from collections import Counter

# --- Thresholds ---
THRESHOLDS = {
    "strict": {
        "sentence_length_std_min": 5.0,   # sentences should vary this much
        "paragraph_length_std_min": 20.0,  # paragraphs should vary
        "parallelism_max_repeats": 2,      # max consecutive same-opening sentences
        "list_ratio_max": 0.25,            # max fraction of lines that are list items
        "colon_density_max": 3.0,          # max colons per 500 words
        "dash_density_max": 2.0,           # max em-dashes per 500 words
        "opening_diversity_min": 0.7,      # min ratio of unique openings to total sentences
    },
    "medium": {
        "sentence_length_std_min": 4.0,
        "paragraph_length_std_min": 15.0,
        "parallelism_max_repeats": 2,
        "list_ratio_max": 0.30,
        "colon_density_max": 4.0,
        "dash_density_max": 3.0,
        "opening_diversity_min": 0.6,
    },
    "relaxed": {
        "sentence_length_std_min": 3.0,
        "paragraph_length_std_min": 10.0,
        "parallelism_max_repeats": 3,
        "list_ratio_max": 0.40,
        "colon_density_max": 5.0,
        "dash_density_max": 4.0,
        "opening_diversity_min": 0.5,
    },
}


def extract_prose(text: str, filepath: str = "") -> str:
    """Strip markup and extract prose. Handles markdown and YAML."""
    if filepath.endswith((".yaml", ".yml")):
        return _extract_yaml_prose(text)
    return _extract_markdown_prose(text)


def _extract_yaml_prose(text: str) -> str:
    """Extract prose from YAML multi-line string blocks (| and >)."""
    lines = text.split("\n")
    prose_lines = []
    in_block = False
    block_indent = 0

    for line in lines:
        stripped = line.rstrip()
        if not in_block:
            # Detect start of a multi-line string block
            if re.match(r"^\s*\w[\w_]*:\s*[|>]\s*$", stripped):
                in_block = True
                block_indent = len(line) - len(line.lstrip()) + 2
                continue
            # Skip scalar key: value lines
            continue
        else:
            # Still inside a multi-line block if indented enough
            if stripped == "":
                prose_lines.append("")
                continue
            current_indent = len(line) - len(line.lstrip())
            if current_indent >= block_indent:
                prose_lines.append(line[block_indent:])
            else:
                in_block = False
                # Check if this line starts a new block
                if re.match(r"^\s*\w[\w_]*:\s*[|>]\s*$", stripped):
                    in_block = True
                    block_indent = len(line) - len(line.lstrip()) + 2

    return "\n".join(prose_lines)


def _extract_markdown_prose(text: str) -> str:
    """Strip markdown headers, code blocks, frontmatter, and metadata."""
    lines = text.split("\n")
    prose_lines = []
    in_code_block = False
    in_frontmatter = False

    for i, line in enumerate(lines):
        # Frontmatter
        if i == 0 and line.strip() == "---":
            in_frontmatter = True
            continue
        if in_frontmatter:
            if line.strip() == "---":
                in_frontmatter = False
            continue

        # Code blocks
        if line.strip().startswith("```"):
            in_code_block = not in_code_block
            continue
        if in_code_block:
            continue

        # Skip headers
        if line.strip().startswith("#"):
            continue

        # Skip HTML comments
        if line.strip().startswith("<!--"):
            continue

        # Skip image/link-only lines
        if re.match(r"^\s*!\[.*\]\(.*\)\s*$", line):
            continue

        prose_lines.append(line)

    return "\n".join(prose_lines)


def split_sentences(text: str) -> list:
    """Split text into sentences. Handles abbreviations roughly."""
    # Replace common abbreviations to avoid false splits
    text = re.sub(r"\b(e\.g|i\.e|etc|vs|Dr|Mr|Mrs|Ms|Jr|Sr)\.", r"\1<DOT>", text)
    sentences = re.split(r"[.!?]+(?=\s|$)", text)
    sentences = [s.strip().replace("<DOT>", ".") for s in sentences if s.strip()]
    # Filter out very short fragments (< 4 words)
    return [s for s in sentences if len(s.split()) >= 4]


def split_paragraphs(text: str) -> list:
    """Split into paragraphs (separated by blank lines)."""
    paragraphs = re.split(r"\n\s*\n", text)
    return [p.strip() for p in paragraphs if p.strip() and len(p.split()) >= 5]


def count_list_lines(text: str) -> tuple:
    """Count lines that are list items vs total non-empty lines."""
    lines = [l for l in text.split("\n") if l.strip()]
    list_lines = [l for l in lines if re.match(r"^\s*[-*+•]\s|^\s*\d+[.)]\s", l)]
    return len(list_lines), len(lines)


def get_sentence_openings(sentences: list) -> list:
    """Extract first 3 words of each sentence (lowercased)."""
    openings = []
    for s in sentences:
        words = s.split()[:3]
        openings.append(" ".join(words).lower().rstrip(",;:"))
    return openings


def detect_parallelism(openings: list, max_repeats: int) -> list:
    """Find runs of consecutive sentences with the same opening pattern."""
    issues = []
    if len(openings) < 3:
        return issues

    run_start = 0
    for i in range(1, len(openings)):
        # Check if opening matches (first 2 words)
        prev_prefix = " ".join(openings[i - 1].split()[:2])
        curr_prefix = " ".join(openings[i].split()[:2])

        if prev_prefix == curr_prefix and len(prev_prefix) > 2:
            # Continue run
            pass
        else:
            run_length = i - run_start
            if run_length > max_repeats:
                issues.append({
                    "type": "parallelism",
                    "detail": f"{run_length} consecutive sentences start with '{openings[run_start].split()[0]}...'",
                    "severity": "high" if run_length > 3 else "medium",
                    "position": f"sentences {run_start + 1}-{i}",
                })
            run_start = i

    # Check final run
    run_length = len(openings) - run_start
    if run_length > max_repeats:
        issues.append({
            "type": "parallelism",
            "detail": f"{run_length} consecutive sentences start with '{openings[run_start].split()[0]}...'",
            "severity": "high" if run_length > 3 else "medium",
            "position": f"sentences {run_start + 1}-{len(openings)}",
        })

    return issues


def analyze(text: str, threshold_name: str = "medium", filepath: str = "") -> dict:
    """Run all structural checks. Return issues dict."""
    thresholds = THRESHOLDS[threshold_name]
    prose = extract_prose(text, filepath)
    issues = []
    metrics = {}

    if len(prose.split()) < 50:
        return {"issues": [], "metrics": {"word_count": len(prose.split())}, "verdict": "too-short"}

    sentences = split_sentences(prose)
    paragraphs = split_paragraphs(prose)
    word_count = len(prose.split())

    # --- Sentence length variance (burstiness) ---
    if len(sentences) >= 5:
        lengths = [len(s.split()) for s in sentences]
        std = statistics.stdev(lengths)
        mean = statistics.mean(lengths)
        metrics["sentence_length_mean"] = round(mean, 1)
        metrics["sentence_length_std"] = round(std, 1)

        if std < thresholds["sentence_length_std_min"]:
            issues.append({
                "type": "low-burstiness",
                "detail": f"Sentence length std={std:.1f} (threshold: >{thresholds['sentence_length_std_min']}). Text is unnaturally uniform.",
                "severity": "high",
                "metric": std,
            })

    # --- Paragraph length uniformity ---
    if len(paragraphs) >= 3:
        para_lengths = [len(p.split()) for p in paragraphs]
        para_std = statistics.stdev(para_lengths)
        metrics["paragraph_length_std"] = round(para_std, 1)

        if para_std < thresholds["paragraph_length_std_min"]:
            issues.append({
                "type": "uniform-paragraphs",
                "detail": f"Paragraph length std={para_std:.1f} (threshold: >{thresholds['paragraph_length_std_min']}). Paragraphs are suspiciously similar in length.",
                "severity": "medium",
                "metric": para_std,
            })

    # --- Parallelism ---
    openings = get_sentence_openings(sentences)
    parallelism_issues = detect_parallelism(openings, thresholds["parallelism_max_repeats"])
    issues.extend(parallelism_issues)

    # --- Opening diversity ---
    if len(openings) >= 5:
        first_words = [o.split()[0] if o.split() else "" for o in openings]
        unique_ratio = len(set(first_words)) / len(first_words)
        metrics["opening_diversity"] = round(unique_ratio, 2)

        if unique_ratio < thresholds["opening_diversity_min"]:
            # Find the most repeated openings
            counter = Counter(first_words)
            top = counter.most_common(3)
            issues.append({
                "type": "low-opening-diversity",
                "detail": f"Only {unique_ratio:.0%} of sentences start with unique words. Most common: {top}",
                "severity": "medium",
                "metric": unique_ratio,
            })

    # --- List ratio ---
    list_count, total_lines = count_list_lines(text)
    if total_lines > 0:
        list_ratio = list_count / total_lines
        metrics["list_ratio"] = round(list_ratio, 2)

        if list_ratio > thresholds["list_ratio_max"]:
            issues.append({
                "type": "list-heavy",
                "detail": f"{list_ratio:.0%} of lines are list items (threshold: <{thresholds['list_ratio_max']:.0%}). Over-reliance on lists.",
                "severity": "low",
                "metric": list_ratio,
            })

    # --- Colon density ---
    colon_count = prose.count(":")
    colon_density = (colon_count / word_count) * 500 if word_count > 0 else 0
    metrics["colon_density_per_500w"] = round(colon_density, 1)

    if colon_density > thresholds["colon_density_max"]:
        issues.append({
            "type": "colon-heavy",
            "detail": f"{colon_density:.1f} colons per 500 words (threshold: <{thresholds['colon_density_max']}). AI over-uses colons as introducers.",
            "severity": "low",
            "metric": colon_density,
        })

    # --- Dash density ---
    dash_count = len(re.findall(r"[—–]", prose))
    dash_density = (dash_count / word_count) * 500 if word_count > 0 else 0
    metrics["dash_density_per_500w"] = round(dash_density, 1)

    if dash_density > thresholds["dash_density_max"]:
        issues.append({
            "type": "dash-heavy",
            "detail": f"{dash_density:.1f} dashes per 500 words (threshold: <{thresholds['dash_density_max']}). AI favors em-dashes.",
            "severity": "low",
            "metric": dash_density,
        })

    # --- Tricolon density ---
    # AI gravitates toward groups of three (X, Y, and Z). Human writers use pairs
    # or irregular counts. Density above 3 per 500 words is suspicious.
    # Word-level: "X, Y, and Z"
    word_tricolons = re.findall(r"\w+,\s+\w+,\s+and\s+\w+", prose)
    # Clause-level: three comma-separated clauses each containing a wh-word
    # Catches rhetorical parallelism like "which capabilities may cross it, under what constraints, authorized by which principal"
    wh_clause = r"[^,]{0,40}(?:which|what|how|where|when|whether|who|whom)[^,]+"
    clause_tricolons = re.findall(
        wh_clause + r",\s+" + wh_clause + r",\s+(?:and\s+)?" + wh_clause,
        prose, re.IGNORECASE
    )
    tricolon_count = len(word_tricolons) + len(clause_tricolons)
    tricolon_density = (tricolon_count / word_count) * 500 if word_count > 0 else 0
    metrics["tricolon_density_per_500w"] = round(tricolon_density, 1)

    if tricolon_density > thresholds.get("tricolon_density_max", 3.0):
        issues.append({
            "type": "tricolon-heavy",
            "detail": f"{tricolon_density:.1f} tricolons per 500 words (threshold: <3.0). AI defaults to groups of three.",
            "severity": "low",
            "metric": tricolon_density,
        })

    # --- Wh-parallelism (2-clause) ---
    # Two comma-separated clauses each containing a wh-word.
    # Catches "what may cross it and on whose authority" and similar paired rhetorical structures.
    # Less severe than tricolon but still an AI tell when dense.
    wh_pair_pattern = wh_clause + r",\s+" + wh_clause
    # Exclude matches already counted as tricolons (3-clause) by checking no third wh-clause follows
    wh_pairs_raw = re.findall(wh_pair_pattern, prose, re.IGNORECASE)
    wh_pair_count = max(0, len(wh_pairs_raw) - len(clause_tricolons))
    wh_pair_density = (wh_pair_count / word_count) * 500 if word_count > 0 else 0
    metrics["wh_pair_density_per_500w"] = round(wh_pair_density, 1)

    if wh_pair_density > thresholds.get("wh_pair_density_max", 2.0):
        issues.append({
            "type": "wh-parallelism-heavy",
            "detail": f"{wh_pair_density:.1f} wh-clause pairs per 500 words (threshold: <2.0). AI favors parallel wh-constructions.",
            "severity": "low",
            "metric": wh_pair_density,
        })

    # --- Exhaustive list density ---
    # AI produces comma-separated lists of 5+ items within a single clause.
    # Human writers rarely enumerate more than 3-4 inline; longer lists go into
    # bullet points or tables.
    exhaustive_lists = re.findall(
        r"(?:\w+(?:\s+\w+){0,2},\s+){4,}\w+(?:\s+\w+){0,2}",
        prose
    )
    exhaustive_count = len(exhaustive_lists)
    exhaustive_density = (exhaustive_count / word_count) * 500 if word_count > 0 else 0
    metrics["exhaustive_list_density_per_500w"] = round(exhaustive_density, 1)

    if exhaustive_density > thresholds.get("exhaustive_list_density_max", 1.5):
        issues.append({
            "type": "exhaustive-list-heavy",
            "detail": f"{exhaustive_density:.1f} exhaustive inline lists (5+ items) per 500 words (threshold: <1.5). AI over-enumerates.",
            "severity": "low",
            "metric": exhaustive_density,
        })

    # --- Parenthetical definition density ---
    # AI inserts inline definitions in parentheses: "the orchestrator (the component
    # responsible for...)" at unnaturally high frequency.
    paren_def_count = len(re.findall(r"\w+\s+\([^)]{10,}[^)]*\)", prose))
    paren_def_density = (paren_def_count / word_count) * 500 if word_count > 0 else 0
    metrics["paren_def_density_per_500w"] = round(paren_def_density, 1)

    if paren_def_density > thresholds.get("paren_def_density_max", 4.0):
        issues.append({
            "type": "paren-def-heavy",
            "detail": f"{paren_def_density:.1f} parenthetical definitions per 500 words (threshold: <4.0). AI over-defines inline.",
            "severity": "low",
            "metric": paren_def_density,
        })

    # --- Passive enabling verb density ---
    # Clustered "is achieved", "is enabled", "is realized", "is facilitated" etc.
    passive_enabling = len(re.findall(
        r"\bis\s+(achieved|enabled|realized|facilitated|accomplished|attained|ensured|maintained|provided|supported)\b",
        prose, re.IGNORECASE
    ))
    passive_density = (passive_enabling / word_count) * 500 if word_count > 0 else 0
    metrics["passive_enabling_per_500w"] = round(passive_density, 1)

    if passive_density > thresholds.get("passive_enabling_max", 2.0):
        issues.append({
            "type": "passive-enabling-heavy",
            "detail": f"{passive_density:.1f} passive enabling verbs per 500 words (threshold: <2.0). AI avoids naming the actor.",
            "severity": "medium",
            "metric": passive_density,
        })

    # --- "rather than" frequency ---
    # AI uses "rather than" at 2-3x the natural rate to set up every contrast.
    rather_than_count = len(re.findall(r"\brather than\b", prose, re.IGNORECASE))
    rather_than_density = (rather_than_count / word_count) * 500 if word_count > 0 else 0
    metrics["rather_than_per_500w"] = round(rather_than_density, 1)

    if rather_than_density > thresholds.get("rather_than_max", 2.0):
        issues.append({
            "type": "rather-than-heavy",
            "detail": f"{rather_than_density:.1f} 'rather than' per 500 words (threshold: <2.0). AI's default contrast device.",
            "severity": "low",
            "metric": rather_than_density,
        })

    # --- "both X and Y" frequency ---
    # AI produces balanced pair constructions at unnaturally high density.
    both_and_count = len(re.findall(r"\bboth\s+\w+\s+and\s+\w+", prose, re.IGNORECASE))
    both_and_density = (both_and_count / word_count) * 500 if word_count > 0 else 0
    metrics["both_and_per_500w"] = round(both_and_density, 1)

    if both_and_density > thresholds.get("both_and_max", 1.5):
        issues.append({
            "type": "both-and-heavy",
            "detail": f"{both_and_density:.1f} 'both X and Y' per 500 words (threshold: <1.5). AI over-balances pairs.",
            "severity": "low",
            "metric": both_and_density,
        })

    # --- Verdict ---
    high_count = sum(1 for i in issues if i.get("severity") == "high")
    med_count = sum(1 for i in issues if i.get("severity") == "medium")

    if high_count >= 2 or (high_count >= 1 and med_count >= 2):
        verdict = "likely-ai"
    elif high_count >= 1 or med_count >= 2:
        verdict = "suspicious"
    elif issues:
        verdict = "minor-issues"
    else:
        verdict = "clean"

    metrics["word_count"] = word_count
    metrics["sentence_count"] = len(sentences)
    metrics["paragraph_count"] = len(paragraphs)

    return {"issues": issues, "metrics": metrics, "verdict": verdict}


def main():
    if len(sys.argv) < 2:
        print("Usage: detect-structural.py <file-or-dir> [file-or-dir ...] [--json] [--threshold=strict|medium|relaxed]", file=sys.stderr)
        sys.exit(2)

    json_mode = "--json" in sys.argv
    threshold = "strict"

    # Separate flags from paths
    paths = []
    for arg in sys.argv[1:]:
        if arg.startswith("--threshold="):
            threshold = arg.split("=")[1]
            if threshold not in THRESHOLDS:
                print(f"Error: Unknown threshold '{threshold}'. Use: strict, medium, relaxed", file=sys.stderr)
                sys.exit(2)
        elif arg == "--json":
            continue
        else:
            paths.append(arg)

    if not paths:
        print("Usage: detect-structural.py <file-or-dir> [file-or-dir ...] [--json] [--threshold=strict|medium|relaxed]", file=sys.stderr)
        sys.exit(2)

    # Resolve all paths into a list of .md files
    files = []
    for p in paths:
        path = Path(p)
        if path.is_dir():
            files.extend(sorted(path.rglob("*.md")))
            files.extend(sorted(path.rglob("*.yaml")))
            files.extend(sorted(path.rglob("*.yml")))
        elif path.is_file():
            files.append(path)
        else:
            print(f"Error: Not found: {p}", file=sys.stderr)
            sys.exit(2)

    if not files:
        print("Error: No .md or .yaml files found in the given paths.", file=sys.stderr)
        sys.exit(2)

    any_issues = False
    all_results = []

    for filepath in files:
        text = filepath.read_text(encoding="utf-8")
        result = analyze(text, threshold, str(filepath))

        if result["issues"]:
            any_issues = True

        if json_mode:
            all_results.append({"file": str(filepath), **result})
        else:
            print(f"=== Structural AI Detection: {filepath} ===")
            print(f"    Threshold: {threshold}")
            print(f"    Verdict: {result['verdict'].upper()}")
            print()

            print("--- Metrics ---")
            for k, v in result["metrics"].items():
                print(f"  {k}: {v}")
            print()

            if result["issues"]:
                print("--- Issues ---")
                for issue in result["issues"]:
                    sev = issue["severity"].upper()
                    print(f"  [{sev}] {issue['type']}: {issue['detail']}")
                    if "position" in issue:
                        print(f"         at {issue['position']}")
                print()
            else:
                print("✓ No structural AI patterns detected.")
                print()

    if json_mode:
        print(json.dumps(all_results if len(all_results) > 1 else all_results[0], indent=2))

    sys.exit(1 if any_issues else 0)


if __name__ == "__main__":
    main()
