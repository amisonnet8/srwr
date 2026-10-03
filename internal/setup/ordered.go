package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// object is a JSON object that keeps the order of its keys, so that rewriting a person's settings file does not shuffle it.
type object struct {
	keys []string
	vals map[string]any
}

func newObject() *object { return &object{vals: map[string]any{}} }

func (o *object) get(k string) (any, bool) { v, ok := o.vals[k]; return v, ok }

func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) remove(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// parseJSON reads one JSON value. Numbers stay as json.Number, so they are written back as they were.
func parseJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the JSON value")
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string, json.Number, bool or nil
	}
	if d == '[' {
		arr := []any{}
		for dec.More() {
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token() // ]
		return arr, err
	}
	o := newObject()
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, _ := kt.(string)
		v, err := readValue(dec)
		if err != nil {
			return nil, err
		}
		o.set(k, v)
	}
	_, err = dec.Token() // }
	return o, err
}

// lineOf is the 1-based line of the error in data, or 0 if it cannot be told.
func lineOf(data []byte, err error) int {
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		off := min(int(syn.Offset), len(data))
		return bytes.Count(data[:off], []byte{'\n'}) + 1
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return bytes.Count(data, []byte{'\n'}) + 1
	}
	return 0
}

// marshalJSON writes v with two-space indentation and a final newline.
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeValue(&b, v, 0); err != nil {
		return nil, err
	}
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func writeValue(b *bytes.Buffer, v any, depth int) error {
	pad := strings.Repeat("  ", depth+1)
	end := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case *object:
		if len(x.keys) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteString("{\n")
		for i, k := range x.keys {
			b.WriteString(pad)
			if err := writeScalar(b, k); err != nil {
				return err
			}
			b.WriteString(": ")
			if err := writeValue(b, x.vals[k], depth+1); err != nil {
				return err
			}
			if i < len(x.keys)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(end + "}")
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return nil
		}
		b.WriteString("[\n")
		for i, e := range x {
			b.WriteString(pad)
			if err := writeValue(b, e, depth+1); err != nil {
				return err
			}
			if i < len(x)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(end + "]")
	default:
		return writeScalar(b, v)
	}
	return nil
}

func writeScalar(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case json.Number:
		b.WriteString(x.String())
		return nil
	case string, bool, nil:
		var s bytes.Buffer
		enc := json.NewEncoder(&s)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(x); err != nil {
			return err
		}
		b.Write(bytes.TrimRight(s.Bytes(), "\n"))
		return nil
	}
	return fmt.Errorf("cannot write %T", v)
}
