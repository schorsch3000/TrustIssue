package main

import (
	"strings"
	"testing"
)

const secret = "EEUUWLPBWBYNVFDNAV4EKSUBWCQX3YEG"

func TestPlanAddNameFromTheURI(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"the example of the user", "otpauth://totp/GitHub-schorsch3000?secret=" + secret + "&issuer=GitHub-schorsch3000&digits=6&period=30", "GitHub-schorsch3000"},
		{"issuer and account in the label", "otpauth://totp/GitHub:alice?secret=" + secret + "&issuer=GitHub", "GitHub:alice"},
		{"an e-mail address as account", "otpauth://totp/GitHub:alice%40example.com?secret=" + secret, "GitHub:alice-example.com"},
		{"the issuer is added to a bare account", "otpauth://totp/alice?secret=" + secret + "&issuer=Example", "Example:alice"},
		{"only an issuer", "otpauth://totp/?secret=" + secret + "&issuer=Example", "Example"},
		{"spaces around the colon", "otpauth://totp/My%20Bank%3A%20bob?secret=" + secret, "My Bank:bob"},
		{"a non-ASCII letter", "otpauth://totp/Caf%C3%A9?secret=" + secret, "Caf-"},
	}
	for _, c := range cases {
		plan, err := planAdd(nil, c.input)
		if err != nil || plan.name != c.want || !plan.derivedFromURI {
			t.Errorf("%s: got %q %v, want %q", c.name, plan.name, err, c.want)
		}
	}
	plan, _ := planAdd(nil, "otpauth://totp/GitHub:alice%40example.com?secret="+secret)
	if !plan.nameWasAdjusted {
		t.Error("a replaced character must be reported")
	}
	plan, _ = planAdd(nil, "otpauth://totp/GitHub-x?secret="+secret+"&issuer=GitHub-x")
	if plan.nameWasAdjusted {
		t.Error("nothing was replaced")
	}
}

func TestPlanAddExplicitNameWins(t *testing.T) {
	plan, err := planAdd([]string{"Work VPN", "--digits", "8"}, "otpauth://totp/GitHub:alice?secret="+secret+"&digits=6")
	if err != nil || plan.name != "Work VPN" || plan.derivedFromURI || plan.digits != 6 {
		t.Errorf("%+v %v (the URI's digits win over --digits, the given name over the label)", plan, err)
	}
	plan, err = planAdd([]string{"Plain", "--digits", "8", "--period", "60"}, "gezd gnbv")
	if err != nil || plan.name != "Plain" || plan.digits != 8 || plan.period != 60 {
		t.Errorf("a plain secret: %+v %v", plan, err)
	}
}

func TestPlanAddErrors(t *testing.T) {
	cases := map[string]struct {
		args  []string
		input string
		want  string
	}{
		"a plain secret needs a name": {nil, secret, "needs a NAME"},
		"a long label":                {nil, "otpauth://totp/AVeryLongIssuerName:averylongaccountname@example.com?secret=" + secret, "at most 27"},
		"no label at all":             {nil, "otpauth://totp/?secret=" + secret, "no label"},
		"an unsupported URI":          {nil, "otpauth://hotp/x?secret=" + secret, "only totp"},
		"bad digits":                  {[]string{"X", "--digits", "9"}, secret, "6 to 8 digits"},
		"a bad given name":            {[]string{"Bad_Name"}, secret, "contains"},
	}
	for what, c := range cases {
		if _, err := planAdd(c.args, c.input); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v (want %q)", what, err, c.want)
		}
	}
}
