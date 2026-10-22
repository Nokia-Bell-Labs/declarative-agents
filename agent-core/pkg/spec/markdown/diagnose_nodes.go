// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// parentAt names the context a heading at level opens under: the document
// at level 2, otherwise the node open one level up, or field:<key> when that
// node is a field.
func (d *diagnosis) parentAt(level int) string {
	if level == 2 {
		return "document"
	}
	if level < 2 || level > maxHeadingLevel {
		return fmt.Sprintf("level %d", level)
	}
	parent := d.stack[level-1]
	switch {
	case parent == nil:
		return "none"
	case parent.tag == "field":
		return "field:" + parent.id
	}
	return parent.kind
}

func (d *diagnosis) heading(b Block, level int, heading string) {
	d.closeOpen()
	line := d.fileLine(b)
	d.stray = d.pending == nil
	if d.stray {
		d.add("heading-untagged", line, "a marker on the line above the heading", "\""+heading+"\" with no marker",
			"nodes", "add the marker that says what the heading opens, e.g. <!-- field: <key> -->")
		return
	}
	m, markerLine := *d.pending, d.pendLine
	d.pending = nil
	parent := d.parentAt(level)
	kind, ok := d.rules.nodeFor(m.Tag, level, parent)
	if !ok {
		d.add("heading-misplaced", line, strings.Join(d.rules.allowedHere(parent), " or ")+" under "+parent,
			fmt.Sprintf("%s at level %d", m.Tag, level), "nodes",
			"change the heading depth or move the section under the node that may hold it")
		return
	}
	d.finishFrom(level)
	node := &dnode{kind: kind, tag: m.Tag, id: m.Value, level: level, line: line,
		markerLine: markerLine, fields: map[string]bool{}}
	if m.Tag == tagField {
		node.form = markerForm(m)
	}
	d.counts[kind]++
	d.checkNode(node, m, heading)
	d.stack[level] = node
	d.open = node
}

// finishFrom closes every node at level or deeper: their children are all
// known now, so per-node required fields can be checked.
func (d *diagnosis) finishFrom(level int) {
	for l := maxHeadingLevel; l >= level; l-- {
		if node := d.stack[l]; node != nil {
			d.finishNode(node)
			d.stack[l] = nil
		}
	}
}

func (d *diagnosis) finishNode(node *dnode) {
	if d.exempt() {
		return
	}
	for _, key := range d.rules.Nodes[node.kind].RequiredFields {
		if !node.fields[key] {
			d.add("missing-required-field", node.line, "a "+key+" field under "+node.id, "none",
				"nodes."+node.kind+".required_fields",
				fmt.Sprintf("add <!-- field: %s --> with a level-%d heading under %s", key, node.level+1, node.id))
		}
	}
}

func (d *diagnosis) checkNode(node *dnode, m Marker, heading string) {
	rule := d.rules.Nodes[node.kind]
	path := "nodes." + node.kind
	if pattern, ok := d.rules.idPatterns[node.kind]; ok && !pattern.MatchString(m.Value) {
		d.add("id-malformed", node.line, "an id matching "+rule.IDPattern, m.Value, path+".id_pattern",
			"use the declared id form; an id a release contains is never renumbered (design.yaml D6)")
	}
	if parent := d.stack[node.level-1]; rule.IDExtendsParent && parent != nil &&
		!strings.HasPrefix(m.Value, parent.id+".") {
		d.add("id-outside-parent", node.line, "an id under "+parent.id, m.Value, path+".id_extends_parent",
			"move the node under the parent its id names, or retire it with a pointer")
	}
	d.checkHeading(node, rule, heading)
	if rule.UniqueID {
		key := node.kind + " " + m.Value // ids are unique within their kind
		if first, seen := d.ids[key]; seen {
			d.add("duplicate-id", node.line, "an id used once", fmt.Sprintf("%s, first used on line %d", m.Value, first),
				path+".unique_id", "give the second node a new id; an id is never reused (design.yaml D6)")
		} else {
			d.ids[key] = node.line
		}
	}
	if node.tag == "field" {
		d.checkField(node)
	}
	d.checkAttributes(node, m)
}

func (d *diagnosis) checkHeading(node *dnode, rule nodeRule, heading string) {
	path := "nodes." + node.kind + ".heading"
	switch rule.Heading {
	case headingID:
		if heading != node.id {
			d.add("heading-id-mismatch", node.line, "heading \""+node.id+"\"", "\""+heading+"\"", path,
				"make the heading say exactly the id in its marker")
		}
	case headingIDTitle:
		if heading != node.id && !strings.HasPrefix(heading, node.id+" - ") {
			d.add("heading-id-mismatch", node.line, "heading \""+node.id+" - <title>\"", "\""+heading+"\"", path,
				"start the heading with its id and \" - \"")
		}
	}
}

