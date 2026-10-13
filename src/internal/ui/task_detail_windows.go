//go:build windows

package ui

import "strings"

func (f *form) layoutDetailMetadata(bounds rectT) {
	if _, ok := f.resultSnapshot(); !ok {
		procShowWindow.Call(f.ctrls[5960], swHide)
		procShowWindow.Call(f.ctrls[5961], swHide)
		return
	}
	pad, gap := f.px(18), f.px(12)
	width := (int(bounds.Right-bounds.Left) - 2*pad - 2*gap) / 3
	left := int(bounds.Left) + pad
	top := int(bounds.Top) + f.px(118)
	f.showAt(5960, left+f.px(16), top, width-f.px(28), f.px(20))
	f.showAt(5961, left+2*(width+gap)+f.px(16), top, width-f.px(28), f.px(92))
}

func (f *form) updateDetailMetadata(snap resultSnapshot, record taskRecord) {
	cards := detailCards(snap, record)
	for _, entry := range []struct {
		id   int
		text string
	}{{5960, strings.TrimPrefix(cards[0].Lines[0], "路径：")}, {5961, strings.Join(cards[2].Lines, "\r\n")}} {
		if windowText(f.ctrls[entry.id]) != entry.text {
			setWindowTextRaw(f.ctrls[entry.id], entry.text)
		}
		f.addTooltip(entry.id, entry.text)
	}
}
