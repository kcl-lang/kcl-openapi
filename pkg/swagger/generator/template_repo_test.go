package generator

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestToKCLValue(t *testing.T) {
	cases := []struct {
		name   string
		value  interface{}
		expect string
	}{
		{
			name:   "nil",
			value:  nil,
			expect: "None",
		},
		{
			name:   "bool-true",
			value:  true,
			expect: "True",
		},
		{
			name:   "bool-false",
			value:  false,
			expect: "False",
		},
		{
			name:   "string",
			value:  "hello",
			expect: "\"hello\"",
		},
		{
			name: "map-string-int",
			value: yaml.MapSlice{
				{
					Key:   "01",
					Value: 123,
				},
				{
					Key:   "02",
					Value: 456,
				},
			},
			expect: "{\"01\": 123, \"02\": 456}",
		},
		{
			name: "map-string-bool",
			value: yaml.MapSlice{
				{
					Key:   "01",
					Value: true,
				},
				{
					Key:   "02",
					Value: false,
				},
			},
			expect: "{\"01\": True, \"02\": False}",
		},
		{
			name: "slice-map",
			value: []yaml.MapSlice{
				{
					{
						Key:   "01",
						Value: 123,
					},
					{
						Key:   "02",
						Value: 456,
					},
				},
				{
					{
						Key:   "03",
						Value: 123,
					},
					{
						Key:   "04",
						Value: 456,
					},
				},
			},
			expect: "[{\"01\": 123, \"02\": 456}, {\"03\": 123, \"04\": 456}]",
		},
	}
	opts := LanguageOpts{}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			got := opts.ToKclValue(testcase.value)
			if got != testcase.expect {
				t.Fatalf("unexpected output, expect:\n%s\ngot:\n%s\n", testcase.expect, got)
			}
		})
	}
}

func TestPadDocument(t *testing.T) {
	cases := []struct {
		doc                  string
		indented             string
		displayedInDocstring string
	}{
		{
			doc:      "one line doc",
			indented: "        one line doc",
			displayedInDocstring: `
schema ABC:
    """
    schema doc

    Attributes
    ----------
    attrName : type, default is defaultValue, optional/required
        one line doc
"""`,
		},
		{
			doc:      "multi line doc:\n\n- line1\n\n-line2\n\nline3",
			indented: "        multi line doc:\n\n        - line1\n\n        -line2\n\n        line3",
			displayedInDocstring: `
schema ABC:
    """
    schema doc

    Attributes
    ----------
    attrName : type, default is defaultValue, optional/required
        multi line doc:

        - line1

        -line2

        line3
"""`,
		},
		{
			doc:      "multi line doc:\nline1\nline2\nline3",
			indented: "        multi line doc:\n        line1\n        line2\n        line3",
			displayedInDocstring: `
schema ABC:
    """
    schema doc

    Attributes
    ----------
    attrName : type, default is defaultValue, optional/required
        multi line doc:
        line1
        line2
        line3
"""`,
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.doc, func(t *testing.T) {
			got := padDocument(testcase.doc, "        ")
			displayed := fmt.Sprintf("%s%s%s", `
schema ABC:
    """
    schema doc

    Attributes
    ----------
    attrName : type, default is defaultValue, optional/required
`, got, `
"""`)
			if got != testcase.indented || displayed != testcase.displayedInDocstring {
				t.Fatalf("unexpected output, expect:\n%s\ngot:\n%s\n\nexpected display:\n%s\ngot display:\n%s\n", testcase.indented, got, testcase.displayedInDocstring, displayed)
			}
		})
	}
}
