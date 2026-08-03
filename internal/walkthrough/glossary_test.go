package walkthrough

import (
	"strings"
	"testing"
)

func terms() []Term {
	return []Term{
		{Term: "SSVC", Short: "a decision method", Also: []string{"stakeholder-specific vulnerability categorization"}},
		{Term: "NVD", Short: "the US vulnerability database"},
		{Term: "technical impact", Short: "one of the four SSVC points"},
		{Term: "impact", Short: "the generic word"},
	}
}

// runsText joins a split back together; a split that loses or reorders text
// would be invisible in every other assertion here.
func runsText(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.T)
	}
	return b.String()
}

func TestTermIndexMatchesWholeWordsOnly(t *testing.T) {
	ix := NewTermIndex(terms())
	cases := []struct {
		text string
		want []string // keys, in order
	}{
		{"The NVD feed carries an SSVC block.", []string{"nvd", "ssvc"}},
		{"nvd lowercase still resolves", []string{"nvd"}},
		{"An SSVCish thing and NVDX are not terms", nil},
		{"the ssvc_version column", nil}, // underscore is a word character
		{"stakeholder-specific vulnerability categorization spelled out", []string{"ssvc"}},
	}
	for _, c := range cases {
		var got []string
		for _, m := range ix.Find(c.text) {
			got = append(got, m.Key)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: got %v, want %v", c.text, got, c.want)
		}
	}
}

// The longest spelling has to win, or a two-word term is shadowed by one of
// its own words and the reader gets the wrong definition.
func TestTermIndexPrefersTheLongerTerm(t *testing.T) {
	ix := NewTermIndex(terms())
	got := ix.Find("the technical impact point")
	if len(got) != 1 || got[0].Key != "technical impact" {
		t.Fatalf("got %v, want one match on technical impact", got)
	}
}

func TestSplitMarksFirstOccurrenceOnly(t *testing.T) {
	ix := NewTermIndex(terms())
	seen := map[string]bool{}

	runs := ix.Split("The NVD feed is the NVD API.", seen)
	if runsText(runs) != "The NVD feed is the NVD API." {
		t.Fatalf("split lost text: %q", runsText(runs))
	}
	marked := 0
	for _, r := range runs {
		if r.Key != "" {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("marked %d occurrences in one segment, want 1", marked)
	}

	// A later segment of the SAME step carries the memory, so the term stays
	// plain; nothing may be lost by not marking it.
	next := ix.Split("NVD again, plus SSVC.", seen)
	if runsText(next) != "NVD again, plus SSVC." {
		t.Fatalf("split lost text: %q", runsText(next))
	}
	for _, r := range next {
		if r.Key == "nvd" {
			t.Error("nvd marked twice in one step")
		}
	}
	if !seen["ssvc"] {
		t.Error("ssvc was not marked in the second segment")
	}
}

func TestSplitReturnsNilWhenNothingIsMarked(t *testing.T) {
	ix := NewTermIndex(terms())
	if runs := ix.Split("no terms in this sentence", map[string]bool{}); runs != nil {
		t.Errorf("got %v, want nil so the segment travels unchanged", runs)
	}
	seen := map[string]bool{"nvd": true}
	if runs := ix.Split("NVD only, already seen", seen); runs != nil {
		t.Errorf("got %v, want nil when every hit is already spent", runs)
	}
}

func TestEmptyIndexIsInert(t *testing.T) {
	ix := NewTermIndex(nil)
	if got := ix.Find("SSVC and NVD"); got != nil {
		t.Errorf("got %v, want no matches from an empty glossary", got)
	}
}
