package recipe

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/schema"
)

const schemaBase = "https://raw.githubusercontent.com/RashKash103/get-cooked/main/schema/"

var (
	schemaOnce sync.Once
	schemas    map[string]*jsonschema.Schema
	schemaErr  error
)

func loadSchemas() {
	c := jsonschema.NewCompiler()
	for _, name := range []string{"recipe", "ingredients", "config", "i18n"} {
		b, err := schema.FS.ReadFile(name + ".schema.json")
		if err != nil {
			schemaErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			schemaErr = err
			return
		}
		if err := c.AddResource(schemaBase+name+".schema.json", doc); err != nil {
			schemaErr = err
			return
		}
	}
	schemas = map[string]*jsonschema.Schema{}
	for _, name := range []string{"recipe", "ingredients", "config", "i18n"} {
		s, err := c.Compile(schemaBase + name + ".schema.json")
		if err != nil {
			schemaErr = err
			return
		}
		schemas[name] = s
	}
}

// ValidateSchema validates a YAML document against a named schema
// ("recipe", "ingredients" or "config") and maps errors to line numbers.
func ValidateSchema(name, file string, root *yaml.Node) Diags {
	schemaOnce.Do(loadSchemas)
	if schemaErr != nil {
		return Diags{{File: file, Msg: "internal schema error: " + schemaErr.Error()}}
	}
	doc := root
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil
		}
		doc = doc.Content[0]
	}
	inst, err := toJSONValue(doc)
	if err != nil {
		return Diags{{File: file, Line: doc.Line, Msg: err.Error()}}
	}
	err = schemas[name].Validate(inst)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return Diags{{File: file, Msg: err.Error()}}
	}
	var leaves []*jsonschema.ValidationError
	var collect func(e *jsonschema.ValidationError)
	collect = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			leaves = append(leaves, e)
		}
		for _, c := range e.Causes {
			collect(c)
		}
	}
	collect(ve)
	// Keep the most specific errors: deepest location, preferring
	// non-type errors (oneOf branches produce noisy "want string" leaves).
	depth := 0
	for _, l := range leaves {
		depth = max(depth, len(l.InstanceLocation))
	}
	var best []*jsonschema.ValidationError
	hasNonType := false
	for _, l := range leaves {
		if len(l.InstanceLocation) == depth {
			best = append(best, l)
			if _, isType := l.ErrorKind.(*kind.Type); !isType {
				hasNonType = true
			}
		}
	}
	p := message.NewPrinter(language.English)
	seen := map[string]bool{}
	var diags Diags
	for _, l := range best {
		if _, isType := l.ErrorKind.(*kind.Type); isType && hasNonType {
			continue
		}
		msg := l.ErrorKind.LocalizedString(p)
		if path := prettyPath(l.InstanceLocation); path != "" {
			msg = path + ": " + msg
		}
		for field, hint := range removedFields {
			if strings.Contains(msg, "'"+field+"'") && strings.Contains(msg, "not allowed") {
				msg += " (" + hint + ")"
			}
		}
		if seen[msg] {
			continue
		}
		seen[msg] = true
		n := nodeAt(doc, l.InstanceLocation)
		if n.Kind == yaml.MappingNode && n.Style&yaml.FlowStyle != 0 {
			for i := 0; i < len(n.Content); i += 2 {
				if strings.Contains(n.Content[i].Value, " ") {
					msg += " (hint: a comma inside { … } separates fields, so quote text that contains commas)"
					break
				}
			}
		}
		diags = append(diags, Diag{File: file, Line: n.Line, Col: n.Column, Msg: msg})
	}
	sort.SliceStable(diags, func(i, j int) bool { return diags[i].Line < diags[j].Line })
	return diags
}

// removedFields explains fields that were replaced, pointing at the
// migration that upgrades them.
var removedFields = map[string]string{
	"grams_per_cup": "replaced by density in 0.3.0; run `get-cooked migrate`",
}

func prettyPath(tokens []string) string {
	var sb strings.Builder
	for _, t := range tokens {
		if _, err := strconv.Atoi(t); err == nil {
			fmt.Fprintf(&sb, "[%s]", t)
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(t)
	}
	return sb.String()
}

func nodeAt(n *yaml.Node, tokens []string) *yaml.Node {
	cur := n
	for _, t := range tokens {
		for cur.Kind == yaml.AliasNode && cur.Alias != nil {
			cur = cur.Alias
		}
		switch cur.Kind {
		case yaml.MappingNode:
			found := false
			for i := 0; i+1 < len(cur.Content); i += 2 {
				if cur.Content[i].Value == t {
					cur = cur.Content[i+1]
					found = true
					break
				}
			}
			if !found {
				return cur
			}
		case yaml.SequenceNode:
			i, err := strconv.Atoi(t)
			if err != nil || i >= len(cur.Content) {
				return cur
			}
			cur = cur.Content[i]
		default:
			return cur
		}
	}
	return cur
}

// toJSONValue converts a YAML node into the generic JSON value model.
func toJSONValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return toJSONValue(n.Alias)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Value == "<<" {
				return nil, fmt.Errorf("line %d: YAML merge keys are not supported", k.Line)
			}
			v, err := toJSONValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := toJSONValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	default:
		switch n.Tag {
		case "!!null":
			return nil, nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err != nil {
				return nil, err
			}
			return b, nil
		case "!!int", "!!float":
			var f float64
			if err := n.Decode(&f); err != nil {
				return nil, err
			}
			return f, nil
		default:
			return n.Value, nil
		}
	}
}
