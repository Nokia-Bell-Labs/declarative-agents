PAPER_SLUG = agentic-design-patterns
OUTPUT_DIR  = generated-files
FIGURES_DIR = figures
CSL = templates/ieee.csl

PANDOC   = pandoc
PLANTUML = plantuml
DATE     = $(shell date +%Y-%m-%d)

MD_FILES   = $(sort $(wildcard [0-9][0-9]-*.md))
PUML_FILES = $(sort $(wildcard $(FIGURES_DIR)/*.puml))
PNG_FILES  = $(PUML_FILES:.puml=.png)
PDF        = $(OUTPUT_DIR)/$(PAPER_SLUG)-v$(DATE).pdf

.PHONY: all pdf figures clean

all: pdf

figures: $(PNG_FILES)

$(FIGURES_DIR)/%.png: $(FIGURES_DIR)/%.puml
	$(PLANTUML) -tpng -o . $<

pdf: $(PDF)

$(PDF): $(MD_FILES) $(PNG_FILES) references.yaml templates/ieee-twocolumn.lua templates/ieee-preamble.tex | $(OUTPUT_DIR)
	@echo "Generating $@ (IEEE two-column) from $(MD_FILES)"
	$(PANDOC) --citeproc --csl=$(CSL) --bibliography=references.yaml \
	          --lua-filter=templates/ieee-twocolumn.lua \
	          --include-in-header=templates/ieee-preamble.tex \
	          --from markdown --pdf-engine=xelatex \
	          --syntax-highlighting=idiomatic \
	          $(MD_FILES) -o $@

$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

clean:
	rm -f $(PDF) $(PNG_FILES)