// checkField enforces where a typed field may appear and that a node holds
// each field once.
func (d *diagnosis) checkField(node *dnode) {
	parent := d.doc
	if node.level > 2 {
		parent = d.stack[node.level-1]
	}
	if parent != nil {
		if parent.fields[node.id] {
			d.add("field-duplicate", node.line, "field "+node.id+" once per node", "a second "+node.id,
				"fields", "merge the two sections")
		}
		parent.fields[node.id] = true
	}
	if field, typed := d.rules.Fields[node.id]; typed && !contains(field.In, node.kind) &&
		!contains(d.rules.Nodes[node.kind].Fields, node.id) {
		d.add("field-disallowed", node.line, node.id+" only as "+strings.Join(field.In, " or "), node.kind,
			"fields."+node.id+".in", "move the section to where the field belongs")
	}
	if parent != nil {
		d.checkFieldOrder(parent, node)
	}
}

// checkFieldOrder holds a field to its slot's fields list: declared, and
// after every field the list puts ahead of it (eng10, declared fields and
// order; GH-2654). A slot without the list is not checked.
func (d *diagnosis) checkFieldOrder(parent, node *dnode) {
	slot := d.rules.Nodes[node.kind].Fields
	if len(slot) == 0 {
		return
	}
	path := "nodes." + node.kind + ".fields"
	index := indexOf(slot, node.id)
	switch {
	case index < 0:
		d.add("field-undeclared", node.line, "one of "+strings.Join(slot, ", "), node.id, path,
			"declare "+node.id+" in "+path+" in its place, or move the content into a declared field")
	case index+1 < parent.orderAt:
		d.add("field-out-of-order", node.line, node.id+" ahead of "+parent.lastField, node.id+" after "+parent.lastField,
			path, "move "+node.id+" ahead of "+parent.lastField+", in the order "+path+" lists")
	default:
		parent.orderAt, parent.lastField = index+1, node.id
	}
}

func indexOf(list []string, value string) int {
	for i, item := range list {
		if item == value {
			return i
		}
	}
	return -1
}

func (d *diagnosis) checkAttributes(node *dnode, m Marker) {
	allowed := d.rules.Nodes[node.kind].Attributes
	for _, a := range m.Attrs {
		if node.tag == tagField && a.Key == attrForm {
			if !contains(typedForms, markerForm(m)) {
				d.add("attribute-shape", node.markerLine, "form as one of "+strings.Join(typedForms, ", "), a.Value,
					"eng10 typed forms", `write form: "scalar", "typed table", "tree", "empty list", or "empty map"`)
			}
			if rule, typed := d.rules.ruleFor(node.kind, node.id); typed && (rule.Code != "" || rule.Shape != "") {
				declared := rule.Shape
				if rule.Code != "" {
					declared = "one ```" + rule.Code + " fence"
				}
				d.add("attribute-shape", node.markerLine, node.id+" as "+declared+", with no form", "form: "+a.Value,
					"fields."+node.id, "drop the form attribute and write "+node.id+" as "+declared)
			}
			continue
		}
		if !contains(allowed, a.Key) {
			d.add("marker-unknown-attribute", node.markerLine, "an attribute "+node.kind+" declares", a.Key,
				"nodes."+node.kind+".attributes", suggest(a.Key, allowed))
			continue
		}
		if shape := d.rules.Attributes[a.Key]; !attributeShapeOK(shape, a.Value) {
			d.add("attribute-shape", node.markerLine, a.Key+" as "+shape, a.Value,
				"attributes."+a.Key, "write the value in its declared shape (JSON for lists and references)")
		}
	}
}

func attributeShapeOK(shape, raw string) bool {
	switch shape {
	case "integer":
		var n int
		return decodeAttr(raw, &n) == nil
	case "list":
		var values []string
		return decodeAttr(raw, &values) == nil
	case "references":
		var refs []Reference
		return decodeAttr(raw, &refs) == nil
	}
	var s string
	return decodeAttr(raw, &s) == nil
}

