package lyrics

import "testing"

// Japanese originals choose Japanese glyphs while Chinese translations do not change detection.
func TestLyricLanguage(t *testing.T) {
	for _, tc := range []struct{ text, translation, want string }{{"君の声", "你的声音", "ja"}, {"中文歌词", "かな", ""}, {"hello", "", ""}, {"ー・", "", ""}} {
		if got := Lang([]Line{{Text: tc.text, Translation: tc.translation}}); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.text, got, tc.want)
		}
	}
}
