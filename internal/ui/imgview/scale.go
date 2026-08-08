package imgview

// pxPerCol is the assumed terminal column width in pixels, used as a default
// when the real cell size isn't known. Conservative across common terminal
// fonts and DPI settings. Cell height is assumed to be 2×pxPerCol (a 2:1
// height:width cell aspect ratio).
const pxPerCol = 10

// fitCols returns the number of terminal columns that best fits an image of
// imgWidth pixels without upscaling, capped at maxCols, given a terminal
// column width of cellW pixels. The result is always at least 1.
func fitCols(imgWidth, maxCols, cellW int) int {
	natural := (imgWidth + cellW - 1) / cellW
	if natural < 1 {
		natural = 1
	}
	if maxCols > 0 && natural > maxCols {
		return maxCols
	}
	return natural
}

// fitRows estimates the number of terminal rows the image will occupy when
// displayed at cols columns, given a terminal cell size of cellW x cellH
// pixels. The result is always at least 1.
func fitRows(imgHeight, imgWidth, cols, cellW, cellH int) int {
	if imgWidth <= 0 || cols <= 0 {
		return 1
	}
	// rows = imgHeight / cellH, where displayWidth in px = cols * cellW
	// and actual displayHeight = imgHeight * (cols*cellW) / imgWidth
	rows := imgHeight * cols * cellW / (imgWidth * cellH)
	if rows < 1 {
		return 1
	}
	return rows
}

// fitBox computes the terminal cols/rows to display an image of imgWidth x
// imgHeight pixels within a maxCols x maxRows box, preserving aspect ratio
// and never upscaling, given a terminal cell size of cellW x cellH pixels.
// If fitting to maxCols would make rows exceed maxRows, cols is recomputed
// from the row constraint instead so both bounds hold.
func fitBox(imgWidth, imgHeight, maxCols, maxRows, cellW, cellH int) (cols, rows int) {
	cols = fitCols(imgWidth, maxCols, cellW)
	rows = fitRows(imgHeight, imgWidth, cols, cellW, cellH)
	if maxRows > 0 && rows > maxRows && imgHeight > 0 {
		rows = maxRows
		cols = cellH * rows * imgWidth / (cellW * imgHeight)
		if cols < 1 {
			cols = 1
		}
		if maxCols > 0 && cols > maxCols {
			cols = maxCols
		}
	}
	return cols, rows
}
