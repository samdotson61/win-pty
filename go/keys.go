package main

// keys.go — split a send payload into literal-text and named-key segments.
// Supports: <Enter> <Esc> <Tab> <BS> <Space> <Up> <Down> <Left> <Right>
// <Home> <End> <PgUp> <PgDn> <Del> <F1>..<F12> <C-x> <S-x> <M-x>.  "<<" is a
// literal "<". Named keys map to tmux send-keys key names.

import (
	"fmt"
	"strings"
)

type seg struct {
	isKey bool
	val   string // literal text, or a tmux key name
}

var keyMap = map[string]string{
	"enter": "Enter", "return": "Enter", "esc": "Escape", "escape": "Escape",
	"tab": "Tab", "bs": "BSpace", "backspace": "BSpace", "space": "Space",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"home": "Home", "end": "End", "pgup": "PageUp", "pgdn": "PageDown",
	"del": "DC", "delete": "DC",
}

func resolveKey(tok string) (string, error) {
	low := strings.ToLower(tok)
	if v, ok := keyMap[low]; ok {
		return v, nil
	}
	// Function keys F1..F12
	if len(low) >= 2 && low[0] == 'f' {
		if _, err := fmt.Sscanf(low, "f%d", new(int)); err == nil {
			return "F" + tok[1:], nil
		}
	}
	// Modifier combos C-x / S-x / M-x (Ctrl/Shift/Alt). tmux uses the same form.
	if len(tok) >= 3 && tok[1] == '-' {
		switch tok[0] {
		case 'C', 'c':
			return "C-" + tok[2:], nil
		case 'S', 's':
			return "S-" + tok[2:], nil
		case 'M', 'm':
			return "M-" + tok[2:], nil
		}
	}
	return "", fmt.Errorf("unknown key token <%s>", tok)
}

func parseKeys(text string) ([]seg, error) {
	var segs []seg
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, seg{isKey: false, val: lit.String()})
			lit.Reset()
		}
	}
	i := 0
	for i < len(text) {
		c := text[i]
		if c == '<' {
			// "<<" -> literal "<"
			if i+1 < len(text) && text[i+1] == '<' {
				lit.WriteByte('<')
				i += 2
				continue
			}
			end := strings.IndexByte(text[i:], '>')
			if end == -1 {
				// no closing '>' — treat as literal
				lit.WriteByte(c)
				i++
				continue
			}
			tok := text[i+1 : i+end]
			key, err := resolveKey(tok)
			if err != nil {
				return nil, err
			}
			flush()
			segs = append(segs, seg{isKey: true, val: key})
			i += end + 1
			continue
		}
		lit.WriteByte(c)
		i++
	}
	flush()
	return segs, nil
}
