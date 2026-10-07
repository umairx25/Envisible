package cli

import "testing"

func TestPsQuote(t *testing.T) {
	cases := map[string]string{
		"simple":  "'simple'",
		"a'b":     "'a''b'",
		"":        "''",
		"a'b'c":   "'a''b''c'",
		"with sp": "'with sp'",
	}
	for in, want := range cases {
		if got := psQuote(in); got != want {
			t.Errorf("psQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseShellArg(t *testing.T) {
	cases := []struct {
		args []string
		want string
		ok   bool
	}{
		{nil, "bash", true},
		{[]string{"bash"}, "bash", true},
		{[]string{"zsh"}, "bash", true}, // zsh shares POSIX syntax
		{[]string{"powershell"}, "powershell", true},
		{[]string{"pwsh"}, "powershell", true},
		{[]string{"ps"}, "powershell", true},
		{[]string{"fish"}, "", false},
	}
	for _, c := range cases {
		got, err := parseShellArg(c.args)
		if c.ok && err != nil {
			t.Errorf("parseShellArg(%v) unexpected error: %v", c.args, err)
		}
		if !c.ok && err == nil {
			t.Errorf("parseShellArg(%v) expected error, got %q", c.args, got)
		}
		if c.ok && got != c.want {
			t.Errorf("parseShellArg(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}
