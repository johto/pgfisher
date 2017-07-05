// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csv

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

var readTests = []struct {
	Name               string
	Input              string
	Output             [][]string
	UseFieldsPerRecord bool // false (default) means FieldsPerRecord is -1
	ByteOffsets        []int64

	// These fields are copied into the Reader
	Comma            rune
	Comment          rune
	FieldsPerRecord  int
	LazyQuotes       bool
	TrailingComma    bool
	TrimLeadingSpace bool

	Error  string
	Line   int // Expected error line if != 0
	Column int // Expected error column if line != 0
}{
	{
		Name:        "Simple",
		Input:       "a,b,c\n",
		Output:      [][]string{{"a", "b", "c"}},
		ByteOffsets: []int64{6},
	},
	{
		Name:        "CRLF",
		Input:       "a,b\r\nc,d\r\n",
		Output:      [][]string{{"a", "b"}, {"c", "d"}},
		ByteOffsets: []int64{5, 10},
	},
	{
		Name:        "BareCR",
		Input:       "a,b\rc,d\r\n",
		Output:      [][]string{{"a", "b\rc", "d"}},
		ByteOffsets: []int64{9},
	},
	{
		Name:               "RFC4180test",
		UseFieldsPerRecord: true,
		Input: `#field1,field2,field3
"aaa","bb
b","ccc"
"a,a","b""bb","ccc"
zzz,yyy,xxx
`,
		Output: [][]string{
			{"#field1", "field2", "field3"},
			{"aaa", "bb\nb", "ccc"},
			{"a,a", `b"bb`, "ccc"},
			{"zzz", "yyy", "xxx"},
		},
		ByteOffsets: []int64{22, 22 + 19, 22 + 19 + 20, 22 + 19 + 20 + 12},
	},
	{
		Name:        "NoEOLTest",
		Input:       "a,b,c",
		Output:      [][]string{{"a", "b", "c"}},
		ByteOffsets: []int64{5},
	},
	{
		Name:        "Semicolon",
		Comma:       ';',
		Input:       "a;b;c\n",
		Output:      [][]string{{"a", "b", "c"}},
		ByteOffsets: []int64{6},
	},
	{
		Name: "MultiLine",
		Input: `"two
line","one line","three
line
field"`,
		Output:      [][]string{{"two\nline", "one line", "three\nline\nfield"}},
		ByteOffsets: []int64{40},
	},
	{
		Name:  "BlankLine",
		Input: "a,b,c\n\nd,e,f\n\n",
		Output: [][]string{
			{"a", "b", "c"},
			{"d", "e", "f"},
		},
		ByteOffsets: []int64{6, 13},
	},
	{
		Name:               "BlankLineFieldCount",
		Input:              "a,b,c\n\nd,e,f\n\n",
		UseFieldsPerRecord: true,
		Output: [][]string{
			{"a", "b", "c"},
			{"d", "e", "f"},
		},
		ByteOffsets: []int64{6, 13},
	},
	{
		Name:             "TrimSpace",
		Input:            " a,  b,   c\n",
		TrimLeadingSpace: true,
		Output:           [][]string{{"a", "b", "c"}},
		ByteOffsets:      []int64{12},
	},
	{
		Name:        "LeadingSpace",
		Input:       " a,  b,   c\n",
		Output:      [][]string{{" a", "  b", "   c"}},
		ByteOffsets: []int64{12},
	},
	{
		Name:        "Comment",
		Comment:     '#',
		Input:       "#1,2,3\na,b,c\n#comment",
		Output:      [][]string{{"a", "b", "c"}},
		ByteOffsets: []int64{13},
	},
	{
		Name:        "NoComment",
		Input:       "#1,2,3\na,b,c",
		Output:      [][]string{{"#1", "2", "3"}, {"a", "b", "c"}},
		ByteOffsets: []int64{7, 12},
	},
	{
		Name:        "LazyQuotes",
		LazyQuotes:  true,
		Input:       `a "word","1"2",a","b`,
		Output:      [][]string{{`a "word"`, `1"2`, `a"`, `b`}},
		ByteOffsets: []int64{20},
	},
	{
		Name:        "BareQuotes",
		LazyQuotes:  true,
		Input:       `a "word","1"2",a"`,
		Output:      [][]string{{`a "word"`, `1"2`, `a"`}},
		ByteOffsets: []int64{17},
	},
	{
		Name:        "BareDoubleQuotes",
		LazyQuotes:  true,
		Input:       `a""b,c`,
		Output:      [][]string{{`a""b`, `c`}},
		ByteOffsets: []int64{6},
	},
	{
		Name:  "BadDoubleQuotes",
		Input: `a""b,c`,
		Error: `bare " in non-quoted-field`, Line: 1, Column: 1,
		ByteOffsets: []int64{6},
	},
	{
		Name:             "TrimQuote",
		Input:            ` "a"," b",c`,
		TrimLeadingSpace: true,
		Output:           [][]string{{"a", " b", "c"}},
		ByteOffsets:      []int64{11},
	},
	{
		Name:  "BadBareQuote",
		Input: `a "word","b"`,
		Error: `bare " in non-quoted-field`, Line: 1, Column: 2,
		ByteOffsets: []int64{12},
	},
	{
		Name:  "BadTrailingQuote",
		Input: `"a word",b"`,
		Error: `bare " in non-quoted-field`, Line: 1, Column: 10,
		ByteOffsets: []int64{11},
	},
	{
		Name:  "ExtraneousQuote",
		Input: `"a "word","b"`,
		Error: `extraneous " in field`, Line: 1, Column: 3,
		ByteOffsets: []int64{13},
	},
	{
		Name:               "BadFieldCount",
		UseFieldsPerRecord: true,
		Input:              "a,b,c\nd,e",
		Error:              "wrong number of fields", Line: 2,
	},
	{
		Name:               "BadFieldCount1",
		UseFieldsPerRecord: true,
		FieldsPerRecord:    2,
		Input:              `a,b,c`,
		Error:              "wrong number of fields", Line: 1,
	},
	{
		Name:        "FieldCount",
		Input:       "a,b,c\nd,e",
		Output:      [][]string{{"a", "b", "c"}, {"d", "e"}},
		ByteOffsets: []int64{6, 9},
	},
	{
		Name:        "TrailingCommaEOF",
		Input:       "a,b,c,",
		Output:      [][]string{{"a", "b", "c", ""}},
		ByteOffsets: []int64{6},
	},
	{
		Name:        "TrailingCommaEOL",
		Input:       "a,b,c,\n",
		Output:      [][]string{{"a", "b", "c", ""}},
		ByteOffsets: []int64{7},
	},
	{
		Name:             "TrailingCommaSpaceEOF",
		TrimLeadingSpace: true,
		Input:            "a,b,c, ",
		Output:           [][]string{{"a", "b", "c", ""}},
		ByteOffsets:      []int64{7},
	},
	{
		Name:             "TrailingCommaSpaceEOL",
		TrimLeadingSpace: true,
		Input:            "a,b,c, \n",
		Output:           [][]string{{"a", "b", "c", ""}},
		ByteOffsets:      []int64{8},
	},
	{
		Name:             "TrailingCommaLine3",
		TrimLeadingSpace: true,
		Input:            "a,b,c\nd,e,f\ng,hi,",
		Output:           [][]string{{"a", "b", "c"}, {"d", "e", "f"}, {"g", "hi", ""}},
		ByteOffsets:      []int64{6, 12, 17},
	},
	{
		Name:        "NotTrailingComma3",
		Input:       "a,b,c, \n",
		Output:      [][]string{{"a", "b", "c", " "}},
		ByteOffsets: []int64{8},
	},
	{
		Name:          "CommaFieldTest",
		TrailingComma: true,
		Input: `x,y,z,w
x,y,z,
x,y,,
x,,,
,,,
"x","y","z","w"
"x","y","z",""
"x","y","",""
"x","","",""
"","","",""
`,
		Output: [][]string{
			{"x", "y", "z", "w"},
			{"x", "y", "z", ""},
			{"x", "y", "", ""},
			{"x", "", "", ""},
			{"", "", "", ""},
			{"x", "y", "z", "w"},
			{"x", "y", "z", ""},
			{"x", "y", "", ""},
			{"x", "", "", ""},
			{"", "", "", ""},
		},
		ByteOffsets: []int64{8, 15, 21, 26, 30, 46, 61, 75, 88, 100},
	},
	{
		Name:             "TrailingCommaIneffective1",
		TrailingComma:    true,
		TrimLeadingSpace: true,
		Input:            "a,b,\nc,d,e",
		Output: [][]string{
			{"a", "b", ""},
			{"c", "d", "e"},
		},
		ByteOffsets: []int64{5, 10},
	},
	{
		Name:             "TrailingCommaIneffective2",
		TrailingComma:    false,
		TrimLeadingSpace: true,
		Input:            "a,b,\nc,d,e",
		Output: [][]string{
			{"a", "b", ""},
			{"c", "d", "e"},
		},
		ByteOffsets: []int64{5, 10},
	},
	{
		Name:        "UTF-8",
		Input:       `a,äö,"äöq",☃落`,
		Output:      [][]string{{"a", "äö", "äöq", "☃落"}},
		ByteOffsets: []int64{21},
	},
}

