package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// lidCloseMenu creates a context menu for configuring lid close behavior.
func lidCloseMenu(win fyne.Window, status *widget.Label, current string) *fyne.Menu {
	items := []*fyne.MenuItem{
		{
			Label: "Lock",
			Action: func() {
				status.SetText("Lid close: lock")
				saveLidClose(win, "lock")
			},
		},
		{
			Label: "Sleep",
			Action: func() {
				status.SetText("Lid close: sleep")
				saveLidClose(win, "sleep")
			},
		},
		{
			Label: "Nothing",
			Action: func() {
				status.SetText("Lid close: nothing")
				saveLidClose(win, "nothing")
			},
		},
	}

	// Mark the current selection.
	for _, item := range items {
		if item.Label == "Lock" && current == "lock" {
			item.Checked = true
		} else if item.Label == "Sleep" && current == "sleep" {
			item.Checked = true
		} else if item.Label == "Nothing" && current == "nothing" {
			item.Checked = true
		}
	}

	return fyne.NewMenu("", items...)
}


