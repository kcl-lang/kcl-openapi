package generator

import (
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

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			got := toKCLValue(testcase.value)
			if got != testcase.expect {
				t.Fatalf("unexpected output, expect:\n%s\ngot:%s\n", testcase.expect, got)
			}
		})
	}
}
