package slug

import "testing"

func TestFromName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Turkish letters must fold to ASCII, not be stripped.
		{"turkish company", "Çağrı Şirketi", "cagri-sirketi"},
		{"dotted capital I", "İstanbul", "istanbul"},
		{"dotless lowercase i", "Işık", "isik"},
		{"all special lower", "çğıöşü", "cgiosu"},
		{"all special upper", "ÇĞİÖŞÜ", "cgiosu"},
		{"mixed with ascii", "Gaziantep Böğürtlen", "gaziantep-bogurtlen"},
		{"umlauts", "Gözlük Üretimi", "gozluk-uretimi"},

		// General behaviour preserved.
		{"simple", "Hello World", "hello-world"},
		{"collapses separators", "a  --  b__c", "a-b-c"},
		{"trims separators", "  -Hello-  ", "hello"},
		{"keeps digits", "Route 66", "route-66"},
		{"empty", "", "item"},
		{"only separators", "   ---   ", "item"},
		{"only non-ascii that folds away to nothing", "₺€", "item"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromName(tc.in); got != tc.want {
				t.Errorf("FromName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFromNameTruncatesTo80(t *testing.T) {
	in := ""
	for i := 0; i < 100; i++ {
		in += "a"
	}
	got := FromName(in)
	if len(got) > 80 {
		t.Errorf("slug length = %d, want <= 80", len(got))
	}
}

func TestFromNameCapitalDottedI(t *testing.T) {
	// "İ" (U+0130) folds to a plain ASCII "i" regardless of ToLower's
	// case-mapping, so no stray separator leaks into the slug.
	got := FromName("İzmir")
	if got != "izmir" {
		t.Errorf("FromName(%q) = %q, want %q", "İzmir", got, "izmir")
	}
}
