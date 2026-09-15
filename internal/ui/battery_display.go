package ui

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
)

// emaSmooth applies exponential moving average smoothing to a time.Duration.
// alpha=0.2 gives a responsive but stable estimate.
type emaSmooth struct {
	value time.Duration
	alpha float64
}

func newEMASmooth(alpha float64) *emaSmooth {
	return &emaSmooth{alpha: alpha}
}

func (e *emaSmooth) reset() {
	e.value = 0
}

func (e *emaSmooth) update(newVal time.Duration) time.Duration {
	if e.value == 0 {
		e.value = newVal
		return newVal
	}
	// EMA: new = alpha * new + (1 - alpha) * old
	n := float64(newVal)
	o := float64(e.value)
	e.value = time.Duration(e.alpha*n + (1-e.alpha)*o)
	return e.value
}

// setTextable is an interface for widgets that can have their text set.
type setTextable interface {
	SetText(string)
}

// richTextLabel is a single-line rich text label with a fixed style.
type richTextLabel struct {
	rt    *widget.RichText
	style widget.RichTextStyle
}

func newRichTextLabel(text string, style widget.RichTextStyle) *richTextLabel {
	return &richTextLabel{
		rt:    widget.NewRichText(&widget.TextSegment{Text: text, Style: style}),
		style: style,
	}
}

// SetText replaces the text, preserving the style.
func (l *richTextLabel) SetText(text string) {
	l.rt.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: text, Style: l.style},
	}
	l.rt.Refresh()
}

// Object returns the underlying widget for layout.
func (l *richTextLabel) Object() fyne.CanvasObject {
	return l.rt
}

// flexRowLayout lays out children in a row with a gap, where one child grows.
type flexRowLayout struct {
	gap     float32
	growIdx int
}

func (l *flexRowLayout) Layout(children []fyne.CanvasObject, size fyne.Size) {
	gap := l.gap
	growIdx := l.growIdx

	// Calculate fixed widths first
	var fixedWidth float32
	for i, child := range children {
		if i != growIdx {
			fixedWidth += child.MinSize().Width
		}
	}

	// Subtract gaps
	fixedWidth += gap * float32(len(children)-1)

	// Available space for growing child
	availWidth := size.Width - fixedWidth
	if availWidth < 0 {
		availWidth = 0
	}

	var x float32
	for i, child := range children {
		minSize := child.MinSize()
		var childWidth float32
		if i == growIdx {
			childWidth = availWidth
			child.Resize(fyne.NewSize(availWidth, minSize.Height))
		} else {
			childWidth = minSize.Width
			child.Resize(minSize)
		}
		// Vertically center each child
		y := (size.Height - minSize.Height) / 2
		child.Move(fyne.NewPos(x, y))
		x += childWidth + gap
	}
}

func (l *flexRowLayout) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	growIdx := l.growIdx
	var maxH float32
	var totalW float32

	for i, child := range children {
		minSize := child.MinSize()
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
		totalW += minSize.Width
		if i != growIdx {
			totalW += gap
		}
	}
	return fyne.NewSize(totalW, maxH)
}

// flexRow creates a container with flex-row layout (children in a row with gap).
// growIdx specifies which child should grow to fill available space.
func flexRow(gap float32, growIdx int, children ...fyne.CanvasObject) *fyne.Container {
	return container.New(&flexRowLayout{
		gap:     gap,
		growIdx: growIdx,
	}, children...)
}

// grid2x2Layout lays out 4 children in a 2x2 grid with equal columns and vertical centering.
type grid2x2Layout struct {
	gap float32
}

func (l *grid2x2Layout) Layout(children []fyne.CanvasObject, size fyne.Size) {
	gap := l.gap

	// Calculate column widths
	var maxW float32
	for _, child := range children {
		minSize := child.MinSize()
		if minSize.Width > maxW {
			maxW = minSize.Width
		}
	}

	// Two columns of equal width
	colW := (size.Width - gap) / 2

	// Position items: 2 columns, 2 rows
	var y float32
	for row := 0; row < 2; row++ {
		var rowMaxH float32
		for col := 0; col < 2; col++ {
			idx := row*2 + col
			if idx >= len(children) {
				continue
			}
			child := children[idx]
			minSize := child.MinSize()
			if minSize.Height > rowMaxH {
				rowMaxH = minSize.Height
			}
		}
		x := float32(0)
		for col := 0; col < 2; col++ {
			idx := row*2 + col
			if idx >= len(children) {
				continue
			}
			child := children[idx]
			minSize := child.MinSize()
			child.Resize(fyne.NewSize(colW, minSize.Height))
			child.Move(fyne.NewPos(x, y))
			x += colW + gap
		}
		y += rowMaxH
	}
}

