package imgview

const pxPerCol = 10

// fitCols returns the number of terminal columns that best fits an image of
// imgWidth pixels without upscaling, capped at maxCols. Assumes ~10 pixels per
// terminal column, which is conservative across common terminal fonts and DPI
// settings. The result is always at least 1.
func fitCols(imgWidth, maxCols int) int {
	natural := (imgWidth + pxPerCol - 1) / pxPerCol
	if natural < 1 {
		natural = 1
	}
	if maxCols > 0 && natural > maxCols {
		return maxCols
	}
	return natural
}

// fitRows estimates the number of terminal rows the image will occupy when
// displayed at cols columns. Assumes terminal cells are 2:1 height:width in
// pixels (i.e. cell height ≈ 2×pxPerCol). The result is always at least 1.
func fitRows(imgHeight, imgWidth, cols int) int {
	if imgWidth <= 0 || cols <= 0 {
		return 1
	}
	// rows = imgHeight / cellHeightPx, where cellHeightPx = 2*pxPerCol
	// and displayWidth in px = cols * pxPerCol
	// actual displayHeight = imgHeight * (cols*pxPerCol) / imgWidth
	rows := imgHeight * cols * pxPerCol / (imgWidth * 2 * pxPerCol)
	if rows < 1 {
		return 1
	}
	return rows
}

// fitBox computes the terminal cols/rows to display an image of imgWidth x
// imgHeight pixels within a maxCols x maxRows box, preserving aspect ratio
// and never upscaling. If fitting to maxCols would make rows exceed maxRows,
// cols is recomputed from the row constraint instead so both bounds hold.
func fitBox(imgWidth, imgHeight, maxCols, maxRows int) (cols, rows int) {
	cols = fitCols(imgWidth, maxCols)
	rows = fitRows(imgHeight, imgWidth, cols)
	if maxRows > 0 && rows > maxRows && imgHeight > 0 {
		rows = maxRows
		cols = 2 * rows * imgWidth / imgHeight
		if cols < 1 {
			cols = 1
		}
		if maxCols > 0 && cols > maxCols {
			cols = maxCols
		}
	}
	return cols, rows
}
