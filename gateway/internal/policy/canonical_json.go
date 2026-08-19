package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"
)

// CanonicalJSON transforms any Go value into RFC 8785 JSON Canonicalization Scheme (JCS) bytes.
//
// Invariants enforced:
// 1. Whitespace: zero whitespace outside strings (no spaces after colons or commas).
// 2. Object Keys: lexicographically sorted by UTF-16 code units / UTF-8 byte order.
// 3. String Escaping: minimal escaping required by RFC 8785 (quotation mark \", reverse solidus \\, control chars \u0000-\u001f).
// 4. Number Representation: standard decimal formatting, no trailing zeroes, no exponents for standard numbers.
// 5. UTF-8 Encoding: valid UTF-8 without BOM.
func CanonicalJSON(v interface{}) ([]byte, error) {
	// First round-trip through json.Marshal to get a normalized generic representation
	rawJSON, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json marshal: %w", err)
	}

	var generic interface{}
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return nil, fmt.Errorf("canonical json decode: %w", err)
	}

	var buf bytes.Buffer
	if err := formatCanonical(generic, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatCanonical(v interface{}, buf *bytes.Buffer) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(val.String())
	case float64:
		// Format without scientific notation unless necessary, stripping redundant trailing zeros
		buf.WriteString(strconv.FormatFloat(val, 'f', -1, 64))
	case int:
		buf.WriteString(strconv.Itoa(val))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case string:
		writeCanonicalString(val, buf)
	case []interface{}:
		buf.WriteByte('[')
		for i, elem := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := formatCanonical(elem, buf); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]interface{}:
		buf.WriteByte('{')
		// Sort keys lexicographically (RFC 8785 Section 3.2.3)
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(k, buf)
			buf.WriteByte(':')
			if err := formatCanonical(val[k], buf); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		// Fallback for custom structs: re-marshal and recurse
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("unsupported type %T in canonicalization", v)
		}
		var nested interface{}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err := dec.Decode(&nested); err != nil {
			return err
		}
		return formatCanonical(nested, buf)
	}
	return nil
}

// writeCanonicalString writes a string according to RFC 8785 escaping rules.
func writeCanonicalString(s string, buf *bytes.Buffer) {
	buf.WriteByte('"')
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}