func (l *grid2x2Layout) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	var maxW, maxH float32
	for _, child := range children {
		minSize := child.MinSize()
		if minSize.Width > maxW {
			maxW = minSize.Width
		}
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}
	return fyne.NewSize(maxW*2+gap, maxH*2)
}

// flexRow creates a container with flex-row layout (children in a row with gap).
// growIdx specifies which child should grow to fill available space.
func flexRow2x2(gap float32, children ...fyne.CanvasObject) *fyne.Container {
	return container.New(&grid2x2Layout{
		gap: gap,
	}, children...)
}

// justifyEvenlyLayout lays out children in a row, evenly spaced across the full width.
type justifyEvenlyLayout struct {
	gap float32
}

func (l *justifyEvenlyLayout) Layout(children []fyne.CanvasObject, size fyne.Size) {
	gap := l.gap
	n := len(children)
	if n == 0 {
		return
	}

	// Calculate total fixed width
	var totalFixedWidth float32
	var maxH float32
	for _, child := range children {
		minSize := child.MinSize()
		totalFixedWidth += minSize.Width
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}

	// Space distribution for justify-evenly
	// space = (width - totalFixedWidth) / (n + 1)
	space := (size.Width - totalFixedWidth) / float32(n+1)

	var x float32
	for i, child := range children {
		minSize := child.MinSize()
		child.Resize(minSize)
		child.Move(fyne.NewPos(x, (maxH-minSize.Height)/2))
		x += minSize.Width + space
		if i < n-1 {
			x += gap
		}
	}
}

func (l *justifyEvenlyLayout) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	var totalW float32
	var maxH float32
	for i, child := range children {
		minSize := child.MinSize()
		totalW += minSize.Width
		if i < len(children)-1 {
			totalW += gap
		}
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}
	return fyne.NewSize(totalW, maxH)
}

// flexRow creates a container with flex-row layout (children in a row with gap).
// growIdx specifies which child should grow to fill available space.
func justifyEvenly(gap float32, children ...fyne.CanvasObject) *fyne.Container {
	return container.New(&justifyEvenlyLayout{
		gap: gap,
	}, children...)
}

// grid3Layout lays out 3 children in a 3-column grid with equal widths.
type grid3Layout struct {
	gap float32
}

func (l *grid3Layout) Layout(children []fyne.CanvasObject, size fyne.Size) {
	gap := l.gap
	n := len(children)
	if n == 0 {
		return
	}

	// Three columns of equal width
	colW := (size.Width - gap*2) / 3

	var maxH float32
	for _, child := range children {
		minSize := child.MinSize()
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}

	for i, child := range children {
		minSize := child.MinSize()
		col := i

		x := float32(col) * (colW + gap)
		y := (maxH - minSize.Height) / 2

		child.Resize(fyne.NewSize(colW, minSize.Height))
		child.Move(fyne.NewPos(x, y))
	}
}

func (l *grid3Layout) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	var totalW, maxH float32
	for i, child := range children {
		minSize := child.MinSize()
		totalW += minSize.Width
		if i < len(children)-1 {
			totalW += gap
		}
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}
	return fyne.NewSize(totalW, maxH)
}

// grid3 creates a 3-column grid with equal widths and gap.
func grid3(gap float32, children ...fyne.CanvasObject) *fyne.Container {
	return container.New(&grid3Layout{gap: gap}, children...)
}

// labelValueLayout lays out label and value side by side with a gap.
type labelValueLayout struct {
	gap float32
}