// closeOpen checks the content the open node collected.
func (d *diagnosis) closeOpen() {
	node := d.open
	d.open = nil
	if node == nil {
		return
	}
	rule := d.rules.Nodes[node.kind]
	path := "nodes." + node.kind + ".content"
	switch {
	case rule.Content == contentNone && len(node.content) > 0:
		d.add("content-untagged", d.fileLine(node.content[0]), "content under a tagged child of "+node.id,
			"untagged content under "+node.id, path,
			fmt.Sprintf("tag it with <!-- field: <key> --> and a level-%d heading, or move it into a child", node.level+1))
	case rule.Content == contentProse && len(node.content) == 0 && !d.exempt():
		d.add("missing-content", node.line, node.kind+" text", "an empty "+node.id, path,
			"write the "+node.kind+" text under its heading")
	case rule.Content == contentProse:
		if b, kind, found := containsStructure(node.content); found {
			d.add("untagged-structure", d.fileLine(b), "prose only in "+node.kind+" text", "an untagged "+kind, path,
				fmt.Sprintf("move the %s under its own <!-- field: <key> --> with a level-%d heading", kind, node.level+1))
		}
	}
	if node.tag == "field" {
		d.checkShape(node)
	}
	if contains(d.rules.ProseContainment.Outside, node.kind) {
		d.proseContainment(node)
	}
}

const shapeEmpty = "empty"

// checkShape enforces a typed field's content shape, list item patterns,
// and table columns. A list or table field may be empty, and a table field
// that declares no columns accepts any header.
func (d *diagnosis) checkShape(node *dnode) {
	field, typed := d.rules.ruleFor(node.kind, node.id)
	if !typed || (field.Shape == "" && field.Code == "") {
		return // passthrough: a rule may declare only links or a heading
	}
	content := d.contentText(node)
	shape := shapeEmpty
	if content != "" {
		shape = classifyContent(content)
	}
	line := node.line
	if len(node.content) > 0 {
		line = d.fileLine(node.content[0])
	}
	if shape == shapeEmpty && (field.Shape == shapeList || field.Shape == shapeTable) {
		return // a list or table with no entries
	}
	if field.Code != "" {
		if !codeFieldReadable(field.Code, content) {
			d.add("field-shape-mismatch", line, node.id+" as one "+field.Code+" fence", shape,
				"fields."+node.id+".code", "write "+node.id+" as one ```"+field.Code+" fenced block")
		}
		return
	}
	if shape != field.Shape && (field.Shape != shapeProse || shape != shapeYAML) {
		d.add("field-shape-mismatch", line, node.id+" as "+field.Shape, shape, "fields."+node.id+".shape",
			"write "+node.id+" as "+field.Shape)
		return
	}
	switch field.Shape {
	case shapeList:
		d.checkListItems(node, content, line)
	case shapeTable:
		header := splitRow(strings.SplitN(content, "\n", 2)[0])
		if len(field.Columns) > 0 && !sameStrings(header, field.Columns) {
			d.add("table-columns-mismatch", line, "columns "+strings.Join(field.Columns, " | "),
				strings.Join(header, " | "), "fields."+node.id+".columns", "use exactly the declared columns, in order")
		}
	}
}

func (d *diagnosis) checkListItems(node *dnode, content string, line int) {
	pattern, ok := d.rules.itemPatterns[node.id]
	if !ok {
		return
	}
	for index, item := range parseList(unlink(content)).Content {
		if !pattern.MatchString(item.Value) {
			d.add("list-item-pattern", line, "each item matching "+d.rules.Fields[node.id].ItemPattern,
				fmt.Sprintf("item %d: %q", index+1, firstLine(item.Value)), "fields."+node.id+".item_pattern",
				"rewrite the item in the declared form")
		}
	}
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func (d *diagnosis) contentText(node *dnode) string {
	if len(node.content) == 0 {
		return ""
	}
	return spanText(d.lines, node.content[0].Start, node.content[len(node.content)-1].End)
}

// proseContainment reports RFC 2119 keywords in paragraphs and lists outside
// any req-tagged block; code blocks are not prose.
// A typed form holds data, as the fenced yaml block it replaced did, so
// containment reads neither (eng10, prose containment; GH-2642).
func (d *diagnosis) proseContainment(node *dnode) {
	if d.rules.keywordRegexp == nil || node.form != "" {
		return
	}
	for _, b := range node.content {
		switch b.Node.(type) {
		case *ast.Paragraph, *ast.List:
		default:
			continue
		}
		for i := b.Start; i <= b.End; i++ {
			if keyword := d.rules.keywordRegexp.FindString(d.lines[i]); keyword != "" {
				d.add("prose-containment", i+d.offset+1, "no RFC 2119 keyword outside a req-tagged block",
					"\""+keyword+"\" in "+node.kind+" "+node.id, "prose_containment",
					"move the obligation into a requirement item, or reword it as description")
			}
		}
	}
}