func TestRead(t *testing.T) {
	readAll := func(r *Reader) (records [][]string, byteOffsets []int64, err error) {
		for {
			record, err := r.Read()
			if err == io.EOF {
				return records, byteOffsets, nil
			}
			if err != nil {
				return nil, nil, err
			}
			records = append(records, record)
			byteOffsets = append(byteOffsets, r.ByteOffset)
		}
	}

	for _, tt := range readTests {
		r := NewReader(strings.NewReader(tt.Input))
		r.Comment = tt.Comment
		if tt.UseFieldsPerRecord {
			r.FieldsPerRecord = tt.FieldsPerRecord
		} else {
			r.FieldsPerRecord = -1
		}
		r.LazyQuotes = tt.LazyQuotes
		r.TrailingComma = tt.TrailingComma
		r.TrimLeadingSpace = tt.TrimLeadingSpace
		if tt.Comma != 0 {
			r.Comma = tt.Comma
		}
		out, byteOffsets, err := readAll(r)
		perr, _ := err.(*ParseError)
		if tt.Error != "" {
			if err == nil || !strings.Contains(err.Error(), tt.Error) {
				t.Errorf("%s: error %v, want error %q", tt.Name, err, tt.Error)
			} else if tt.Line != 0 && (tt.Line != perr.Line || tt.Column != perr.Column) {
				t.Errorf("%s: error at %d:%d expected %d:%d", tt.Name, perr.Line, perr.Column, tt.Line, tt.Column)
			}
		} else if err != nil {
			t.Errorf("%s: unexpected error %v", tt.Name, err)
		} else if !reflect.DeepEqual(out, tt.Output) {
			t.Errorf("%s: out=%q want %q", tt.Name, out, tt.Output)
		} else if !reflect.DeepEqual(byteOffsets, tt.ByteOffsets) {
			t.Errorf("%s: ByteOffset=%v want %v", tt.Name, byteOffsets, tt.ByteOffsets)
		}
	}
}