func (l *labelValueLayout) Layout(children []fyne.CanvasObject, size fyne.Size) {
	if len(children) == 2 {
		label := children[0]
		value := children[1]
		labelMin := label.MinSize()
		valueMin := value.MinSize()
		label.Resize(labelMin)
		value.Resize(valueMin)
		// Label left-aligned, value right-aligned, both vertically centered
		label.Move(fyne.NewPos(0, (size.Height-labelMin.Height)/2))
		value.Move(fyne.NewPos(size.Width-valueMin.Width, (size.Height-valueMin.Height)/2))
	}
}

func (l *labelValueLayout) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	var maxH float32
	var totalW float32
	for i, child := range children {
		minSize := child.MinSize()
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
		totalW += minSize.Width
		if i < len(children)-1 {
			totalW += gap
		}
	}
	return fyne.NewSize(totalW, maxH)
}

// labelValue creates a horizontal label+value container with gap.
func labelValue(gap float32, label, value fyne.CanvasObject) *fyne.Container {
	return container.New(&labelValueLayout{gap: gap}, label, value)
}

// batteryStack lays out header and grid vertically with a gap.
type batteryStack struct {
	gap float32
}

func (l *batteryStack) Layout(children []fyne.CanvasObject, size fyne.Size) {
	gap := l.gap
	if len(children) == 2 {
		header := children[0]
		grid := children[1]
		headerMin := header.MinSize()
		gridMin := grid.MinSize()
		header.Resize(fyne.NewSize(size.Width, headerMin.Height))
		grid.Resize(fyne.NewSize(size.Width, gridMin.Height))
		header.Move(fyne.NewPos(0, 0))
		grid.Move(fyne.NewPos(0, headerMin.Height+gap))
	}
}

func (l *batteryStack) MinSize(children []fyne.CanvasObject) fyne.Size {
	gap := l.gap
	var maxW float32
	var totalH float32
	for i, child := range children {
		minSize := child.MinSize()
		if minSize.Width > maxW {
			maxW = minSize.Width
		}
		totalH += minSize.Height
		if i < len(children)-1 {
			totalH += gap
		}
	}
	return fyne.NewSize(maxW, totalH)
}

// batteryDisplay builds and updates the battery section: a header row with
// a battery icon, title and percentage in large text, followed by a
// two-by-two grid of details in small text where the values are bold.
type batteryDisplay struct {
	icon       *canvas.Text
	percentage *richTextLabel
	sizeValue  *richTextLabel
	timeLabel  *richTextLabel
	timeValue  *richTextLabel
	cycleValue *richTextLabel
	rateLabel  *richTextLabel
	rateValue  *richTextLabel
	ema        *emaSmooth
	lastStatus battery.Status
}

func newBatteryDisplay() (*fyne.Container, *batteryDisplay) {
	header := widget.RichTextStyle{SizeName: theme.SizeNameHeadingText}
	headerBold := widget.RichTextStyle{
		SizeName:  theme.SizeNameHeadingText,
		TextStyle: fyne.TextStyle{Bold: true},
	}
	small := widget.RichTextStyle{SizeName: theme.SizeNameCaptionText}
	smallBold := widget.RichTextStyle{
		SizeName:  theme.SizeNameCaptionText,
		TextStyle: fyne.TextStyle{Bold: true},
	}

	gap4 := float32(4)

	// Header: [ icon ] Battery [percentage%]
	// flex row, items center, Battery grows, gap 4px
	iconSize := theme.Size(theme.SizeNameHeadingText)
	icon := canvas.NewText("󰁹", theme.Color(theme.ColorNameForeground))
	icon.TextStyle = fyne.TextStyle{Monospace: true}
	icon.TextSize = iconSize
	percentage := newRichTextLabel("", headerBold)
	title := newRichTextLabel("Battery", header)
	headerBar := flexRow(gap4, 1, icon, title.Object(), percentage.Object())

	// Grid: labels are regular, values are bold.
	sizeLabel := newRichTextLabel("Battery size:", small)
	sizeValue := newRichTextLabel("", smallBold)
	timeLabel := newRichTextLabel("Time left:", small)
	timeValue := newRichTextLabel("", smallBold)
	cycleLabel := newRichTextLabel("Charge cycles:", small)
	cycleValue := newRichTextLabel("", smallBold)
	rateLabel := newRichTextLabel("Discharging:", small)
	rateValue := newRichTextLabel("", smallBold)

	// 2x2 grid, equal columns, vertically centered, no gap
	grid := flexRow2x2(0,
		labelValue(4, sizeLabel.Object(), sizeValue.Object()),
		labelValue(4, timeLabel.Object(), timeValue.Object()),
		labelValue(4, cycleLabel.Object(), cycleValue.Object()),
		labelValue(4, rateLabel.Object(), rateValue.Object()),
	)

	// Vertical stack with no gap
	content := container.New(&batteryStack{gap: 0}, headerBar, grid)

	return content, &batteryDisplay{
		icon:       icon,
		percentage: percentage,
		sizeValue:  sizeValue,
		timeLabel:  timeLabel,
		timeValue:  timeValue,
		cycleValue: cycleValue,
		rateLabel:  rateLabel,
		rateValue:  rateValue,
		ema:        newEMASmooth(0.2), // 20% weight on new reading
	}
}

