package generator

import "testing"

func TestEscapedModelName(t *testing.T) {
	cases := []struct {
		value  string
		expect string
	}{
		{
			value:  "a.b.c",
			expect: "a.b.c",
		},
		{
			value:  ".c",
			expect: ".c",
		},
		{
			value:  "c.",
			expect: "c.",
		},
		{
			value:  "a.b.c-d",
			expect: "a.b.c_d",
		},
		{
			value:  "a-a.b.c",
			expect: "a-a.b.c",
		},
		{
			value:  "a-a.b.c-d",
			expect: "a-a.b.c_d",
		},
	}
	opts := LanguageOpts{}

	for _, testcase := range cases {
		t.Run(testcase.value, func(t *testing.T) {
			got := opts.MangleModelName(testcase.value)
			if got != testcase.expect {
				t.Fatalf("unexpected output, expect:\n%s\ngot:%s\n", testcase.expect, got)
			}
		})
	}
}