// nTimes is an io.Reader which yields the string s n times.
type nTimes struct {
	s   string
	n   int
	off int
}

func (r *nTimes) Read(p []byte) (n int, err error) {
	for {
		if r.n <= 0 || r.s == "" {
			return n, io.EOF
		}
		n0 := copy(p, r.s[r.off:])
		p = p[n0:]
		n += n0
		r.off += n0
		if r.off == len(r.s) {
			r.off = 0
			r.n--
		}
		if len(p) == 0 {
			return
		}
	}
}

// benchmarkRead measures reading the provided CSV rows data.
// initReader, if non-nil, modifies the Reader before it's used.
func benchmarkRead(b *testing.B, initReader func(*Reader), rows string) {
	b.ReportAllocs()
	r := NewReader(&nTimes{s: rows, n: b.N})
	if initReader != nil {
		initReader(r)
	}
	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

const benchmarkCSVData = `x,y,z,w
x,y,z,
x,y,,
x,,,
,,,
"x","y","z","w"
"x","y","z",""
"x","y","",""
"x","","",""
"","","",""
`

func BenchmarkRead(b *testing.B) {
	benchmarkRead(b, nil, benchmarkCSVData)
}

func BenchmarkReadWithFieldsPerRecord(b *testing.B) {
	benchmarkRead(b, func(r *Reader) { r.FieldsPerRecord = 4 }, benchmarkCSVData)
}

func BenchmarkReadWithoutFieldsPerRecord(b *testing.B) {
	benchmarkRead(b, func(r *Reader) { r.FieldsPerRecord = -1 }, benchmarkCSVData)
}

func BenchmarkReadLargeFields(b *testing.B) {
	benchmarkRead(b, nil, strings.Repeat(`xxxxxxxxxxxxxxxx,yyyyyyyyyyyyyyyy,zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz,wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww,vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
xxxxxxxxxxxxxxxxxxxxxxxx,yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy,zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz,wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww,vvvv
,,zzzz,wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww,vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx,yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy,zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz,wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww,vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
`, 3))
}
