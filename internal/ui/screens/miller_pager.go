package screens

import "strings"

// millerPageNav computes the new replyIndex and scrollOffset after a single j/k
// keypress in a Miller-layout detail pane (or PostDetail in tabs mode).
//
// Pager behaviour: scroll one line at a time within the current item; only advance
// to the next item once the current item's trailing edge becomes visible (delta>0),
// or retreat once the leading edge becomes visible (delta<0).
//
//   - replyStarts[i] is the start line of reply i within the full content.
//   - replyHeights[i] is the rendered height of reply i.
//   - replyIndex == -1 means the post itself is selected; 0+ indexes into the reply slice.
func millerPageNav(delta, paneH, postH int, replyStarts, replyHeights []int, replyIndex, scrollOffset int) (newReplyIndex, newScrollOffset int) {
	newReplyIndex = replyIndex
	newScrollOffset = scrollOffset

	totalLines := postH
	if n := len(replyStarts); n > 0 {
		totalLines = replyStarts[n-1] + replyHeights[n-1]
	}

	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if max := totalLines - paneH; v > max {
			if max < 0 {
				return 0
			}
			return max
		}
		return v
	}

	if delta > 0 {
		if replyIndex == -1 {
			viewBottom := scrollOffset + paneH - 1
			if viewBottom >= postH-1 && len(replyStarts) > 0 {
				newReplyIndex = 0
				newScrollOffset = clamp(replyStarts[0])
			} else {
				newScrollOffset = clamp(scrollOffset + 1)
			}
		} else {
			replyBottom := replyStarts[replyIndex] + replyHeights[replyIndex] - 1
			viewBottom := scrollOffset + paneH - 1
			if replyBottom > viewBottom {
				newScrollOffset = clamp(scrollOffset + 1)
			} else if replyIndex < len(replyStarts)-1 {
				newReplyIndex = replyIndex + 1
				newScrollOffset = clamp(replyStarts[newReplyIndex])
			}
		}
	} else {
		if replyIndex == -1 {
			newScrollOffset = clamp(scrollOffset - 1)
		} else {
			replyTop := replyStarts[replyIndex]
			if replyTop < scrollOffset {
				newScrollOffset = clamp(scrollOffset - 1)
			} else {
				newReplyIndex = replyIndex - 1
				var prevStart, prevH int
				if newReplyIndex == -1 {
					prevStart = 0
					prevH = postH
				} else {
					prevStart = replyStarts[newReplyIndex]
					prevH = replyHeights[newReplyIndex]
				}
				if prevH <= paneH {
					newScrollOffset = clamp(prevStart)
				} else {
					newScrollOffset = clamp(prevStart + prevH - paneH)
				}
			}
		}
	}
	return newReplyIndex, newScrollOffset
}

// sliceContent clips fullContent to a height-line window starting at offset.
// If lineCount fits within height, the full content is returned unchanged.
func sliceContent(fullContent string, offset, height, lineCount int) string {
	if lineCount <= height {
		return fullContent
	}
	if offset < 0 {
		offset = 0
	}
	if offset+height > lineCount {
		offset = lineCount - height
	}
	if offset < 0 {
		offset = 0
	}
	lines := strings.Split(fullContent, "\n")
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[offset:end], "\n")
}
