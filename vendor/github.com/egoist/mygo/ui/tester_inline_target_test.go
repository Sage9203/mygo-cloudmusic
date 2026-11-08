package ui

import "testing"

// A paragraph's duplicate label must reach its own inline link, while an overlay still blocks it.
func TestInlineLinkClickTarget(t *testing.T) {
	for _, covered := range []bool{false, true} {
		row, link := 0, 0
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Children(func() {
				ButtonBase(c).Width(300).Height(60).OnClick(func() { row++ }).Children(func() {
					RichText(c).Children(func() { Link(c, "artist", "").OnClick(func() { link++ }) })
				})
				if covered {
					Button(c, "cover").Absolute().Top(0).Left(0).Width(300).Height(60)
				}
			})
		}, 400, 100)
		tt.Frame()
		err := tt.Click("artist")
		if covered {
			if err == nil || row != 0 || link != 0 {
				t.Fatal("covered link accepted input")
			}
		} else if err != nil || link != 1 || row != 0 {
			t.Fatalf("err=%v link=%d row=%d", err, link, row)
		}
	}
}

// An explicitly named interactive parent must not be replaced by its same-named child.
func TestNamedParentKeepsClickTarget(t *testing.T) {
	tt := NewTester(func(c *Context) {
		ButtonBase(c).Label("artist").Width(300).Height(60).Children(func() {
			RichText(c).Children(func() { Link(c, "artist", "") })
		})
	}, 400, 100)
	tt.Frame()
	if err := tt.Click("artist"); err == nil {
		t.Fatal("named parent silently selected its child")
	}
}
