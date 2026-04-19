package emailclean

import "testing"

func TestExtract(t *testing.T) {
	tests := []struct {
		raw        string
		cleaned    string
		normalized string
	}{
		{"%%oon1@gmail.com%%", "oon1@gmail.com", "oon1@gmail.com"},
		{"%$<isfandiary@gmail.com>%%", "isfandiary@gmail.com", "isfandiary@gmail.com"},
		{" %%<Ageviakharnadara@icloud.com>%% ", "Ageviakharnadara@icloud.com", "ageviakharnadara@icloud.com"},
		{"13352543_oongabut@gmail.com", "oongabut@gmail.com", "oongabut@gmail.com"},
		{"13352543_oongabut2@gmail.com... [38]", "oongabut2@gmail.com", "oongabut2@gmail.com"},
	}
	for _, tt := range tests {
		got := Extract(tt.raw)
		if got.Cleaned != tt.cleaned || got.Normalized != tt.normalized {
			t.Fatalf("Extract(%q) = cleaned=%q normalized=%q; want cleaned=%q normalized=%q",
				tt.raw, got.Cleaned, got.Normalized, tt.cleaned, tt.normalized)
		}
	}
}
