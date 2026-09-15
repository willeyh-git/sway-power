package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// Layout assembles all UI parts with explicit control over gaps.
type Layout struct {
	batContent *fyne.Container
	status     fyne.CanvasObject
	separator  fyne.CanvasObject
	powerLabel fyne.CanvasObject
	powerBar   *fyne.Container
	lidLabel   fyne.CanvasObject
	lidBar     *fyne.Container
}

func NewLayout(batContent *fyne.Container, status fyne.CanvasObject, separator fyne.CanvasObject, powerLabel fyne.CanvasObject, powerBar *fyne.Container, lidLabel fyne.CanvasObject, lidBar *fyne.Container) *Layout {
	return &Layout{
		batContent: batContent,
		status:     status,
		separator:  separator,
		powerLabel: powerLabel,
		powerBar:   powerBar,
		lidLabel:   lidLabel,
		lidBar:     lidBar,
	}
}

// Container returns the root container.
func (l *Layout) Container() *fyne.Container {
	return container.NewVBox(
		l.batContent,
		l.status,
		l.separator,
		l.powerLabel,
		l.powerBar,
		l.lidLabel,
		l.lidBar,
	)
}

// flexRow lays out children in a row, with one child growing.
// growIdx is the index of the child that should grow.
type flexRow struct {
	growIdx int
}

func (l *flexRow) Layout(children []fyne.CanvasObject, size fyne.Size) {
	var fixedWidth float32
	for i, child := range children {
		if i != l.growIdx {
			fixedWidth += child.MinSize().Width
		}
	}

	availWidth := size.Width - fixedWidth
	if availWidth < 0 {
		availWidth = 0
	}

	var x float32
	for i, child := range children {
		minSize := child.MinSize()
		var childWidth float32
		if i == l.growIdx {
			childWidth = availWidth
			child.Resize(fyne.NewSize(availWidth, minSize.Height))
		} else {
			childWidth = minSize.Width
			child.Resize(minSize)
		}
		y := (size.Height - minSize.Height) / 2
		child.Move(fyne.NewPos(x, y))
		x += childWidth
	}
}

func (l *flexRow) MinSize(children []fyne.CanvasObject) fyne.Size {
	var maxH, totalW float32
	for _, child := range children {
		minSize := child.MinSize()
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
		totalW += minSize.Width
	}
	return fyne.NewSize(totalW, maxH)
}

// labelValue lays out label (left) and value (right).
type labelValue struct{}

func (l *labelValue) Layout(children []fyne.CanvasObject, size fyne.Size) {
	if len(children) == 2 {
		label := children[0]
		value := children[1]
		labelMin := label.MinSize()
		valueMin := value.MinSize()
		label.Resize(labelMin)
		value.Resize(valueMin)
		label.Move(fyne.NewPos(0, (size.Height-labelMin.Height)/2))
		value.Move(fyne.NewPos(size.Width-valueMin.Width, (size.Height-valueMin.Height)/2))
	}
}

func (l *labelValue) MinSize(children []fyne.CanvasObject) fyne.Size {
	var maxH float32
	var totalW float32
	for _, child := range children {
		minSize := child.MinSize()
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
		totalW += minSize.Width
	}
	return fyne.NewSize(totalW, maxH)
}

// btnBar lays out 3 buttons in a 1x3 grid with equal column widths.
type btnBar struct{}

func (l *btnBar) Layout(children []fyne.CanvasObject, size fyne.Size) {
	n := len(children)
	if n == 0 {
		return
	}

	colW := size.Width / float32(n)

	var maxH float32
	for _, child := range children {
		if child.MinSize().Height > maxH {
			maxH = child.MinSize().Height
		}
	}

	var x float32
	for _, child := range children {
		minSize := child.MinSize()
		child.Resize(fyne.NewSize(colW, minSize.Height))
		child.Move(fyne.NewPos(x, (maxH-minSize.Height)/2))
		x += colW
	}
}

func (l *btnBar) MinSize(children []fyne.CanvasObject) fyne.Size {
	var totalW, maxH float32
	for _, child := range children {
		minSize := child.MinSize()
		totalW += minSize.Width
		if minSize.Height > maxH {
			maxH = minSize.Height
		}
	}
	return fyne.NewSize(totalW, maxH)
}

// grid2x2 lays out 4 children in a 2x2 grid.
type grid2x2 struct{}

func (l *grid2x2) Layout(children []fyne.CanvasObject, size fyne.Size) {
	colW := size.Width / 2
	rowH := size.Height / 2

	for i, child := range children {
		minSize := child.MinSize()
		col := i % 2
		row := i / 2
		x := float32(col) * colW
		y := float32(row) * rowH
		child.Resize(fyne.NewSize(colW, minSize.Height))
		child.Move(fyne.NewPos(x, y))
	}
}

func (l *grid2x2) MinSize(children []fyne.CanvasObject) fyne.Size {
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
	return fyne.NewSize(maxW * 2, maxH * 2)
}
