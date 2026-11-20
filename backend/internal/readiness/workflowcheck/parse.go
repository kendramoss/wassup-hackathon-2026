package workflowcheck

import (
	"fmt"
	"strings"
)

type line struct {
	indent int
	text   string
	no     int
}

// Parse reads the restricted YAML subset: block maps, block lists (including
// lists of maps), plain/quoted scalars, and "|" / "|-" block scalars. Maps are
// map[string]any, lists []any, scalars string. A trailing " #" comment is
// removed from unquoted values only.
func Parse(src string) (any, error) {
	if strings.Contains(src, "\t") {
		return nil, fmt.Errorf("tab characters are not supported")
	}
	raw := strings.Split(src, "\n")
	var ls []line
	blocks := map[string]string{}
	for i := 0; i < len(raw); i++ {
		t := strings.TrimRight(raw[i], " \r")
		trim := strings.TrimLeft(t, " ")
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		ind := len(t) - len(trim)
		if trim == "---" || trim == "..." {
			return nil, fmt.Errorf("line %d: multi-document files are not supported", i+1)
		}
		key, val, isKV := splitKV(trim)
		if isKV && (val == "|" || val == "|-") {
			var body []string
			j := i + 1
			base := -1
			for ; j < len(raw); j++ {
				r := strings.TrimRight(raw[j], " \r")
				if strings.TrimSpace(r) == "" {
					body = append(body, "")
					continue
				}
				ri := len(r) - len(strings.TrimLeft(r, " "))
				if ri <= ind {
					break
				}
				if base < 0 {
					base = ri
				}
				if ri < base {
					return nil, fmt.Errorf("line %d: block scalar indentation", j+1)
				}
				body = append(body, r[base:])
			}
			for len(body) > 0 && body[len(body)-1] == "" {
				body = body[:len(body)-1]
			}
			id := fmt.Sprintf("\x00block%d", len(blocks))
			blocks[id] = strings.Join(body, "\n") + "\n"
			ls = append(ls, line{ind, key + ": " + id, i + 1})
			i = j - 1
			continue
		}
		ls = append(ls, line{ind, trim, i + 1})
	}
	if len(ls) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	p := &parser{ls: ls, blocks: blocks}
	v, err := p.block(ls[0].indent)
	if err != nil {
		return nil, err
	}
	if p.i != len(ls) {
		return nil, fmt.Errorf("line %d: unexpected content", ls[p.i].no)
	}
	return v, nil
}

type parser struct {
	ls     []line
	i      int
	blocks map[string]string
}

func splitKV(s string) (key, val string, ok bool) {
	if strings.HasPrefix(s, "'") || strings.HasPrefix(s, "\"") {
		return "", "", false
	}
	if strings.HasSuffix(s, ":") {
		return strings.TrimSpace(strings.TrimSuffix(s, ":")), "", true
	}
	k, v, found := strings.Cut(s, ": ")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

func (p *parser) block(indent int) (any, error) {
	l := p.ls[p.i]
	if l.text == "-" || strings.HasPrefix(l.text, "- ") {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func (p *parser) list(indent int) (any, error) {
	out := []any{}
	for p.i < len(p.ls) {
		l := p.ls[p.i]
		if l.indent != indent || !(l.text == "-" || strings.HasPrefix(l.text, "- ")) {
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		if rest == "" {
			return nil, fmt.Errorf("line %d: empty list item", l.no)
		}
		if _, _, isKV := splitKV(rest); isKV {
			// "- key: v" opens a map whose keys sit two columns right of the dash.
			p.ls[p.i] = line{indent + 2, rest, l.no}
			m, err := p.mapping(indent + 2)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
			continue
		}
		s, err := p.scalar(rest, l.no)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		p.i++
	}
	return out, nil
}

func (p *parser) mapping(indent int) (any, error) {
	out := map[string]any{}
	for p.i < len(p.ls) {
		l := p.ls[p.i]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: unexpected indentation", l.no)
		}
		if l.text == "-" || strings.HasPrefix(l.text, "- ") {
			break
		}
		key, val, ok := splitKV(l.text)
		if !ok || key == "" {
			return nil, fmt.Errorf("line %d: expected key: value, got %q", l.no, l.text)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", l.no, key)
		}
		p.i++
		if val != "" {
			s, err := p.scalar(val, l.no)
			if err != nil {
				return nil, err
			}
			out[key] = s
			continue
		}
		if p.i < len(p.ls) {
			n := p.ls[p.i]
			isDash := n.text == "-" || strings.HasPrefix(n.text, "- ")
			if n.indent > indent || (n.indent == indent && isDash) {
				v, err := p.block(n.indent)
				if err != nil {
					return nil, err
				}
				out[key] = v
				continue
			}
		}
		out[key] = ""
	}
	return out, nil
}

func (p *parser) scalar(v string, no int) (string, error) {
	if b, ok := p.blocks[v]; ok {
		return b, nil
	}
	switch v[0] {
	case '\'', '"':
		q := v[0]
		if len(v) < 2 || v[len(v)-1] != q {
			return "", fmt.Errorf("line %d: unsupported quoted scalar %q", no, v)
		}
		return v[1 : len(v)-1], nil
	case '[', '{', '&', '*', '!', '>', '|', '%', '@', '`':
		return "", fmt.Errorf("line %d: unsupported YAML construct %q", no, v)
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v, nil
}
