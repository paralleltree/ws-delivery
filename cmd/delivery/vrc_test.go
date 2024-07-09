package main

import "testing"

func TestParseInstanceOwner(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "hidden instance",
			input: `wrld_aaaaaaaa-8175-vvvv-cccc-qqqqqqqqqqqq:69931~hidden(usr_23dbxxxx-e8xx-xxxx-yyyy-zzzzzzzzzzzz)~region(jp)`,
			want:  `usr_23dbxxxx-e8xx-xxxx-yyyy-zzzzzzzzzzzz`,
		},
		{
			name:  "group instance",
			input: `wrld_b4494444-9444-4444-bb0d-ddddd38d9ddd:77673~group(grp_xxxxxxx-1111-4444-8888-8aa3cc832b33)~groupAccessType(members)~region(jp)`,
			want:  `grp_xxxxxxx-1111-4444-8888-8aa3cc832b33`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInstanceOwner(tt.input)
			if tt.want != got {
				t.Errorf("unexpected result: want %s, but got %s", tt.want, got)
			}
		})
	}
}