func (d *batteryDisplay) update(bat *battery.Battery, err error) {
	if err != nil {
		d.percentage.SetText("Error")
		d.clearDetails()
		return
	}

	if bat == nil {
		d.percentage.SetText("No battery")
		d.clearDetails()
		return
	}

	d.percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
	d.setIcon(bat.Percentage, bat.Status)

	if bat.SizeWh > 0 {
		d.sizeValue.SetText(fmt.Sprintf("%d Wh", int(math.Round(bat.SizeWh))))
	} else {
		d.sizeValue.SetText("")
	}

	if bat.Cycles > 0 {
		d.cycleValue.SetText(strconv.Itoa(bat.Cycles))
	} else {
		d.cycleValue.SetText("")
	}

	// Time to full while charging, time left while discharging.
	if bat.Status == battery.StatusCharging {
		d.timeLabel.SetText("Time to full:")
	} else {
		d.timeLabel.SetText("Time left:")
	}

	// Smooth the time estimate, but never blend estimates from a
	// different status (time-to-full vs time-to-empty).
	if bat.Status != d.lastStatus {
		d.ema.reset()
		d.lastStatus = bat.Status
	}
	if bat.TimeLeft > 0 {
		d.timeValue.SetText(formatDuration(d.ema.update(bat.TimeLeft)))
	} else {
		d.timeValue.SetText("")
	}

	switch bat.Status {
	case battery.StatusCharging:
		d.rateLabel.SetText("Charging:")
	case battery.StatusDischarging:
		d.rateLabel.SetText("Discharging:")
	default:
		d.rateLabel.SetText("Power:")
	}
	if bat.RateW > 0 {
		d.rateValue.SetText(fmt.Sprintf("%.1f W", bat.RateW))
	} else {
		d.rateValue.SetText("")
	}
}

// iconLevels maps battery charge levels to Nerd Font icons, from empty to full.
var iconLevels = []string{
	"󰁺", // 0%
	"󰁻", // ~10%
	"󰁼", // ~20%
	"󰁽", // ~30%
	"󰁾", // ~40%
	"󰁿", // ~50%
	"󰂀", // ~60%
	"󰂁", // ~70%
	"󰂂", // ~80%
	"󰁹", // ~90-100%
}

func (d *batteryDisplay) setIcon(percentage int, status battery.Status) {
	switch status {
	case battery.StatusCharging:
		d.icon.Text = "󰂄"
	case battery.StatusFull:
		d.icon.Text = "󰚥"
	case battery.StatusNotCharging, battery.StatusUnknown:
		d.icon.Text = "󰚥"
	default:
		idx := percentage / 10
		if idx >= len(iconLevels) {
			idx = len(iconLevels) - 1
		}
		if idx < 0 {
			idx = 0
		}
		d.icon.Text = iconLevels[idx]
	}
}

// iconLargeStyle returns a RichTextStyle that's 4x the heading font size.
func iconLargeStyle(baseSize float32) widget.RichTextStyle {
	return widget.RichTextStyle{
		SizeName:  theme.SizeNameHeadingText,
		TextStyle: fyne.TextStyle{Monospace: true},
	}
}

func (d *batteryDisplay) clearDetails() {
	d.sizeValue.SetText("")
	d.timeLabel.SetText("")
	d.timeValue.SetText("")
	d.cycleValue.SetText("")
	d.rateLabel.SetText("")
	d.rateValue.SetText("")
}
