// Package kv reads and edits Valve's KeyValues text files (gamemodes_server.txt of CS2): only the part changed is
// rewritten, the rest of the file (other blocks, comments, layout) stays as it was.
package kv

import "strings"

// Node is a key with a value, or with a block of keys.
type Node struct {
	Key   string
	Value string
	Block bool
	Kids  []*Node
	start int // where its key starts in the text
	open  int // its block's "{" (Block)
	close int // its block's "}" (Block)
}

// Child finds a key of a block, ignoring case (as the game does).
func (n *Node) Child(key string) *Node {
	for _, k := range n.Kids {
		if strings.EqualFold(k.Key, key) {
			return k
		}
	}
	return nil
}

type token struct {
	s     string
	quote bool
	pos   int
}

func lex(text string) []token {
	var out []token
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == 0xEF || c == 0xBB || c == 0xBF: // BOM too
			i++
		case c == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '{' || c == '}':
			out = append(out, token{s: string(c), pos: i})
			i++
		case c == '"':
			start := i
			i++
			var b strings.Builder
			for i < len(text) && text[i] != '"' {
				if text[i] == '\\' && i+1 < len(text) && (text[i+1] == '"' || text[i+1] == '\\') {
					i++
				}
				b.WriteByte(text[i])
				i++
			}
			i++ // the closing quote
			out = append(out, token{s: b.String(), quote: true, pos: start})
		default:
			start := i
			for i < len(text) && !strings.ContainsRune(" \t\r\n{}\"", rune(text[i])) {
				i++
			}
			out = append(out, token{s: text[start:i], pos: start})
		}
	}
	return out
}

// conditional: [$WIN32] and the like after a value
func conditional(t token) bool { return !t.quote && strings.HasPrefix(t.s, "[") }

// Parse reads the text into a root holding its top keys. A broken file gives what could be read.
func Parse(text string) *Node {
	toks := lex(text)
	root := &Node{Block: true, open: -1, close: len(text)}
	k := 0
	var parse func(parent *Node)
	parse = func(parent *Node) {
		for k < len(toks) {
			t := toks[k]
			if !t.quote && t.s == "}" {
				if parent != root {
					parent.close = t.pos
					k++
					return
				}
				k++ // a stray one
				continue
			}
			if !t.quote && t.s == "{" || conditional(t) {
				k++
				continue
			}
			n := &Node{Key: t.s, start: t.pos}
			k++
			if k < len(toks) && !toks[k].quote && toks[k].s == "{" {
				n.Block, n.open, n.close = true, toks[k].pos, len(text)
				k++
				parse(n)
			} else if k < len(toks) && !(toks[k].s == "}" && !toks[k].quote) {
				n.Value = toks[k].s
				k++
			}
			for k < len(toks) && conditional(toks[k]) {
				k++
			}
			parent.Kids = append(parent.Kids, n)
		}
	}
	parse(root)
	return root
}

// mapGroups is the "mapgroups" block of the file (under its top key)
func mapGroups(root *Node) *Node {
	for _, top := range root.Kids {
		if top.Block {
			if mg := top.Child("mapgroups"); mg != nil && mg.Block {
				return mg
			}
		}
	}
	return nil
}

// MapGroups gives the map groups of a gamemodes_server.txt, each with its maps in order.
func MapGroups(text string) (names []string, maps map[string][]string) {
	maps = map[string][]string{}
	mg := mapGroups(Parse(text))
	if mg == nil {
		return nil, maps
	}
	for _, g := range mg.Kids {
		if !g.Block {
			continue
		}
		var list []string
		if m := g.Child("maps"); m != nil && m.Block {
			for _, x := range m.Kids {
				list = append(list, x.Key)
			}
		}
		if _, dup := maps[g.Key]; !dup {
			names = append(names, g.Key)
		}
		maps[g.Key] = list
	}
	return names, maps
}

// lineStart is where the line holding pos starts; indentOf, the spaces and tabs it begins with.
func lineStart(text string, pos int) int { return strings.LastIndexByte(text[:pos], '\n') + 1 }

func indentOf(text string, pos int) string {
	s := lineStart(text, pos)
	e := s
	for e < len(text) && (text[e] == ' ' || text[e] == '\t') {
		e++
	}
	return text[s:e]
}

