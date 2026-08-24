package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrNonFiniteNumber    = errors.New("canonical JSON: non-finite number (NaN or Infinity) is prohibited")
	ErrDuplicateObjectKey = errors.New("canonical JSON: duplicate object key detected in JSON input")
	ErrInvalidUTF8        = errors.New("canonical JSON: invalid UTF-8 string")
	ErrLoneSurrogate      = errors.New("canonical JSON: lone surrogate code point is prohibited")
)

// UTF16KeyLess compares two strings according to RFC 8785 Section 3.2.3:
// "The sorting MUST be based on the UTF-16 code units of the keys."
func UTF16KeyLess(a, b string) bool {
	uA := utf16.Encode([]rune(a))
	uB := utf16.Encode([]rune(b))
	minLen := len(uA)
	if len(uB) < minLen {
		minLen = len(uB)
	}
	for i := 0; i < minLen; i++ {
		if uA[i] != uB[i] {
			return uA[i] < uB[i]
		}
	}
	return len(uA) < len(uB)
}

// ValidateJSONNoDuplicates scans raw JSON bytes to verify valid UTF-8 and ensure no object contains duplicate keys.
func ValidateJSONNoDuplicates(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrInvalidUTF8
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var keyStack []map[string]struct{}

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("scan JSON tokens: %w", err)
		}

		delim, ok := tok.(json.Delim)
		if ok {
			switch delim {
			case '{':
				keyStack = append(keyStack, make(map[string]struct{}))
			case '}':
				if len(keyStack) > 0 {
					keyStack = keyStack[:len(keyStack)-1]
				}
			case '[', ']':
				// Array delimiters, no key tracking needed
			}
			continue
		}

		// If we are directly inside an object and expecting a key
		if len(keyStack) > 0 && dec.More() {
			keyStr, isStr := tok.(string)
			if isStr {
				currentObj := keyStack[len(keyStack)-1]
				if _, exists := currentObj[keyStr]; exists {
					return fmt.Errorf("%w: key %q", ErrDuplicateObjectKey, keyStr)
				}
				currentObj[keyStr] = struct{}{}
			}
		}
	}
	return nil
}

// CanonicalJSON transforms any Go value or raw JSON into RFC 8785 JSON Canonicalization Scheme (JCS) bytes.
//
// Invariants enforced (RFC 8785 & I-JSON RFC 7493):
// 1. Whitespace: zero whitespace outside strings (no spaces after colons or commas).
// 2. Object Keys: sorted strictly by UTF-16 code units (RFC 8785 Section 3.2.3).
// 3. String Escaping: minimal escaping required by RFC 8785 (\", \\, control chars \u0000-\u001f).
// 4. Number Representation: standard decimal formatting, no trailing zeroes, rejects NaN/Infinity.
// 5. UTF-8 Encoding: valid UTF-8 without BOM, rejects lone surrogates, preserves Unicode without normalization.
// 6. Duplicate Keys: rejects raw JSON input with duplicate keys.
func CanonicalJSON(v interface{}) ([]byte, error) {
	// If input is already raw JSON bytes, validate duplicate keys first
	var generic interface{}

	switch input := v.(type) {
	case []byte:
		if err := ValidateJSONNoDuplicates(input); err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(input))
		decoder.UseNumber()
		if err := decoder.Decode(&generic); err != nil {
			return nil, fmt.Errorf("canonical json decode: %w", err)
		}
	case string:
		raw := []byte(input)
		if err := ValidateJSONNoDuplicates(raw); err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&generic); err != nil {
			return nil, fmt.Errorf("canonical json decode: %w", err)
		}
	default:
		// First round-trip through json.Marshal to get normalized generic representation
		rawJSON, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("canonical json marshal: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(rawJSON))
		decoder.UseNumber()
		if err := decoder.Decode(&generic); err != nil {
			return nil, fmt.Errorf("canonical json decode: %w", err)
		}
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
		str := val.String()
		if str == "-0" || str == "-0.0" {
			buf.WriteString("0")
			return nil
		}
		if !strings.ContainsAny(str, ".eE") {
			if i, err := strconv.ParseInt(str, 10, 64); err == nil {
				buf.WriteString(strconv.FormatInt(i, 10))
				return nil
			}
			if u, err := strconv.ParseUint(str, 10, 64); err == nil {
				buf.WriteString(strconv.FormatUint(u, 10))
				return nil
			}
		}
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return fmt.Errorf("invalid number %q: %w", str, err)
		}
		if err := formatCanonicalNumber(f, buf); err != nil {
			return err
		}
	case float64:
		if err := formatCanonicalNumber(val, buf); err != nil {
			return err
		}
	case float32:
		if err := formatCanonicalNumber(float64(val), buf); err != nil {
			return err
		}
	case int:
		buf.WriteString(strconv.Itoa(val))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(val, 10))
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case string:
		if err := writeCanonicalString(val, buf); err != nil {
			return err
		}
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
		// Sort keys strictly by UTF-16 code units (RFC 8785 Section 3.2.3)
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return UTF16KeyLess(keys[i], keys[j])
		})

		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(k, buf); err != nil {
				return err
			}
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
			return fmt.Errorf("unsupported type %T in canonicalization: %w", v, err)
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

// writeCanonicalString writes a string according to RFC 8785 escaping rules and validates UTF-8 / surrogate constraints.
func writeCanonicalString(s string, buf *bytes.Buffer) error {
	if !utf8.ValidString(s) {
		return ErrInvalidUTF8
	}

	buf.WriteByte('"')
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			return ErrInvalidUTF8
		}
		// Check for lone surrogate codepoints (U+D800 to U+DFFF)
		if r >= 0xD800 && r <= 0xDFFF {
			return ErrLoneSurrogate
		}

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
	return nil
}

// formatCanonicalNumber formats an IEEE-754 binary64 number per RFC 8785 Section 3.2.2.3.
func formatCanonicalNumber(f float64, buf *bytes.Buffer) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ErrNonFiniteNumber
	}
	// Negative zero (-0.0) must be serialized as "0"
	if f == 0 {
		buf.WriteString("0")
		return nil
	}

	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		// Non-exponential range
		buf.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
	} else {
		// Scientific notation with lowercase 'e' and normalized exponent
		s := strconv.FormatFloat(f, 'e', -1, 64)
		buf.WriteString(normalizeExponent(s))
	}
	return nil
}

func normalizeExponent(s string) string {
	idx := strings.IndexByte(s, 'e')
	if idx == -1 {
		return s
	}
	prefix := s[:idx]
	sign := s[idx+1]
	expStr := s[idx+2:]
	expStr = strings.TrimLeft(expStr, "0")
	if expStr == "" {
		expStr = "0"
	}
	return prefix + "e" + string(sign) + expStr
}
