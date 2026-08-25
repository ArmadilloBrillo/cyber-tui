package imgview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"regexp"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// BadgeURLPrefix marks an InlineImageSlot.URL as a badge icon code (e.g.
// "badge:ph:crown") rather than a real HTTP URL — Fetch dispatches on it to
// FetchBadgeIcon instead of the normal HTTP-image path.
const BadgeURLPrefix = "badge:"

// badgeIconNameRe bounds an icon name/prefix to the charset every real
// Lucide/Lucide Lab/Phosphor icon name uses. SupporterIcon/GuildIcon values
// come from the API and are spliced directly into a URL path below, so this
// rejects anything that isn't a plain lowercase-kebab identifier before it
// gets near an HTTP request.
var badgeIconNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// badgeIconColor is the fixed fill/stroke color substituted for every icon's
// "currentColor" — these SVGs have no inherent color of their own (they're
// styled via CSS on the web), and terminal image protocols render fixed
// pixels with no access to the caller's theme, so there is no per-theme
// value to plug in here.
const badgeIconColor = "#e6e6e6"

// badgeIconRasterSize is the square pixel size icons are rasterized at,
// large enough to downscale cleanly to the 1-2 terminal cells a badge
// actually occupies (EncodeKitty/EncodeITerm2/EncodeSixel all fit-to-box).
const badgeIconRasterSize = 64

// ResolveBadgeIconURL turns a SupporterIcon/GuildIcon code into the raw SVG
// source URL for the icon library it names. The convention (confirmed
// against the live API/web client): no prefix means Lucide, "lucide-lab:"
// means Lucide Lab, "ph:" means Phosphor — all three are open-source SVG
// icon sets, not fonts, so a name resolves to one static asset file.
func ResolveBadgeIconURL(code string) (string, bool) {
	if name, ok := strings.CutPrefix(code, "lucide-lab:"); ok {
		if !badgeIconNameRe.MatchString(name) {
			return "", false
		}
		return "https://raw.githubusercontent.com/lucide-icons/lucide-lab/main/icons/" + name + ".svg", true
	}
	if name, ok := strings.CutPrefix(code, "ph:"); ok {
		if !badgeIconNameRe.MatchString(name) {
			return "", false
		}
		return "https://raw.githubusercontent.com/phosphor-icons/core/main/assets/regular/" + name + ".svg", true
	}
	if strings.Contains(code, ":") {
		return "", false // unrecognized prefix — don't guess
	}
	if !badgeIconNameRe.MatchString(code) {
		return "", false
	}
	return "https://cdn.jsdelivr.net/npm/lucide-static/icons/" + code + ".svg", true
}

// FetchBadgeIcon resolves code to its SVG source, downloads it, and
// rasterizes it to a badgeIconRasterSize×badgeIconRasterSize image — the
// same image.Image shape Fetch returns for a normal raster image, so it
// flows into EncodeKitty/EncodeITerm2/EncodeSixel unchanged.
func FetchBadgeIcon(ctx context.Context, code string) (image.Image, error) {
	rawURL, ok := ResolveBadgeIconURL(code)
	if !ok {
		return nil, fmt.Errorf("imgview: unrecognized badge icon %q", code)
	}
	body, err := fetchBody(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(body), badgeIconColor)
	if err != nil {
		return nil, fmt.Errorf("imgview: parse badge icon svg: %w", err)
	}
	icon.SetTarget(0, 0, badgeIconRasterSize, badgeIconRasterSize)
	img := image.NewRGBA(image.Rect(0, 0, badgeIconRasterSize, badgeIconRasterSize))
	scanner := rasterx.NewScannerGV(badgeIconRasterSize, badgeIconRasterSize, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(badgeIconRasterSize, badgeIconRasterSize, scanner), 1.0)
	return img, nil
}
