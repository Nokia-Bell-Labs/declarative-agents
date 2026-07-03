-- ieee-twocolumn.lua
-- Two-column (IEEEtran) layout for pandoc output:
--   * Mermaid diagrams     -> full-width figure* floats spanning BOTH columns,
--                             scaled to fit, with the document's manual caption
--                             kept directly beneath the image;
--   * multi-column tables   -> full-width table* floats with wrapping columns;
--   * stray 1-column "caption tables" with no diagram -> centered caption text.
--
-- Why this is needed: pandoc renders tables as longtable (illegal in two-column)
-- and renders the mermaid images inline (so wide diagrams overflow one column).
-- This filter rewrites both to raw LaTeX. Because the Image/Table nodes are gone
-- before the writer runs, ieee-preamble.tex supplies graphicx/adjustbox/booktabs/
-- tabularx itself.

local function blocks_to_inline_latex(blocks)
  local s = pandoc.write(pandoc.Pandoc(blocks), 'latex')
  return (s:gsub('^%s+', ''):gsub('%s+$', ''):gsub('%s*\n%s*', ' '))
end

-- Join every cell of a single-column table into one caption string,
-- stripping any leading \textbf{Figure N.} or \textbf{Table N.} prefix
-- so LaTeX's \caption{} can supply the label and number itself.
local function caption_table_text(tbl)
  local parts = {}
  for _, r in ipairs(tbl.head.rows) do
    parts[#parts + 1] = blocks_to_inline_latex(r.cells[1].contents)
  end
  for _, body in ipairs(tbl.bodies) do
    for _, r in ipairs(body.body) do
      parts[#parts + 1] = blocks_to_inline_latex(r.cells[1].contents)
    end
  end
  local s = table.concat(parts, ' ')
  -- strip \textbf{Figure N.} or \textbf{Table N.} prefix (with optional trailing space)
  s = s:gsub('^\\textbf{%a[^}]*%d+%.}%s*', '')
  return s
end

-- Pull the image out of a Para/Plain-wrapped image or a pandoc Figure block.
local function image_from_block(b)
  if (b.t == 'Para' or b.t == 'Plain') and #b.content == 1 and b.content[1].t == 'Image' then
    return b.content[1]
  end
  if b.t == 'Figure' then
    local found
    pandoc.walk_block(b, { Image = function(im) found = found or im end })
    return found
  end
  return nil
end

-- Build a full-width figure* float around the *pandoc* Image element (kept intact
-- so pandoc still decodes mermaid's base64 data-URI to a temp file for graphicx).
-- The image is wrapped in raw adjustbox so wide diagrams scale to fit.
local function figure_blocks(img, captex)
  img.caption = {}              -- avoid pandoc's implicit-figure wrapping
  -- Markers at end of caption (stripped before rendering):
  --   {wide}      → figure*, max width=\textwidth
  --   {wide 0.6}  → figure*, width=0.6\textwidth
  --   {0.7}       → figure,  width=0.7\columnwidth
  --   (none)      → figure,  max width=\columnwidth
  local wide = false
  local wide_frac = nil
  local col_frac = nil
  if captex then
    local stripped, frac = captex:match('^(.-)%s*\\{wide%s+([%d%.]+)\\}%s*$')
    if not stripped then
      stripped, frac = captex:match('^(.-)%s*{wide%s+([%d%.]+)}%s*$')
    end
    if stripped then
      wide = true
      wide_frac = frac
      captex = stripped
    else
      local s = captex:match('^(.-)%s*\\{wide\\}%s*$')
               or captex:match('^(.-)%s*{wide}%s*$')
      if s then
        wide = true
        captex = s
      else
        local s2, f2 = captex:match('^(.-)%s*\\{([%d%.]+)\\}%s*$')
        if not s2 then s2, f2 = captex:match('^(.-)%s*{([%d%.]+)}%s*$') end
        if s2 then
          col_frac = f2
          captex = s2
        end
      end
    end
  end
  local env = wide and 'figure*' or 'figure'
  local maxw
  if wide_frac then
    maxw = 'width=' .. wide_frac .. '\\textwidth'
  elseif wide then
    maxw = 'max width=\\textwidth'
  elseif col_frac then
    maxw = 'width=' .. col_frac .. '\\columnwidth'
  else
    maxw = 'max width=\\columnwidth'
  end
  local tail = '\\end{adjustbox}'
  if captex and captex ~= '' then
    tail = tail .. '\n\\caption{' .. captex .. '}'
  end
  tail = tail .. '\n\\end{' .. env .. '}'
  return {
    pandoc.RawBlock('latex',
      '\\begin{' .. env .. '}[!t]\n\\centering\n\\begin{adjustbox}{' .. maxw .. ',max totalheight=0.8\\textheight}'),
    pandoc.Plain({ img }),
    pandoc.RawBlock('latex', tail),
  }
end

-- Render a multi-column table as a table* float, with an optional caption above.
local function render_table(tbl, captex)
  local ncol = #tbl.colspecs

  local function row(cells, bold)
    local out = {}
    for _, c in ipairs(cells) do
      local s = blocks_to_inline_latex(c.contents)
      if bold and s ~= '' then s = '\\textbf{' .. s .. '}' end
      out[#out + 1] = s
    end
    return table.concat(out, ' & ') .. ' \\\\'
  end

  local l = { '\\begin{table*}[!t]', '\\centering', '\\footnotesize' }
  if captex and captex ~= '' then
    l[#l + 1] = '\\caption{' .. captex .. '}'
  end
  l[#l + 1] = '\\begin{tabularx}{\\textwidth}{' .. string.rep('X', ncol) .. '}'
  l[#l + 1] = '\\toprule'
  for _, r in ipairs(tbl.head.rows) do l[#l + 1] = row(r.cells, true) end
  l[#l + 1] = '\\midrule'
  for _, body in ipairs(tbl.bodies) do
    for _, r in ipairs(body.body) do l[#l + 1] = row(r.cells, false) end
  end
  l[#l + 1] = '\\bottomrule'
  l[#l + 1] = '\\end{tabularx}'
  l[#l + 1] = '\\end{table*}'
  return pandoc.RawBlock('latex', table.concat(l, '\n'))
end

-- Pair each diagram with the caption table that follows it; pair each
-- multi-column table with the single-column caption table that precedes it;
-- convert any remaining stray single-column tables to centered italic text.
function Blocks(blocks)
  local out = pandoc.List()
  local i = 1
  while i <= #blocks do
    local b = blocks[i]
    local img = image_from_block(b)
    if img then
      -- figure: caption comes AFTER (existing behaviour)
      local captex
      local nxt = blocks[i + 1]
      if nxt and nxt.t == 'Table' and #nxt.colspecs <= 1 then
        captex = caption_table_text(nxt)
        i = i + 1
      end
      for _, blk in ipairs(figure_blocks(img, captex)) do out:insert(blk) end
    elseif b.t == 'Table' and #b.colspecs <= 1 then
      -- single-column table: could be a caption BEFORE a data table
      local nxt = blocks[i + 1]
      if nxt and nxt.t == 'Table' and #nxt.colspecs > 1 then
        -- caption + data table pair
        out:insert(render_table(nxt, caption_table_text(b)))
        i = i + 1
      else
        -- stray caption — render as centered italic text
        out:insert(pandoc.RawBlock('latex',
          '\\par\\begin{center}\\footnotesize\\textit{' .. caption_table_text(b) .. '}\\end{center}\\par'))
      end
    elseif b.t == 'Table' and #b.colspecs > 1 then
      -- multi-column table with no preceding caption
      out:insert(render_table(b, nil))
    else
      out:insert(b)
    end
    i = i + 1
  end
  return out
end
