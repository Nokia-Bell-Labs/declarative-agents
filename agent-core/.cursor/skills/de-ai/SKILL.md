---
name: de-ai
description: 'Detect and fix AI writing patterns recursively. Use when: reviewing text for AI tells, cleaning AI-generated drafts, checking for CoT leakage, measuring text perplexity and burstiness, making text sound human, fixing opening diversity. Triggers: de-ai, ai detection, ai writing, perplexity, burstiness, CoT leakage, humanize text, opening diversity, sentence starts.'
argument-hint: 'Path to file or directory to analyze and fix'
---

# De-AI: Recursive AI Writing Detection and Correction

Detects AI writing patterns at three layers (lexical, structural, semantic) and recursively rewrites flagged passages until they pass all checks.

Handles both markdown (`.md`) and YAML (`.yaml`) files. For YAML, prose is extracted from multi-line string blocks (`|` and `>`).

## When to Use

- Before committing any document drafted or edited by AI
- When reviewing a file for AI writing tells
- When a document reads like a wall of wordy prose
- After AI-assisted writing sessions to clean the output

## Procedure

### Step 1: Lexical Scan

```bash
bash .cursor/skills/de-ai/scripts/detect-lexical.sh <file-or-dir> [file-or-dir ...]
```

Scans `*.md` and `*.yaml` files. Produces line-numbered matches grouped by category: banned words, AI clichés, false emphasis, mechanical transitions, CoT structural patterns, and CoT candidates.

Zero cost, instant results. CoT candidates are advisory — carry them to Step 3 for LLM verification.

### Step 2: Structural Analysis

```bash
python3 .cursor/skills/de-ai/scripts/detect-structural.py <file-or-dir> [file-or-dir ...]
```

Scans `*.md` and `*.yaml` files. For YAML, extracts prose from multi-line string blocks automatically.

Default threshold is `strict`. Use `--threshold=medium` for drafts.

Key signals:
- `sentence_length_std < 5.0` = unnaturally uniform (AI)
- `opening_diversity < 0.7` = repetitive sentence starts (AI)
- `dash_density > 2.0` = em-dash overuse (AI)
- `verdict: likely-ai` or `suspicious` = proceed to Step 3

If `opening_diversity` is flagged, load [opening-diversity-fixes.md](./references/opening-diversity-fixes.md) for 15 rewrite techniques.

### Step 3: Semantic Analysis

Load prompts from [perplexity-prompts.md](./references/perplexity-prompts.md) and run against the target text. Feed ALL lexical and structural output to the semantic prompts:

1. **Vocabulary Predictability** (Prompt 1) — Score each sentence 1-5
2. **Burstiness Assessment** (Prompt 2) — Confirm structural findings
3. **Cross-Sentence Surprise** (Prompt 3) — Detect absence of genuine thought progression
4. **CoT Leakage Detection** (Prompt 4) — Find reasoning scaffolding regex missed

Run Prompts 1-3 in parallel. Run Prompt 4 after reviewing lexical results.

Then run **Prompt 5** (Overall Assessment) with all collected evidence.

### Step 4: Targeted Rewrite

For each flagged passage (priority order from Prompt 5):

1. Load [rewrite instructions](./references/rewrite-instructions.md)
2. For CoT leaks: remove the sentence, re-read the paragraph. No information lost = delete. Information lost = reword.
3. Rewrite ONLY the flagged passage
4. Constraints: preserve meaning, don't introduce new AI patterns

### Step 5: Recursive Validation

After each rewrite:

1. Re-run `detect-lexical.sh` on the rewritten section
2. Re-run `detect-structural.py` on the rewritten section
3. Issues remain AND count decreased: iterate (max 3 total passes)
4. Issues remain AND count same or increased: STOP, flag for human review
5. Clean on both scripts: accept

### Step 6: Final Semantic Verification

After all passages are rewritten, run full semantic analysis one more time on the complete document.

## Convergence Rules

- Maximum 3 rewrite iterations per passage
- If iteration N finds >= issues as iteration N-1, stop immediately
- Never rewrite formal specifications or requirement statements
- When in doubt, flag for human rather than risk meaning loss

## Quick Mode (Lexical + Structural Only)

```bash
bash .cursor/skills/de-ai/scripts/detect-lexical.sh <file-or-dir>
python3 .cursor/skills/de-ai/scripts/detect-structural.py <file-or-dir> --json
```

Catches ~60% of AI patterns at zero cost.

## Reference Documents

- [Banned patterns database](./references/banned-patterns.md)
- [CoT leakage patterns](./references/cot-leakage-patterns.md)
- [Opening diversity fixes](./references/opening-diversity-fixes.md)
- [Perplexity proxy prompts](./references/perplexity-prompts.md)
- [Rewrite instructions](./references/rewrite-instructions.md)
- [Report template](./assets/report-template.md)