// unit: one level of indentation, as the file has it
func unit(ind string) string {
	if ind == "" || strings.Contains(ind, "\t") {
		return "\t"
	}
	if len(ind)%4 == 0 {
		return "    "
	}
	return "  "
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `'`) + `"` }

// mapLines writes the maps of a group, values lined up
func mapLines(maps []string, ind, nl string) string {
	w := 0
	for _, m := range maps {
		w = max(w, len(m))
	}
	var b strings.Builder
	for _, m := range maps {
		b.WriteString(ind + quote(m) + strings.Repeat(" ", w-len(m)+2) + `""` + nl)
	}
	return b.String()
}

// insertBefore puts lines before a "}": at the start of its line when it is alone on it
func insertBefore(text string, close int, lines, nl string) string {
	ls := lineStart(text, close)
	if strings.TrimSpace(text[ls:close]) == "" {
		return text[:ls] + lines + text[ls:]
	}
	return text[:close] + nl + lines + text[close:]
}

// SetMapGroup gives the text with a map group's maps replaced (the group, its "mapgroups" block, even the whole
// file, made when missing). The rest of the text stays as it was.
func SetMapGroup(text, group string, maps []string) string {
	nl := "\n"
	if strings.Contains(text, "\r\n") || strings.TrimSpace(text) == "" {
		nl = "\r\n"
	}
	step := "" // one level of indentation for what is added: as where it goes
	block := func(ind, key string, body func(in string) string) string {
		u := step
		if u == "" {
			u = unit(ind)
		}
		return ind + quote(key) + nl + ind + "{" + nl + body(ind+u) + ind + "}" + nl
	}
	groupText := func(ind string) string {
		return block(ind, group, func(in string) string {
			return in + quote("name") + "  " + quote(group) + nl + block(in, "maps", func(in2 string) string { return mapLines(maps, in2, nl) })
		})
	}
	root := Parse(text)
	mg := mapGroups(root)
	if mg == nil {
		// no "mapgroups": in the top block, or a new file
		var top *Node
		for _, t := range root.Kids {
			if t.Block {
				top = t
				break
			}
		}
		if top == nil {
			if strings.TrimSpace(text) != "" && !strings.HasSuffix(text, "\n") {
				text += nl
			}
			return text + block("", "GameModes.txt", func(in string) string {
				return block(in, "mapgroups", func(in2 string) string { return groupText(in2) })
			})
		}
		ind := indentOf(text, top.close) + unit(indentOf(text, top.start))
		if len(top.Kids) > 0 {
			ind = indentOf(text, top.Kids[0].start)
		}
		step = unit(ind)
		return insertBefore(text, top.close, block(ind, "mapgroups", func(in string) string { return groupText(in) }), nl)
	}
	var g *Node
	for _, x := range mg.Kids {
		if x.Block && strings.EqualFold(x.Key, group) {
			g = x // the last one with the name: the one the game keeps
		}
	}
	if g == nil {
		ind := indentOf(text, mg.start) + unit(indentOf(text, mg.start))
		if len(mg.Kids) > 0 {
			ind = indentOf(text, mg.Kids[0].start)
		}
		step = unit(ind)
		return insertBefore(text, mg.close, groupText(ind), nl)
	}
	m := g.Child("maps")
	if m == nil || !m.Block {
		ind := indentOf(text, g.start) + unit(indentOf(text, g.start))
		if len(g.Kids) > 0 {
			ind = indentOf(text, g.Kids[0].start)
		}
		step = unit(ind)
		return insertBefore(text, g.close, block(ind, "maps", func(in string) string { return mapLines(maps, in, nl) }), nl)
	}
	// the maps block: what is between its braces, replaced
	ind := indentOf(text, m.start) + unit(indentOf(text, m.start))
	if len(m.Kids) > 0 {
		ind = indentOf(text, m.Kids[0].start)
	}
	from := strings.IndexByte(text[m.open:], '\n') + m.open + 1 // the line after "{"
	to := lineStart(text, m.close)                              // the line of "}"
	if from <= m.open || from > to || strings.TrimSpace(text[m.open+1:from]) != "" || strings.TrimSpace(text[to:m.close]) != "" {
		// "{" and "}" not each alone on its line: the maps between them, on lines of their own
		return text[:m.open+1] + nl + mapLines(maps, ind, nl) + indentOf(text, m.start) + text[m.close:]
	}
	return text[:from] + mapLines(maps, ind, nl) + text[to:]
}
