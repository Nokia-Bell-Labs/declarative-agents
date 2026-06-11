#!/usr/bin/env bash
# detect-lexical.sh — Scan markdown files for AI writing giveaway words/phrases
# Usage: ./detect-lexical.sh <file-or-dir> [file-or-dir ...] [--json]
#
# Accepts: single file, multiple files, directories (scans *.md recursively).
# Outputs line-numbered matches grouped by category.
# Exit code: 0 = clean, 1 = issues found, 2 = usage error

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <file-or-dir> [file-or-dir ...] [--json]" >&2
  exit 2
fi

# Separate flags from paths
JSON_MODE=""
declare -a PATHS=()
for arg in "$@"; do
  if [[ "$arg" == "--json" ]]; then
    JSON_MODE="--json"
  else
    PATHS+=("$arg")
  fi
done

if [[ ${#PATHS[@]} -eq 0 ]]; then
  echo "Usage: $0 <file-or-dir> [file-or-dir ...] [--json]" >&2
  exit 2
fi

# Resolve all paths into a list of .md files
declare -a FILES=()
for p in "${PATHS[@]}"; do
  if [[ -d "$p" ]]; then
    while IFS= read -r -d '' f; do
      FILES+=("$f")
    done < <(find "$p" \( -name '*.md' -o -name '*.yaml' -o -name '*.yml' \) -type f -print0 | sort -z)
  elif [[ -f "$p" ]]; then
    FILES+=("$p")
  else
    echo "Error: Not found: $p" >&2
    exit 2
  fi
done

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "Error: No .md or .yaml files found in the given paths." >&2
  exit 2
fi

GLOBAL_EXIT=0
ISSUES_FOUND=0
CANDIDATES_FOUND=0
declare -a RESULTS=()

# --- Category: Banned adjectives/adverbs (from writing-style-guide.md) ---
BANNED_WORDS=(
  "critical" "critically"
  "key"
  "deliberate" "deliberatively"
  "correctly"
  "sound"
  "strategic" "strategically"
  "precisely"
  "absolutely"
  "fundamental" "fundamentally"
  "breakthrough"
  "principled"
  "standards-aligned"
  "honest"
  "grounded"
  "concrete"
  "distinction"
  "cleanly" "neatly"
  "sharp" "sharpen"
  "underpins" "underpinning"
  "dovetails"
  "illuminates" "illuminating"
  "overarching"
  "interplay"
  "salient"
  "delineate" "delineating"
  "encapsulate" "encapsulates"
  "myriad"
  "plethora"
  "burgeoning"
  "nascent"
  "hinges on"
)

# --- Category: AI cliché phrases ---
AI_PHRASES=(
  "at the heart of"
  "it's worth noting"
  "it is worth noting"
  "it's important to note"
  "it is important to note"
  "it bears mentioning"
  "let's consider"
  "let's break this down"
  "let's think about"
  "let me explain"
  "in this section, we will"
  "to put it differently"
  "simply put"
  "to put it simply"
  "in other words"
  "the question then becomes"
  "this raises the question"
  "which brings us to"
  "this brings us to"
  "one might argue"
  "some might say"
  "the key takeaway"
  "there are [0-9]+ main"
  "there are several"
  "this means that"
  "this implies that"
  "this suggests that"
  "this ensures that"
  "this enables"
  "this allows"
  "this provides"
  "this represents"
  "this highlights"
  "this underscores"
  "this demonstrates"
  "in summary"
  "in conclusion"
  "to summarize"
  "as mentioned earlier"
  "as noted above"
  "as discussed"
  "it should be noted"
  "it is evident"
  "it becomes clear"
  "it is clear that"
  "needless to say"
  "without a doubt"
  "undeniably"
  "undoubtedly"
  "unquestionably"
  "comprehensive"
  "holistic"
  "holistically"
  "robust"
  "robustly"
  "seamless"
  "seamlessly"
  "leverage"
  "leveraging"
  "utilize"
  "utilizing"
  "facilitate"
  "facilitating"
  "empower"
  "empowering"
  "enhance"
  "enhancing"
  "foster"
  "fostering"
  "navigate"
  "navigating"
  "landscape"
  "paradigm"
  "ecosystem"
  "synergy"
  "transformative"
  "cutting-edge"
  "state-of-the-art"
  "game-changing"
  "groundbreaking"
  "innovative"
  "revolutionize"
  "revolutionizing"
  "delve"
  "delving"
  "realm"
  "tapestry"
  "multifaceted"
  "intricate"
  "intricacies"
  "nuanced"
  "nuances"
  "pivotal"
  "moreover"
  "furthermore"
  "additionally"
  "consequently"
  "nevertheless"
  "nonetheless"
  "henceforth"
  "thereby"
  "wherein"
  "thereof"
  "albeit"
  "inasmuch"
  "coupled with"
  "in tandem"
  "advent"
  "akin to"
  "renders"
  "warrants"
  "dictates"
  "speaks to"
  "constitutes"
  "manifests"
  "affords"
  "it is worth emphasizing"
  "it is no coincidence"
  "it is precisely this"
  "strikes a balance"
  "stands in contrast"
  "lends itself to"
  "gives rise to"
  "paves the way"
  "a testament to"
  "is tantamount to"
  "by the same token"
  "in light of"
  "in the context of"
  "in a manner that"
  "to that end"
  "to this end"
  "along these lines"
  "with this in mind"
  "bears emphasizing"
  "merits attention"
  "worthy of note"
  "the crux of the matter"
  "the key insight"
  "the upshot is"
  "the takeaway is"
  "what emerges is"
  "at a high level"
  "zooming out"
  "zooming in"
  "stepping back"
  "put differently"
  "stated differently"
  "viewed through this lens"
  "through the lens of"
  "taken together"
  "in doing so"
  "in this way"
  "in effect"
  "orthogonal"
  "non-trivial"
  "out of the box"
  "under the hood"
  "at scale"
)

# --- Category: False emphasis adverbs ---
FALSE_EMPHASIS=(
  "crucially"
  "notably"
  "importantly"
  "significantly"
  "remarkably"
  "interestingly"
  "essentially"
  "at its core"
  "ultimately"
  "inherently"
  "particularly"
)

# --- Category: Mechanical transitions ---
MECHANICAL_TRANSITIONS=(
  "^first,"
  "^second,"
  "^third,"
  "^finally,"
  "^in addition,"
  "^on one hand"
  "^on the other hand"
  "^while this is true"
  "^having said that"
  "^that being said"
  "^with that in mind"
  "^with this in place"
  "^given this,"
  "^that said,"
  "^and so,"
  "^moving on"
  "^turning to"
  "^building on"
  "^to begin with"
)

# --- Category: CoT structural patterns (definite) ---
COT_STRUCTURAL=(
  "is not a monolithic"
  "is not a simple"
  "is not a trivial"
  "is not a single"
  "is not merely"
  "is not just a"
  "is not simply a"
  "is not simply that"
  "are not merely"
  "are not just"
  "are not simply"
  "this is not a"
  # Academic positioning: "We adopt/extend/refine X"
  "[Ww]e adopt .* and extend"
  "[Ww]e adopt .* and expand"
  "[Ww]e adopt .* and refine"
  "[Ww]e extend .* with"
  "[Ww]e build on .* and extend"
  "[Ww]e take .* as a starting point"
  "[Ww]e adopt this .* and"
  # Define-by-negation: "not X in the abstract. It is Y"
  "not .* in the abstract"
  "is not .* per se[.;,]"
  "not merely .*[.;] It is"
  "not .* in isolation[.;,]"
  # Interpretation bridges: "The analogy/parallel/point is direct/clear"
  "[Tt]he analogy .* is "
  "[Tt]he parallel .* is "
  "[Tt]he comparison .* is "
  "[Tt]he implication .* is "
  "[Tt]he consequence .* is "
  "[Tt]he takeaway .* is "
  "[Tt]he insight .* is "
  "[Tt]he point .* is "
  "[Tt]he difference .* is "
  "[Tt]he distinction .* is "
)

# --- Category: CoT candidates (broad, need LLM verification) ---
# These are common CoT scaffolding shapes but also appear in legitimate prose.
# Flagged as candidates for the semantic pass to confirm or dismiss.
# Patterns match after sentence boundaries (. ! ?) since markdown paragraphs
# are single long lines.
COT_CANDIDATES=(
  '[.!?] This .* is '
  '[.!?] These .* are '
  '[.!?] That .* is '
  '[.!?] It is a '
  '[.!?] It is the '
  '[.!?] It is an '
  'What .* is '
  'Consider '
  'not only .* but'
  '[Tt]wo distinct '
  '[Tt]hree .* together '
  '[Tt]here are [a-z]* [a-z]* that '
  '^[Ww]hile .*, '
  'whether .* or '
  '[Ee]ach of these '
  '[Aa]ll of these '
)

scan_patterns() {
  local category="$1"
  shift
  local patterns=("$@")

  for pattern in "${patterns[@]}"; do
    # Case-insensitive grep with line numbers
    local matches
    matches=$(grep -in "$pattern" "$FILE" 2>/dev/null || true)
    if [[ -n "$matches" ]]; then
      ISSUES_FOUND=1
      while IFS= read -r line; do
        local lineno="${line%%:*}"
        local content="${line#*:}"
        if [[ "$JSON_MODE" == "--json" ]]; then
          RESULTS+=("{\"line\": $lineno, \"category\": \"$category\", \"pattern\": \"$(echo "$pattern" | sed 's/"/\\"/g')\", \"text\": \"$(echo "$content" | sed 's/"/\\"/g' | head -c 200)\"}")
        else
          printf "  L%-4s [%s] %s\n" "$lineno" "$pattern" "$(echo "$content" | head -c 120)"
        fi
      done <<< "$matches"
    fi
  done
}

# Like scan_patterns but advisory-only: does not set ISSUES_FOUND.
# These need LLM verification before acting on them.
scan_candidates() {
  local category="$1"
  shift
  local patterns=("$@")

  for pattern in "${patterns[@]}"; do
    local matches
    matches=$(grep -in "$pattern" "$FILE" 2>/dev/null || true)
    if [[ -n "$matches" ]]; then
      CANDIDATES_FOUND=1
      while IFS= read -r line; do
        local lineno="${line%%:*}"
        local content="${line#*:}"
        if [[ "$JSON_MODE" == "--json" ]]; then
          RESULTS+=("{\"line\": $lineno, \"category\": \"$category\", \"severity\": \"candidate\", \"pattern\": \"$(echo "$pattern" | sed 's/"/\\"/g')\", \"text\": \"$(echo "$content" | sed 's/"/\\"/g' | head -c 200)\"}")
        else
          printf "  L%-4s [%s] %s\n" "$lineno" "$pattern" "$(echo "$content" | head -c 120)"
        fi
      done <<< "$matches"
    fi
  done
}

run_on_file() {
  local FILE="$1"
  ISSUES_FOUND=0
  CANDIDATES_FOUND=0
  RESULTS=()

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo "=== Lexical AI Detection: $FILE ==="
    echo ""
    echo "--- Banned Words ---"
  fi
  scan_patterns "banned-word" "${BANNED_WORDS[@]}"

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    echo "--- AI Cliché Phrases ---"
  fi
  scan_patterns "ai-cliche" "${AI_PHRASES[@]}"

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    echo "--- False Emphasis ---"
  fi
  scan_patterns "false-emphasis" "${FALSE_EMPHASIS[@]}"

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    echo "--- Mechanical Transitions ---"
  fi
  scan_patterns "mechanical-transition" "${MECHANICAL_TRANSITIONS[@]}"

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    echo "--- CoT Structural Patterns ---"
  fi
  scan_patterns "cot-structural" "${COT_STRUCTURAL[@]}"

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    echo "--- CoT Candidates (needs LLM verification) ---"
  fi
  scan_candidates "cot-candidate" "${COT_CANDIDATES[@]}"

  if [[ "$JSON_MODE" == "--json" ]]; then
    echo "["
    local first=true
    for r in "${RESULTS[@]}"; do
      if [[ "$first" == "true" ]]; then
        echo "  $r"
        first=false
      else
        echo "  ,$r"
      fi
    done
    echo "]"
  fi

  if [[ "$JSON_MODE" != "--json" ]]; then
    echo ""
    if [[ $ISSUES_FOUND -eq 0 && $CANDIDATES_FOUND -eq 0 ]]; then
      echo "✓ No lexical AI patterns detected."
    elif [[ $ISSUES_FOUND -eq 0 && $CANDIDATES_FOUND -eq 1 ]]; then
      echo "⚠ CoT candidates found above. Pass to semantic layer for adjudication."
    else
      echo "✗ Lexical AI patterns found. Review above."
    fi
  fi

  if [[ $ISSUES_FOUND -eq 1 ]]; then
    GLOBAL_EXIT=1
  fi
}

# --- Main: iterate over all resolved files ---
for FILE in "${FILES[@]}"; do
  run_on_file "$FILE"
  if [[ ${#FILES[@]} -gt 1 && "$JSON_MODE" != "--json" ]]; then
    echo ""
  fi
done

exit $GLOBAL_EXIT
