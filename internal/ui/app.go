package ui

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"math/rand"
	neturl "net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/sanitize"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
	"github.com/ragnar/cyber-tui/internal/update"
	"github.com/ragnar/cyber-tui/internal/version"
	"github.com/ragnar/cyber-tui/internal/youtube"
)

type screen int

const (
	screenLogin screen = iota
	screenFeed
	screenChatrooms
	screenCMail
	screenProfile
	screenPostDetail
	screenNotifications
	screenSettings
	screenBookmarks
	screenGuilds
	screenTopics
	screenJournal
	screenSearch
)

type focusTarget int

const (
	focusMenu   focusTarget = iota
	focusList               // list pane (compact post list in 3-pane Miller)
	focusDetail             // reading pane (full post view in 3-pane Miller)
)

// availableThemes is the ordered list of selectable themes shown in the picker.
var availableThemes = []string{"cyber", "c64", "vt320", "bland", "custom"}

// defaultThemeFilePath prefills the export/import path prompt, so a plain
// export-then-import round trip needs no retyping.
const defaultThemeFilePath = "~/cyber-tui-theme.json"

const (
	logoOrig          = "ᑕ¥βєяรקค¢є"
	logoHoldFrames    = 5 // frames held fully scrambled (~300ms at 60ms/frame)
	logoFrameInterval = 60 * time.Millisecond
)

var logoOrigRunes = []rune(logoOrig)

var logoCyberPool = []rune{
	'¥', '¢', '€', '£', '₿', '₽', '¤', '§', '©', '®',
	'α', 'β', 'γ', 'δ', 'ε', 'λ', 'μ', 'π', 'σ', 'φ', 'ψ', 'ω',
	'я', 'ю', 'э', 'ш', 'щ', 'ж', 'ф', 'г', 'ц', 'б',
	'א', 'ב', 'ג', 'ד', 'ה', 'כ', 'ל', 'מ', 'נ', 'ק', 'ש',
	'æ', 'ø', 'þ', 'ß', 'ñ', 'ç',
	'ᑕ', 'ᑎ', 'ᒋ', 'ᓯ', 'ᑲ', 'ᒪ',
	'∞', '∑', '√', '≈', '≠', '⊗', '⊕',
	'░', '▒', '▓',
}

func randomCyberRune(exclude rune) rune {
	for len(logoCyberPool) > 1 {
		r := logoCyberPool[rand.Intn(len(logoCyberPool))]
		if r != exclude {
			return r
		}
	}
	return logoCyberPool[0]
}

// pathPromptPurpose distinguishes what App does with a submitted PathPromptModel path.
type pathPromptPurpose int

const (
	pathPromptExport pathPromptPurpose = iota
	pathPromptImport
)

type App struct {
	layout      Layout
	layoutName  string // "tabs" (default) or "miller"; used when persisting to config
	client      api.Client
	tokens      model.Tokens
	currentUser model.User
	// ownApprenticeSlugs are the logged-in user's apprenticed guild slugs
	// (from GetUserGuilds on their own username), used to order the Guilds
	// tab — separate from ProfileModel's apprenticeships, which get
	// overwritten when viewing another user's profile.
	ownApprenticeSlugs []string
	active             screen
	focus              focusTarget
	width              int
	height             int

	// autoEmail and autoPassword are set from the config file.
	// When both are non-empty, Init fires loginCmd immediately.
	autoEmail    string
	autoPassword string

	// savedSession is set when a config file was loaded at startup.
	// When non-nil, Init fires tokenLoginCmd instead of showing the login screen.
	savedSession *config.Config

	// relaxed controls display density: false = dense (default), true = blank lines between items.
	relaxed bool

	// themePicker state — open with 't', close with Enter/Esc.
	themePickerOpen   bool
	themePickerCursor int    // index into availableThemes
	themePickerOrig   string // theme name when picker was opened (for Esc revert)

	// themeEditor state — opened from the theme picker with 'e' on the
	// "custom" row (or from Post Detail's try-theme key), closed by
	// SaveThemeMsg/CloseThemeEditorMsg.
	themeEditorOpen bool
	themeEditor     screens.ThemeEditorModel
	themeEditorOrig string // theme name before entering the editor, for close-without-save revert
	// themeEditorOrigPalette is a CurrentPalette() snapshot taken at the same
	// moment as themeEditorOrig, before the editor's preview starts mutating
	// theme's package-level customPalette. Needed to correctly revert when
	// themeEditorOrig == "custom": theme.Set("custom") alone would just
	// re-apply whatever the abandoned edit left in that shared variable
	// instead of the palette that was actually active before previewing.
	themeEditorOrigPalette theme.Palette

	// customPalette is the persisted custom theme, loaded from config. Nil
	// means the user has never saved one yet.
	customPalette *theme.Palette

	// pathPrompt state — opened from the theme picker's currently
	// highlighted row with 'x' (export) or 'i' (import), closed by
	// PathPromptSubmitMsg/PathPromptCancelMsg.
	pathPromptOpen    bool
	pathPrompt        screens.PathPromptModel
	pathPromptPurpose pathPromptPurpose
	// pathPromptOverwritePending holds the export path once it's been
	// flagged as already existing — an identical resubmit proceeds without
	// asking again; any other path (or a fresh Open) resets this.
	pathPromptOverwritePending string
	// pathPromptExportPalette is the palette to write when pathPromptPurpose
	// is pathPromptExport — whichever theme was highlighted (a built-in or
	// the saved custom theme) when 'x' was pressed, captured once so a later
	// picker-cursor move can't change what gets written.
	pathPromptExportPalette theme.Palette

	// helpModal state — open with '?', close with any key.
	helpModalOpen bool

	// leaderPending is armed by the "g" ("go to") leader key and resolved by
	// the very next keypress against screenForMnemonic — e.g. "g" then "f"
	// jumps to Feed. An unmapped follow-up key silently cancels it.
	leaderPending bool

	// urlPicker state — open with 'o' when multiple URLs are available.
	urlPickerOpen   bool
	urlPickerItems  []string
	urlPickerCursor int

	// iconPicker state — open with ctrl+] from any of the 7 chat/compose
	// text inputs.
	iconPickerOpen bool
	iconPicker     screens.IconPickerModel

	// attachURLPrompt state — open with ctrl+g from any focused compose
	// field. Submit either sets a native post attachment (Feed's new-post
	// panel or PostDetail's edit panel) or inserts markdown image syntax
	// wherever InsertIconMsg would land — see applyAttachURL.
	attachURLPromptOpen bool
	attachURLPrompt     screens.PathPromptModel

	// songPrompt state — open with ctrl+j from a focused cIRC composer
	// (supporter accounts only). Submit hands off a built "/song ..."
	// command via SetComposeValueMsg, the same mechanism ctrl+g's GIF
	// attach uses — see applyAttachURL and submitSongPrompt.
	songPromptOpen bool
	songPrompt     screens.SongPromptModel

	// imageCarousel state — populated when an image is opened from a picker
	// containing more than one image, letting left/right cycle between them
	// without closing the image modal. Nil imageCarouselItems means a plain
	// single-image view (existing behavior, arrows never shown).
	imageCarouselItems []string
	imageCarouselIndex int
	// carouselCycleGen is bumped on every left/right carousel keypress and
	// captured by the debounce tick cycleImageCarousel schedules (see its
	// doc comment) — holding the key down just moves imageCarouselIndex and
	// reschedules the tick; only the tick whose gen still matches when it
	// fires actually starts the (expensive: real decode+encode work even on
	// a cache hit, see openImageInTerminal) fetch for wherever the index
	// ended up. Without this, holding the key fired one fetch per repeat,
	// almost all wasted on results that would just be discarded by
	// imageFetchGen's newest-wins guard — confirmed live to leave the
	// carousel's counter racing far ahead of the displayed image, and,
	// after enough of a backlog, an occasional black screen (needing a
	// keypress to recover) plausibly from the sheer volume of near-
	// simultaneous full-frame renders that backlog produced.
	carouselCycleGen int
	// imageCache holds decoded images already fetched during the current
	// modal's lifetime, keyed by URL, so cycling back to one skips the
	// network fetch. Cleared whenever the modal closes.
	imageCache map[string]cachedImage

	// inlineImageCache holds encoded inline-image escape sequences, keyed by
	// inlineImageCacheKey (slot key + URL + column budget + protocol),
	// bounded by inlineImageCacheMaxBytes — see cacheInlineImage, which
	// evicts the oldest-inserted entry first once exceeded.
	// inlineImageCacheOrder/inlineImageCacheElems/inlineImageCacheBytes are
	// cacheInlineImage's bookkeeping for that eviction; nothing else should
	// touch them. inlineImageFetching tracks keys with a fetch already in
	// flight, so syncInlineImages (called every Update) doesn't refire the
	// same request every frame while it's pending. inlineImageFailedAt
	// records when a key last failed to fetch, so a permanently-broken URL
	// gets a cooldown (inlineImageFailureCooldown) instead of being retried
	// on every single Update its slot stays visible for.
	//
	// Sixel/iTerm2 have no placement-delete primitive (Kitty does, see
	// kittyPlacementIDs) and need two independent mechanisms:
	//
	//   - inlineImageVisibleRects (previous frame's key->rect map) and
	//     inlineImageStaleRows (this frame's absolute row numbers whose
	//     previously-drawn image content is now stale — a moved or removed
	//     image, whose old and new screen position can differ) — see
	//     syncInlineImageErasures' doc comment. These rows get an inert
	//     dirty marker (forceRowsDirty in layout.go) rather than a manual
	//     blank-fill: Bubble Tea's own per-line diff then resends each
	//     row's real, always-correct current content. Earlier designs tried
	//     a full tea.ClearScreen (flashed on every scroll) and then an
	//     out-of-band absolute-cursor blank-fill that accumulated forever
	//     waiting for an exact-rect "claim" that, for a scrolled-off or
	//     tab-switched-away image, could never come — corrupting whatever
	//     unrelated content later rendered at that same screen position.
	//     Forcing a real resend of the row's true content sidesteps both:
	//     it's always correct (not a guessed replacement), and it only
	//     needs to fire for the one transition frame — recomputed fresh
	//     every Update with no carry-forward — since even losing that one
	//     frame to Bubble Tea's renderer coalescing self-heals as soon as
	//     that row's content next changes, rather than corrupting anything.
	//   - inlineImageLastSelKey (previous frame's activeSelectionKey) and
	//     inlineImagePaintGen (bumped for a selection change that touches
	//     a visible image, read by injectInlineImages to force its
	//     trailing paint line "changed" to Bubble Tea's per-line diff)
	//     handle a selection change recoloring a card's border without
	//     moving anything — old and new position are identical here, so
	//     reissuing the same paint in place is safe. Selection changes
	//     that don't touch any visible image skip this — bumping on every
	//     selection move regardless caused a visible blink on every
	//     arrow-key step through a feed with any inline image on screen
	//     (an earlier, since-fixed regression).
	//
	// A smaller single-line flash still remains on the selection-touches-
	// image case and is an accepted ceiling, not an unnoticed gap: Lip
	// Gloss regenerates a card's whole bordered box as one string whenever
	// its border color changes (selection), so the image band row's bytes
	// differ too (border color chars) even though its interior content
	// didn't conceptually change — Bubble Tea's line-diff rewrites that
	// line, incidentally overwriting the image pixels, and
	// inlineImagePaintGen's reissue happens right after but is still a
	// second, visible terminal-side paint. The clean fix would be Bubble
	// Tea shipping DECSET 2026 synchronized-output support (v1.3.10, the
	// latest release, doesn't have it); hand-rolling begin/end markers
	// ourselves was evaluated and rejected — Bubble Tea's renderer diffs
	// and skips lines independently, so a mismatched pair could leave the
	// terminal stuck buffering indefinitely, a worse regression than the
	// flash. The other alternative — keeping the image band's border color
	// constant regardless of selection, so those rows never need rewriting
	// — would fix it at the source but requires replacing Lip Gloss's
	// single-call box styling with manual per-line border construction in
	// feed.go/postdetail.go, judged too large/risky for this.
	inlineImageCache        map[string]string
	inlineImageCacheOrder   *list.List
	inlineImageCacheElems   map[string]*list.Element
	inlineImageCacheBytes   int
	inlineImageFetching     map[string]bool
	inlineImageFailedAt     map[string]time.Time
	inlineImageVisibleRects map[string]inlineImageRect
	inlineImageStaleRows    []int
	// inlineImageStaleSince is when a row was last newly added to
	// inlineImageStaleRows. syncInlineImages runs after every tea.Msg the
	// whole app processes, including several independent tea.Tick loops
	// unrelated to the active screen (chat/RTDB heartbeats, notification
	// polls, gif-frame ticks) — so "the very next Update computed nothing
	// new" is not a reliable signal that a forced-dirty resend actually got
	// flushed to the terminal; it just as easily means an unrelated tick
	// fired first. Clearing inlineImageStaleRows requires both a quiet
	// Update AND inlineImageStaleGrace elapsed since this timestamp (see
	// syncInlineImages) — long enough to comfortably outlast known
	// multi-Update gaps like the feed's feedMergeAnimDelay, regardless of
	// how many unrelated Updates interleave.
	inlineImageStaleSince time.Time
	inlineImageLastSelKey string
	inlineImagePaintGen   int
	// imageRepaintGen is a monotonically incrementing counter, bumped
	// (never reset — same never-auto-expire posture as pendingKittyDeletes)
	// each time an inline-image repaint is newly triggered for either
	// protocol (a stale row in syncInlineImages, a selection change that
	// touches a visible image, or a size-changed carousel cycle in
	// handleImageViewer). Encoded via imageDirtyMarker (layout.go) into a
	// zero-width true-color SGR marker so two consecutive *actually
	// flushed* repaint frames never produce identical bytes for an
	// unchanged row — a fixed marker (or a %2 toggle) isn't enough here:
	// Bubble Tea's per-line diff would skip resending any row whose bytes
	// match the last flushed frame, which for Sixel's real erase
	// (sixelFullRepaint) means a skipped row comes back genuinely blank
	// rather than merely stale, and for iTerm2's in-place resend
	// (forceRowsDirty) means a skipped row silently keeps its old content —
	// confirmed live for Sixel as stale pixels on fast scroll and, worse,
	// the whole screen going black on fast carousel cycling; the identical
	// mechanism affects iTerm2's resend path, just with a less visually
	// obvious symptom (an occasional not-fully-drawn image on fast feed
	// scroll/refresh) — originally scoped to Sixel only in the round-5/6
	// fix (docs/plan-inline-images-improvements.md §10), extended to both
	// protocols once the same collision was confirmed possible for iTerm2.
	imageRepaintGen int

	// screenSwitchedAt is when a.active last changed (App.Update, comparing
	// active before/after updateInner). injectInlineImages (layout.go) uses
	// it to briefly hold back inline image draws right after a screen
	// switch — see inlineImageSwitchSettleDelay's doc comment for why: live
	// debug-log evidence (docs/plan-inline-images-improvements.md Round 7)
	// showed the app correctly recomputes and reissues the exact right
	// draw command on returning to a screen after a fast switch away and
	// back, yet the image still failed to render on real iTerm2 — pointing
	// at the terminal still processing the large, unrelated screen-redraw
	// content when the image's OSC sequence arrives, the same class of
	// issue already mitigated for the fullscreen carousel
	// (carouselCycleDebounce).
	screenSwitchedAt time.Time

	// kittyPlacementIDs assigns a stable id (used as both image id and
	// placement id, see imgview.EncodeKitty) to each inline image slot ever
	// seen, keyed by InlineImageSlot.Key. Entries are PERMANENT — never
	// removed once assigned, even after the slot scrolls off-screen and gets
	// deleted on the terminal side. This matters because inlineImageCacheKey
	// doesn't vary by id (only by slot key/URL/width/protocol): if a key's id
	// changed on every re-entry into view, a slot scrolling back into view
	// would hit its OLD cache entry (still embedding the OLD, already-deleted
	// id) and, since that old id sits permanently in pendingKittyDeletes (see
	// below), get deleted again the instant it's redrawn — invisible forever
	// after its first scroll-off. Keeping the id (and thus the cache entry)
	// stable for a key's whole session lifetime avoids that entirely.
	// kittyNextPlacementID is the session-lived counter handing out new ids —
	// never reused/freed within a session. This stays safe even now that
	// inlineImageCache is bounded (see its doc comment): evicting a cache
	// entry doesn't touch its id, since inlineImageCacheKey doesn't embed
	// one — a slot whose entry got evicted just re-fetches and re-encodes
	// using its already-stable id, same as any other cache miss. If ids were
	// ever evicted too, that would have to happen in the same step as
	// evicting the matching cache entry, or a reissued id could collide with
	// a still-cached payload embedding the old one.
	//
	// kittyVisibleKeys is the set of slot keys visible as of the last sync
	// call. Because kittyPlacementIDs is never pruned, "not in
	// kittyPlacementIDs" can no longer be used to detect a drop — nearly
	// every key ever seen is "not currently visible" at any given moment, not
	// just ones that just transitioned. Comparing against kittyVisibleKeys
	// instead of kittyPlacementIDs is what lets syncKittyPlacements detect
	// exactly the visible->invisible and invisible->visible transitions.
	//
	// pendingKittyDeletes is the set of dropped-out placement ids whose
	// delete sequence gets resent on every subsequent frame. Deletes are the
	// one part of this feature that's a single-shot event rather than
	// something re-emitted every frame a slot is visible (creates/
	// repositions are, by nature of the normal per-frame render loop) — and
	// Bubble Tea's renderer batches/throttles actual terminal writes, so it
	// can call View() many times between two real flushes and only the last
	// computed View() before a flush tick ever reaches the terminal. A
	// countdown-based resend budget was tried here first, decremented once
	// per Update; a fast enough scroll can still rack up enough Updates
	// between two real flushes to exhaust that budget on nothing but
	// never-flushed intermediate renders, losing the delete exactly like the
	// original single-shot version. Since Kitty placement ids are never
	// reused within a session (kittyNextPlacementID only grows), resending an
	// already-deleted id's delete is always a harmless no-op — so, like
	// imageNeedsCleanup above (same underlying race, same fix), entries here
	// are never auto-expired by a countdown; they're only removed early when
	// syncKittyPlacements reports the same key has become visible again (see
	// "revived" in syncInlineImages), which cancels the stale delete before it
	// can wipe out the slot's freshly redrawn placement.
	kittyPlacementIDs    map[string]int
	kittyNextPlacementID int
	kittyVisibleKeys     map[string]struct{}
	pendingKittyDeletes  map[int]struct{}

	// timezone is the active UTC offset label (e.g. "UTC+2"). Empty = UTC.
	// loc is the parsed *time.Location derived from timezone.
	timezone string
	loc      *time.Location

	login          screens.LoginModel
	feed           screens.FeedModel
	chatrooms      screens.ChatroomsModel
	cmail          screens.CMailModel
	profile        screens.ProfileModel
	postDetail     screens.PostDetailModel
	notifications  screens.NotificationsModel
	settingsScreen screens.SettingsModel
	bookmarks      screens.BookmarksModel
	guilds         screens.GuildsModel
	topics         screens.TopicsModel
	journal        screens.JournalModel
	search         screens.SearchModel

	// postDetailReturn is the screen to go back to when ESC is pressed in PostDetail.
	postDetailReturn screen

	// postDetailStack holds prior PostDetail snapshots pushed when an in-post
	// link (routeURL/urlPostLoadedMsg) is opened while already viewing a
	// post — Post A -> link -> Post B pushes A here so Esc from B restores it
	// (post, replies, selection, scroll) before falling back to
	// postDetailReturn once the stack is empty. Note: a stacked snapshot's
	// viewport keeps whatever width it had when pushed — a resize while
	// nested doesn't reach it until it's popped back and re-renders.
	postDetailStack []screens.PostDetailModel

	// profileReturn is the screen to go back to when ESC is pressed in a read-only profile.
	profileReturn screen

	// searchReturn is the screen to go back to when ESC is pressed at Search's
	// outermost level (blurred query, nothing left to peel back). Set whenever
	// '/' switches into Search from somewhere else.
	searchReturn screen

	// cmailReturn is the screen to go back to when ESC is pressed in a
	// deep-linked C-Mail conversation (see CMailModel.canGoBack).
	cmailReturn screen

	// chatroomsReturn is the screen to go back to when ESC is pressed in a
	// deep-linked Chatrooms room (see ChatroomsModel.canGoBack).
	chatroomsReturn screen

	// pendingReplyID is set when navigating to PostDetail from a reply/thread_reply
	// notification. After replies load, PostDetail scrolls to this reply, then it is cleared.
	pendingReplyID string

	// polledUnreadCount is the single source of truth for the tab badge unread count.
	// It is synced from: 60-second server poll, m/M key, and enter on a notification.
	// Never overwrite with the local list count — the server count is always authoritative.
	polledUnreadCount int
	// polledUnreadCountExact is false once polledUnreadCount was capped at 100 by the
	// server (v0.8.5+); the badge renders "99+" instead of the number in that case.
	polledUnreadCountExact bool

	// settings holds the user's preferences fetched from GET /v1/settings on login.
	settings model.Settings

	// wanderLust is the local config value for wander mode. Defaults to false (off).
	wanderLust bool
	// maxThreadDepth is the local config value for reply nesting depth. Defaults to 3.
	maxThreadDepth int
	// feedManualRefreshOnly is the local config value disabling Feed's
	// background poll (see docs/39-feed-background-poll.md). Defaults to
	// false (auto-poll on).
	feedManualRefreshOnly bool
	// typingIndicatorsEnabled is the local config value (inverted from
	// config.Config.TypingIndicatorsDisabled) gating C-Mail's typing
	// indicators — see docs/00-battery-audit.md item #6. Defaults to false
	// until WithSavedPreferences runs (then true unless the user opted out),
	// same zero-value-before-hydration window every local preference has.
	typingIndicatorsEnabled bool

	// graphicsProtocol is the terminal image display protocol detected at startup.
	// ProtocolNone means no image display is available and URLs open in a browser.
	graphicsProtocol imgview.GraphicsProtocol

	// imageViewer is the user's preference from config.ImageViewer. When "browser",
	// image URLs always open in the OS browser even if a protocol is detected.
	imageViewer string

	// graphicsProtocolName is the user's raw override preference from
	// config.GraphicsProtocol ("" autodetects; "kitty"/"iterm2"/"sixel" forces a
	// choice). Kept alongside the already-resolved graphicsProtocol so the
	// settings screen can display/edit it; saving re-resolves graphicsProtocol
	// (see settingsSavedMsg handling) without re-running the Sixel DA1 probe,
	// which requires raw terminal access before Bubble Tea takes over stdin.
	graphicsProtocolName string

	// inlineImages is the user's raw preference from config.InlineImages.
	// See canInlineImages for the fully-gated value broadcast to screens.
	inlineImages bool

	// dithering is the user's preference from config.Dithering. See
	// ditheringEnabled for the gated value used when encoding images.
	dithering bool

	// ditherSharpness is the user's preference from config.GetDitherSharpness
	// ("rough"/"medium"/"sharp"), controlling the dithering pixelation block
	// size via imgview.PixelSizeForSharpness. Only meaningful when dithering
	// is on.
	ditherSharpness string

	// imageScale multiplies the computed size of the fullscreen image modal.
	// Starts from config.Config.GetImageScale() and is live-adjustable with
	// +/- while the modal is open (session-only, not persisted). <= 0 is
	// treated as the default (1.0) — see openImageInTerminal.
	imageScale float64

	// imageModal holds the state for the inline image overlay. When imageModalOpen
	// is true, View composites the encoded image sequence over the base content.
	imageModalOpen    bool
	imageModalEncoded string
	imageModalCols    int
	imageModalRows    int
	// imageModalURL is the URL of the currently displayed modal image, kept
	// so a live +/- scale adjustment can re-run openImageInTerminal for the
	// same image (a cache hit, so it only re-encodes, not re-fetches).
	imageModalURL string
	// imageModalPrevRows/Cols are the modal's interior size as of the
	// previous frame — 0 means no previous box to worry about (a fresh
	// open, not a carousel cycle). Used by compositeOverlays to force the
	// previous box's row range dirty when a cycle changes its size, instead
	// of a full tea.ClearScreen — see compositeOverlays' doc comment.
	imageModalPrevRows int
	imageModalPrevCols int
	imageNeedsCleanup  bool // true after modal closes until a delete-placement frame reaches the terminal
	imageFetchGen      int  // bumped on every fetch and on close; stale imageFetchedMsg results are dropped

	// ephemeral marks an SSH-hosted session whose state must never be read from
	// or written to the host operator's config file.
	ephemeral bool

	// bookmarkedPostIDs and bookmarkedReplyIDs track which posts/replies the current
	// user has bookmarked, populated from the bookmarks list and kept in sync on
	// create/delete. Used to show [★] indicators in feed, postdetail, and topics.
	bookmarkedPostIDs  map[string]struct{}
	bookmarkedReplyIDs map[string]struct{}
	// postBookmarkIDs and replyBookmarkIDs are reverse lookups: content ID → bookmark UUID.
	// Required to call deleteBookmarkCmd when the user toggles off a bookmark with 'b'.
	postBookmarkIDs  map[string]string // postID  → bookmark UUID
	replyBookmarkIDs map[string]string // replyID → bookmark UUID

	// watchedPostIDs tracks which thread-root posts the current user is watching.
	// Populated progressively at login via GET /v1/watches (all pages) and kept in
	// sync on watch/unwatch. Used to show [◉] indicators in feed and post detail.
	watchedPostIDs map[string]struct{}

	// notifyText is the transient global notification shown in place of the status
	// bar. Empty means no notification is visible. notifyGen is bumped on every new
	// notification and on dismissal so a stale expire tick can never clear a newer one.
	notifyText  string
	notifyLevel notifyLevel
	notifyGen   int

	// settingsSaveSeq is bumped on every SaveSettingsMsg dispatch. Its value
	// is stamped onto the resulting settingsSavedMsg so an out-of-order
	// completion — e.g. an earlier save still waiting on the UpdateSettings
	// network round-trip while a later save (dispatched from a second ctrl+s
	// before the first returned) finishes and persists first — is dropped
	// instead of clobbering the newer values, in memory and on disk, with
	// its own stale snapshot.
	settingsSaveSeq int

	// sessionGen is bumped in handleUnauthorized on session expiry, so the
	// self-rescheduling poll/wander/logo-idle tea.Tick chains started by
	// afterLoginCmd (each stamped with the gen they were scheduled under)
	// drop themselves instead of doing work or rescheduling once stale —
	// otherwise those chains kept running forever after logout, and could
	// double up if the user logged back in while an old set was still ticking.
	sessionGen int

	logoText      string
	logoPhase     logoAnimPhase
	logoFrame     int
	logoPositions []int // shuffled index order for the current animation cycle
}

func NewApp(client api.Client) App {
	return App{
		layout:             TabsLayout{},
		layoutName:         "tabs",
		client:             client,
		active:             screenLogin,
		focus:              focusMenu,
		loc:                time.UTC,
		wanderLust:         false,
		login:              screens.NewLoginModel(""),
		feed:               screens.NewFeedModel(),
		chatrooms:          screens.NewChatroomsModel("", client),
		cmail:              screens.NewCMailModel("", "", client),
		profile:            screens.NewProfileModel(),
		postDetail:         screens.NewPostDetailModel(),
		notifications:      screens.NewNotificationsModel(),
		settingsScreen:     screens.NewSettingsModel(),
		bookmarks:          screens.NewBookmarksModel(),
		guilds:             screens.NewGuildsModel(),
		topics:             screens.NewTopicsModel(),
		journal:            screens.NewJournalModel(0),
		search:             screens.NewSearchModel(),
		pathPrompt:         screens.NewPathPromptModel(),
		iconPicker:         screens.NewIconPickerModel(),
		attachURLPrompt:    screens.NewPathPromptModel(),
		songPrompt:         screens.NewSongPromptModel(),
		bookmarkedPostIDs:  make(map[string]struct{}),
		bookmarkedReplyIDs: make(map[string]struct{}),
		postBookmarkIDs:    make(map[string]string),
		replyBookmarkIDs:   make(map[string]string),
		watchedPostIDs:     make(map[string]struct{}),
		logoText:           logoOrig,
		logoPhase:          logoPhaseIdle,
	}
}

// WithSavedEmail pre-fills the email field on the login screen.
// Used when a previous session email is known but no token is available.
func (a App) WithSavedEmail(email string) App {
	if email != "" {
		a.login = screens.NewLoginModel(email)
	}
	return a
}

// WithAutoLogin pre-fills credentials loaded from the environment.
// When both email and password are non-empty, Init skips the login screen.
func (a App) WithAutoLogin(email, password string) App {
	a.autoEmail = email
	a.autoPassword = password
	return a
}

// WithSavedPreferences loads display/behavior preferences from a persisted
// config. Unlike WithSavedSession, this does not require a refresh token, so
// it must be applied on every launch regardless of login method — otherwise
// a token-expiry relogin silently resets these to zero values in memory,
// and a later Settings save clobbers the real values on disk.
func (a App) WithSavedPreferences(s config.Config) App {
	a.relaxed = s.Density == "relaxed"
	a.timezone = s.Timezone
	a.loc = s.GetLocation()
	a.wanderLust = s.WanderLust
	a.feedManualRefreshOnly = s.FeedManualRefreshOnly
	a.typingIndicatorsEnabled = !s.TypingIndicatorsDisabled
	a.maxThreadDepth = s.GetMaxThreadDepth()
	a.imageViewer = s.ImageViewer
	a.graphicsProtocolName = s.GraphicsProtocol
	a.inlineImages = s.InlineImages
	a.dithering = s.Dithering
	a.ditherSharpness = s.GetDitherSharpness()
	a.imageScale = s.GetImageScale()
	a.layoutName = s.Layout
	a.layout = layoutFromName(s.Layout)
	a.customPalette = s.CustomPalette
	return a
}

// WithSavedSession attaches a persisted session loaded from ~/.cyber-tui.json.
// When set, Init attempts to resume the session via token refresh instead of
// showing the login screen.
func (a App) WithSavedSession(s config.Config) App {
	a.savedSession = &s
	a = a.WithSavedPreferences(s)
	return a
}

func layoutFromName(name string) Layout {
	if name == "miller" {
		return MillerLayout{}
	}
	return TabsLayout{}
}

// WithGraphicsProtocol sets the terminal graphics protocol detected at startup.
// When proto is ProtocolNone the image viewer feature is disabled entirely.
func (a App) WithGraphicsProtocol(proto imgview.GraphicsProtocol) App {
	a.graphicsProtocol = proto
	return a
}

// WithEphemeralSession marks the App as a remote SSH-hosted session. Such a
// session must not persist or read session credentials and display preferences
// from the host operator's config file.
func (a App) WithEphemeralSession() App {
	a.ephemeral = true
	return a
}

// saveConfigMu serializes saveConfig's load-mutate-write cycle across the
// several tea.Cmd goroutines that can call it concurrently (settings save,
// density toggle, theme save, login, ...). Without it, two overlapping calls
// each read the same stale snapshot and the last write wins, silently
// discarding whichever fields the other call touched.
var saveConfigMu sync.Mutex

// saveConfig loads the persisted config, applies mutate, and writes it back. It
// is a no-op for ephemeral (SSH-hosted) sessions.
func (a *App) saveConfig(mutate func(cfg *config.Config)) {
	if a.ephemeral {
		return
	}
	saveConfigMu.Lock()
	defer saveConfigMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	mutate(&cfg)
	_ = config.Save(cfg)
}

// --- init ---

func (a App) Init() tea.Cmd {
	if a.savedSession != nil && a.savedSession.RefreshToken != "" {
		return a.tokenLoginCmd(a.savedSession.RefreshToken)
	}
	if a.autoEmail != "" && a.autoPassword != "" {
		return a.loginCmd(a.autoEmail, a.autoPassword)
	}
	return a.login.Init()
}

// --- update ---

// Update is the top-level Bubble Tea update function. It chains domain
// handlers so each can claim the message and return early. WindowSizeMsg is
// handled first and always falls through to delegateUpdate so the active
// screen can also react to it.
// Update is the top-level bubbletea entry point. It delegates to updateInner
// for all existing message handling, then runs syncInlineImages once per
// message on the resulting state — after the active screen has already
// processed msg, so VisibleInlineImages() reflects any scroll/selection
// change from this same message — batching its command (if any) with
// whatever updateInner returned.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevActive := a.active
	a2, cmd := a.updateInner(msg)
	if a2.active != prevActive {
		// See screenSwitchedAt's doc comment (App struct) and
		// inlineImageSwitchSettleDelay's — injectInlineImages uses this to
		// briefly hold back inline image draws right after a screen switch.
		a2.screenSwitchedAt = time.Now()
	}
	a3, syncCmd := a2.syncInlineImages()
	if syncCmd == nil {
		return a3, cmd
	}
	return a3, tea.Batch(cmd, syncCmd)
}

func (a App) updateInner(msg tea.Msg) (App, tea.Cmd) {
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a = a.applyWindowSize(m)
		contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(m.Width), Height: a.layout.ContentHeight(m.Height)}
		return a, a.delegateUpdate(contentMsg)
	}
	// Any keypress dismisses a visible notification early. We do NOT return here,
	// so the key still flows on to do its normal job; bumping notifyGen neutralizes
	// the pending expire tick.
	if _, ok := msg.(tea.KeyMsg); ok && a.notifyText != "" {
		a.notifyText = ""
		a.notifyGen++
	}
	// Left/right cycle through a picker-opened image carousel without closing
	// the modal. Any other keypress closes it — consume the key so it doesn't
	// accidentally trigger another action while the modal is visible.
	if km, ok := msg.(tea.KeyMsg); ok && a.imageModalOpen {
		if len(a.imageCarouselItems) > 1 {
			switch km.String() {
			case "left":
				return a.cycleImageCarousel(-1)
			case "right":
				return a.cycleImageCarousel(+1)
			}
		}
		switch km.String() {
		case "+", "=":
			return a.adjustImageScale(imageScaleStep)
		case "-":
			return a.adjustImageScale(-imageScaleStep)
		}
		a.imageModalOpen = false
		a.chatrooms = a.chatrooms.SetAnimPaused(false)
		a.cmail = a.cmail.SetAnimPaused(false)
		a.imageNeedsCleanup = (a.graphicsProtocol == imgview.ProtocolKitty)
		a.imageCarouselItems = nil
		a.imageCarouselIndex = 0
		a.imageFetchGen++    // invalidate anything still in flight
		a.carouselCycleGen++ // invalidate any pending debounce tick — see its doc comment for why a stale one firing after imageCarouselItems is nil'd would panic
		a.imageCache = nil
		if a.graphicsProtocol == imgview.ProtocolSixel {
			// Sixel has no delete-placement primitive like Kitty's a=d,d=A, and
			// Bubble Tea's diff renderer can skip re-emitting a row whose text
			// happens to match the prior frame, leaving stray pixels behind. A
			// full repaint is the only reliable way to clear them.
			return a, tea.ClearScreen
		}
		return a, nil
	}
	if a2, cmd, ok := a.handleKeys(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleAuth(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleFeed(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handlePostDetail(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleChatrooms(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleCMail(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleRelativeTimeTick(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleProfile(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleNotifications(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleSettings(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleThemeEditor(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handlePathPrompt(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleBookmarks(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleWatches(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleGuilds(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleTopics(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleJournal(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleSearch(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleUnauthorized(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleLogoAnim(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleNotify(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleImageViewer(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleInlineImageFetched(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleSongMetadataFetched(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleErr(msg); ok {
		return a2, cmd
	}
	return a, a.delegateUpdate(msg)
}

// updateAll sends msg to every screen, discarding returned commands.
// Adding a new screen: add one line here. All broadcast helpers call this.
func (a App) updateAll(msg tea.Msg) App {
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.settingsScreen, _ = a.settingsScreen.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.journal, _ = a.journal.Update(msg)
	a.search, _ = a.search.Update(msg)
	return a
}

// broadcastConfig pushes the current display settings to all screens.
// Call this whenever loc, relaxed, or dimensions change outside of a
// WindowSizeMsg (e.g. after login, timezone change, or density toggle).
func (a *App) broadcastConfig() {
	msg := screens.SharedConfigMsg{Width: a.layout.ContentWidth(a.width), Height: a.height, Loc: a.loc, Relaxed: a.relaxed, Settings: a.settings, WanderLust: a.wanderLust, FeedManualRefreshOnly: a.feedManualRefreshOnly, TypingIndicatorsEnabled: a.typingIndicatorsEnabled, MaxThreadDepth: a.maxThreadDepth, Timezone: a.timezone, ImageViewer: a.imageViewer, GraphicsProtocol: a.graphicsProtocolName, InlineImages: a.inlineImages, InlineImagesEnabled: a.canInlineImages(), Dithering: a.dithering, DitherSharpness: a.ditherSharpness, OwnGuildSlug: a.currentUser.GuildSlug, OwnApprenticeSlugs: a.ownApprenticeSlugs, LayoutName: a.layoutName}
	*a = a.updateAll(msg)
}

// broadcastBookmarkedIDs pushes the current bookmarked-ID sets to all screens
// that render posts or replies (feed, postDetail, topics). Call this whenever
// the sets change (bookmark loaded, created, or deleted).
func (a *App) broadcastBookmarkedIDs() {
	msg := screens.BookmarkedIDsMsg{
		PostIDs:  a.bookmarkedPostIDs,
		ReplyIDs: a.bookmarkedReplyIDs,
	}
	a.feed, _ = a.feed.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.search, _ = a.search.Update(msg)
}

// broadcastWatchedIDs pushes the current watched-post ID set to all screens
// that render posts (feed, postDetail, guilds, topics). Call this whenever
// the set changes (progressive load page, watch, unwatch).
func (a *App) broadcastWatchedIDs() {
	msg := screens.WatchedPostIDsMsg{PostIDs: a.watchedPostIDs}
	a.feed, _ = a.feed.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.search, _ = a.search.Update(msg)
}

// applyWindowSize stores the new terminal dimensions and broadcasts the size
// to all screens so their viewports initialise before they become active.
// The active screen gets a second update via delegateUpdate, which is harmless.
func (a App) applyWindowSize(m tea.WindowSizeMsg) App {
	a.width = m.Width
	a.height = m.Height
	contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(m.Width), Height: a.layout.ContentHeight(m.Height)}
	return a.updateAll(contentMsg)
}

// handleKeys processes tea.KeyMsg events: modal intercepts, focused-input
// bypass, and all global keyboard shortcuts.
func (a App) handleKeys(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil, false
	}
	// Modal overlays intercept all keys while open.
	if a.themePickerOpen {
		model, cmd := a.handleThemePickerKey(m)
		return model.(App), cmd, true
	}
	if a.themeEditorOpen {
		model, cmd := a.handleThemeEditorKey(m)
		return model.(App), cmd, true
	}
	if a.pathPromptOpen {
		model, cmd := a.handlePathPromptKey(m)
		return model.(App), cmd, true
	}
	if a.helpModalOpen {
		model, cmd := a.handleHelpModalKey(m)
		return model.(App), cmd, true
	}
	if a.urlPickerOpen {
		model, cmd := a.handleURLPickerKey(m)
		return model.(App), cmd, true
	}
	if a.iconPickerOpen {
		model, cmd := a.handleIconPickerKey(m)
		return model.(App), cmd, true
	}
	if a.attachURLPromptOpen {
		model, cmd := a.handleAttachURLPromptKey(m)
		return model.(App), cmd, true
	}
	if a.songPromptOpen {
		model, cmd := a.handleSongPromptKey(m)
		return model.(App), cmd, true
	}
	// When a screen has a focused text input, let it consume all keys.
	// ctrl+c is kept as a hard escape hatch; a handful of other global
	// shortcuts get a ctrl-prefixed twin that reaches through too, since
	// their bare key is unreachable while chatting (CIRC/C-Mail's compose
	// input is focused for the entire detail view, not just a transient
	// sub-mode like Feed's reply box): ctrl+o (open link), ctrl+q (quit),
	// ctrl+t (theme picker), ctrl+] (icon picker), ctrl+left/right (cycle
	// tabs). ctrl+/ (search)
	// was tried and removed — the byte a physical ctrl+/ keystroke sends is
	// inconsistent across terminals (0x1F on most, a literal NUL on e.g. Git
	// Bash/MinTTY, indistinguishable there from ctrl+space/ctrl+2/ctrl+@), so
	// there's no reliable encoding to match on.
	if a.activeScreenHasFocusedInput() {
		if m.String() == "ctrl+c" {
			return a, tea.Quit, true
		}
		// A backgrounded Circ room or C-Mail conversation resumes detail mode
		// (and its "always focused" compose input, see
		// ChatroomsModel.InputFocused / CMailModel.InputFocused) the instant
		// you tab back into it — so plain left/right otherwise gets captured
		// into a box you never asked to type into, forcing ctrl+left/right
		// for what was plain left/right on every other tab a moment ago. An
		// empty compose box has nothing for left/right to do anyway, so let
		// it fall through to tab-cycling in that case only; once there's
		// text (typed just now, or a draft left over from before
		// backgrounding), left/right goes back to normal cursor movement and
		// ctrl+left/right remains the way out, same as today.
		bareArrowEscapesEmptyCompose := (m.String() == "left" || m.String() == "right") &&
			((a.active == screenChatrooms && a.chatrooms.ComposeEmpty()) ||
				(a.active == screenCMail && a.cmail.ComposeEmpty()))
		switch {
		case m.String() == "ctrl+o", m.String() == "ctrl+q", m.String() == "ctrl+t",
			m.String() == "ctrl+]", m.String() == "ctrl+g", m.String() == "ctrl+j", m.String() == "ctrl+left", m.String() == "ctrl+right", bareArrowEscapesEmptyCompose:
			// fall through to the global switch below
		default:
			return a, nil, false // fall through to delegateUpdate
		}
	}
	// "g" arms the leader key; the very next keypress resolves against
	// screenForMnemonic regardless of what it is (even another global key
	// like "t" or "q"), so it must be checked ahead of the switch below.
	if a.leaderPending {
		a.leaderPending = false
		if a.active != screenLogin {
			if s, ok := screenForMnemonic(m.String()); ok {
				var cmd tea.Cmd
				a, cmd = activateScreen(a, s)
				if s == screenSearch {
					// activateScreen leaves Search in whatever state it was
					// last left in (correct for arrow-cycling, which no
					// longer reaches Search at all) — "g s" is a deliberate
					// "go to Search" action like '/', so it must always focus
					// the query box the same way '/' already does below.
					a.search = a.search.FocusQuery()
				}
				return a, cmd, true
			}
		}
		return a, nil, true
	}
	if m.String() == "g" {
		if a.active != screenLogin {
			a.leaderPending = true
			return a, nil, true
		}
	}
	switch m.String() {
	case "t", "ctrl+t":
		if a.active != screenLogin {
			a.themePickerOpen = true
			a.themePickerOrig = theme.CurrentName()
			a.themePickerCursor = themeIndex(theme.CurrentName())
			return a, nil, true
		}
	case "ctrl+]":
		if a.activeScreenHasFocusedInput() {
			width := int(float64(a.width) * modalScreenMarginFrac)
			if mw := a.layout.ModalMaxWidth(a.width); width > mw {
				width = mw
			}
			height := int(float64(a.height) * modalScreenMarginFrac)
			a.iconPickerOpen = true
			var cmd tea.Cmd
			a.iconPicker, cmd = a.iconPicker.Open()
			a.iconPicker = a.iconPicker.SetSize(width, height)
			return a, cmd, true
		}
	case "ctrl+g":
		if a.activeScreenHasFocusedInput() {
			a.attachURLPromptOpen = true
			var cmd tea.Cmd
			a.attachURLPrompt, cmd = a.attachURLPrompt.Open("attach image/gif url", "")
			return a, cmd, true
		}
	case "ctrl+j":
		if a.active == screenChatrooms && a.activeScreenHasFocusedInput() {
			if !a.currentUser.IsSupporter {
				a.chatrooms = a.chatrooms.AppendSystemMessage(a.chatrooms.ActiveRoomSlug(), "*** song attachments require supporter status")
				return a, nil, true
			}
			a.songPromptOpen = true
			var cmd tea.Cmd
			a.songPrompt, cmd = a.songPrompt.Open()
			return a, cmd, true
		}
	case "v":
		if a.active != screenLogin {
			a.relaxed = !a.relaxed
			a.broadcastConfig()
			relaxed := a.relaxed
			return a, func() tea.Msg {
				a.saveConfig(func(cfg *config.Config) {
					if relaxed {
						cfg.Density = "relaxed"
					} else {
						cfg.Density = ""
					}
				})
				return nil
			}, true
		}
	case "?":
		if a.active != screenLogin {
			a.helpModalOpen = true
			return a, nil, true
		}
	case "o", "ctrl+o":
		if a.active != screenLogin {
			app, cmd := a.handleOpenURL(a.getFocusedURLs())
			return app, cmd, true
		}
	case "/":
		if a.active != screenLogin {
			if a.active != screenSearch {
				a.searchReturn = a.active
			}
			a.cmail = a.cmail.SetFocused(false)
			a.chatrooms = a.chatrooms.SetFocused(false)
			a.active = screenSearch
			a.search = a.search.FocusQuery()
			return a, nil, true
		}
	case "ctrl+c", "q", "ctrl+q":
		if a.active != screenLogin {
			return a, tea.Quit, true
		}
	case "esc":
		if a.active == screenLogin {
			return a, tea.Quit, true
		}
	}
	return a.layout.HandleNav(m, a)
}

// handleAuth processes login/registration flow messages.
func (a App) handleAuth(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SubmitLoginMsg:
		return a, a.loginCmd(msg.Email, msg.Password), true
	case loginSuccessMsg:
		a.tokens = msg.tokens
		a.currentUser = msg.user
		a.cmail = screens.NewCMailModel(msg.user.Username, msg.user.ID, a.client)
		a.chatrooms = screens.NewChatroomsModel(msg.user.Username, a.client)
		// Initialize the fresh models' viewports with the current terminal size.
		if a.width > 0 {
			contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(a.width), Height: a.layout.ContentHeight(a.height)}
			a.cmail, _ = a.cmail.Update(contentMsg)
			a.chatrooms, _ = a.chatrooms.Update(contentMsg)
		}
		loginCmd := a.afterLoginCmd()
		return a, loginCmd, true
	case screens.LoginErrMsg:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd, true
	case screens.ResendVerificationMsg:
		return a, a.resendVerificationCmd(msg.IDToken), true
	case screens.ResendVerificationResultMsg:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd, true
	}
	return a, nil, false
}

// handleFeed processes feed and post-navigation messages.
func (a App) handleFeed(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case feedLoadedMsg:
		a.feed = a.feed.SetPosts(msg.posts, msg.cursor)
		var detailCmd tea.Cmd
		a.feed, detailCmd = a.feed.CurrentDetailCmd()
		// Auto-fill the compact list column if the initial page is shorter than it.
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.feed.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadFeedPageCmd(msg.cursor)), true
		}
		return a, detailCmd, true
	case feedPageMsg:
		a.feed = a.feed.AppendPosts(msg.posts, msg.cursor)
		// Keep auto-filling until the compact list column is full.
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.feed.PostCount() < min {
			return a, a.loadFeedPageCmd(msg.cursor), true
		}
		return a, nil, true
	case screens.RefreshFeedMsg:
		return a, a.loadFeedCmd(), true
	case screens.LoadMoreFeedMsg:
		return a, a.loadFeedPageCmd(msg.Cursor), true
	case screens.LoadFeedDetailMsg:
		return a, a.loadFeedDetailCmd(msg.PostID), true
	case screens.FeedDetailRepliesMsg:
		a.feed, _ = a.feed.Update(msg)
		return a, nil, true
	case screens.FeedDetailNavMsg:
		a.feed, _ = a.feed.Update(msg)
		return a, nil, true
	case screens.ShowPostMsg:
		a.postDetailReturn = a.active
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	case screens.ShowPostForReplyMsg:
		a.postDetailReturn = screenFeed
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		var openCmd tea.Cmd
		a.postDetail, openCmd = a.postDetail.OpenCompose()
		return a, tea.Batch(a.loadRepliesCmd(msg.Post.ID), openCmd), true
	case screens.ShowUserProfileMsg:
		if a.active != screenFeed {
			return a, nil, false
		}
		a.profileReturn = screenFeed
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.DeletePostMsg:
		if a.active != screenFeed {
			return a, nil, false
		}
		postID := msg.PostID
		return a, a.deletePostCmd(postID, true), true
	case postDeletedMsg:
		if msg.fromFeed {
			// Deleted from feed: remove locally.
			a.feed = a.feed.RemovePost(msg.postID)
		} else {
			// Deleted from post detail: navigate to feed and reload.
			a.active = screenFeed
			a.postDetailStack = nil
			return a, a.loadFeedCmd(), true
		}
		return a, nil, true
	case screens.FlagPostMsg:
		if a.active != screenFeed {
			return a, nil, false
		}
		return a, a.flagPostCmd(msg.PostID, msg.Reason), true
	}
	return a, nil, false
}

// handlePostDetail processes post detail, reply, and compose messages.
func (a App) handlePostDetail(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case repliesLoadedMsg:
		if msg.postID != a.postDetail.PostID() {
			// Superseded: the user navigated away from this post (and,
			// possibly, back to a different one) before this request
			// resolved. Applying it anyway would silently rewrite the
			// current post's reply tree with another post's data — see
			// repliesLoadedMsg's doc comment.
			return a, nil, true
		}
		a.postDetail = a.postDetail.SetReplies(msg.replies)
		if a.pendingReplyID != "" {
			a.postDetail = a.postDetail.ScrollToReply(a.pendingReplyID)
			a.pendingReplyID = ""
		}
		return a, nil, true
	case screens.SubmitNewPostMsg:
		return a, a.createPostCmd(msg.Content, msg.Title, msg.Slug, msg.Topics, msg.IsPublic, msg.IsNSFW, msg.AttachmentURL), true
	case postCreatedMsg:
		return a, a.loadFeedCmd(), true
	case postConvertedToNoteMsg:
		a, notifyCmd := a.notify(notifyWarn, "posted too soon after your last entry — saved to your Journal instead")
		return a, tea.Batch(notifyCmd, a.loadFeedCmd()), true
	case screens.SubmitPostEditMsg:
		return a, a.editPostCmd(msg.PostID, msg.Content, msg.Title, msg.Topics, msg.IsPublic, msg.IsNSFW, msg.AttachmentURL, msg.AttachmentTouched, msg.OtherAttachments), true
	case postEditedMsg:
		// Both Feed and PostDetail may hold their own local copy of this post;
		// PostDetail's ApplyPostEdit has no postID param, so guard it here —
		// otherwise editing from Feed while PostDetail has an unrelated post
		// open would overwrite that unrelated post's content.
		a.feed = a.feed.ApplyPostEdit(msg.postID, msg.content, msg.title, msg.topics, msg.isPublic, msg.isNSFW, msg.editedAt, msg.attachments, msg.attachmentsTouched)
		if a.postDetail.PostID() == msg.postID {
			a.postDetail = a.postDetail.ApplyPostEdit(msg.content, msg.title, msg.topics, msg.isPublic, msg.isNSFW, msg.editedAt, msg.attachments, msg.attachmentsTouched)
		}
		return a, nil, true
	case screens.SubmitReplyMsg:
		return a, a.createReplyCmd(msg.PostID, msg.Content, msg.ParentReplyID), true
	case screens.SubmitReplyEditMsg:
		return a, a.editReplyCmd(msg.ReplyID, msg.Content), true
	case replyEditedMsg:
		a.postDetail = a.postDetail.ApplyReplyEdit(msg.replyID, msg.content, msg.editedAt)
		return a, nil, true
	case replyCreatedMsg:
		if a.settings.AutoWatchOnReply {
			if _, alreadyWatched := a.watchedPostIDs[msg.postID]; !alreadyWatched {
				newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+1)
				for k := range a.watchedPostIDs {
					newIDs[k] = struct{}{}
				}
				newIDs[msg.postID] = struct{}{}
				a.watchedPostIDs = newIDs
				a.broadcastWatchedIDs()
			}
		}
		a.pendingReplyID = msg.replyID
		return a, a.loadRepliesCmd(msg.postID), true
	case screens.BackToFeedMsg:
		if n := len(a.postDetailStack); n > 0 {
			a.postDetail = a.postDetailStack[n-1]
			a.postDetailStack = a.postDetailStack[:n-1]
			return a, nil, true
		}
		a.active = a.postDetailReturn
		a.postDetail = a.postDetail.Close()
		return a, nil, true
	case screens.ShowUserProfileMsg:
		if a.active != screenPostDetail {
			return a, nil, false
		}
		a.profileReturn = screenPostDetail
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.DeletePostMsg:
		if a.active != screenPostDetail {
			return a, nil, false
		}
		postID := msg.PostID
		return a, a.deletePostCmd(postID, false), true
	case screens.DeleteReplyMsg:
		return a, a.deleteReplyCmd(msg.ReplyID), true
	case replyDeletedMsg:
		a.postDetail = a.postDetail.RemoveReply(msg.replyID)
		return a, nil, true
	case screens.FlagPostMsg:
		if a.active != screenPostDetail {
			return a, nil, false
		}
		return a, a.flagPostCmd(msg.PostID, msg.Reason), true
	case screens.FlagReplyMsg:
		return a, a.flagReplyCmd(msg.ReplyID, msg.Reason), true
	}
	return a, nil, false
}

// handleChatrooms processes chatroom messages.
func (a App) handleChatrooms(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case roomsLoadedMsg:
		a.chatrooms = a.chatrooms.SetRooms(msg.rooms)
		var cmd tea.Cmd
		a.chatrooms, cmd = a.chatrooms.OpenPendingRoom()
		return a, cmd, true
	case screens.OpenRoomMsg:
		// NotifID is empty when this came from a URL open (routeURL) rather
		// than a chat_mention notification — nothing to mark read there.
		var markReadCmd tea.Cmd
		if msg.NotifID != "" {
			// Optimistic mark-read already applied in NotificationsModel.Update; confirm with API.
			if a.polledUnreadCount > 0 {
				a.polledUnreadCount--
			}
			markReadCmd = a.markNotifReadCmd(msg.NotifID)
		}
		a.chatroomsReturn = a.active
		a.chatrooms = a.chatrooms.SetPendingRoomSlug(msg.RoomSlug)
		// activateScreen resets canGoBack for ordinary tab/leader entry into
		// Chatrooms, so it must be set true *after* that call, not before.
		a, activateCmd := activateScreen(a, screenChatrooms)
		a.chatrooms = a.chatrooms.SetCanGoBack(true)
		return a, tea.Batch(markReadCmd, activateCmd), true
	case screens.SendRoomMessageMsg:
		return a, a.sendRoomMessageCmd(msg.RoomID, msg.Body), true
	case screens.MuteUserMsg:
		return a, a.muteUserCmd(msg.RoomID, msg.Username), true
	case userMutedMsg:
		// Muting is enforced client-side off Settings.MutedUsersByRoom (see
		// ChatroomsModel's SharedConfigMsg handler), which /mute updates
		// server-side but never pushes to us — same reload roomCommandReplyMsg
		// already does after a command completes. Without it the room keeps
		// showing the just-muted user's messages until something else happens
		// to reload settings.
		a, notifyCmd := a.notify(notifyInfo, "muted "+msg.username)
		return a, tea.Batch(notifyCmd, a.loadSettingsCmd()), true
	case screens.FlagMessageMsg:
		return a, a.flagRoomMessageCmd(msg.RoomID, msg.MessageID, msg.Reason), true
	case screens.DeleteRoomMessageMsg:
		return a, a.deleteRoomMessageCmd(msg.RoomID, msg.MessageID), true
	case roomMessageDeletedMsg:
		a.chatrooms = a.chatrooms.ApplyMessageDeleted(msg.messageID)
		return a, nil, true
	case screens.RoomOpenedMsg:
		return a, a.markRoomReadCmd(msg.RoomID), true
	case screens.RoomReconnectedMsg:
		a, cmd := a.notify(notifyInfo, "reconnected to live chat")
		return a, cmd, true
	case roomCommandReplyMsg:
		a.chatrooms = a.chatrooms.AppendSystemMessage(msg.roomID, sanitize.Strip(msg.reply))
		return a, a.loadSettingsCmd(), true
	case screens.ShowUserProfileMsg:
		if a.active != screenChatrooms {
			return a, nil, false
		}
		a.profileReturn = screenChatrooms
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.LeaveChatroomsMsg:
		a.active = a.chatroomsReturn
		return a, nil, true
	default:
		// Keep the open room's RTDB subscription (and its reconnect/heartbeat
		// chains) alive while another tab is active — see SetFocused and
		// IsRoomStreamMsg. When Chatrooms *is* active, delegateUpdate already
		// routes these the normal way, so this only fires while backgrounded.
		if a.active != screenChatrooms && screens.IsRoomStreamMsg(msg) {
			var cmd tea.Cmd
			a.chatrooms, cmd = a.chatrooms.Update(msg)
			return a, cmd, true
		}
	}
	return a, nil, false
}

// handleCMail processes C-Mail messages. DM subscription lifecycle is managed
// entirely within CMailModel; only the conversation list load and message send
// are coordinated here.
func (a App) handleCMail(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SendCMailMsg:
		return a, a.sendCMailCmd(msg.ConversationID, msg.Body), true
	case screens.CMailConvSelectedMsg:
		cmds := []tea.Cmd{a.markCMailReadCmd(msg.ConversationID)}
		if msg.OtherUsername != "" {
			cmds = append(cmds, a.loadCMailOtherProfileCmd(msg.OtherUsername))
		}
		return a, tea.Batch(cmds...), true
	case cmailOtherProfileLoadedMsg:
		a.cmail = a.cmail.SetOtherProfile(msg.username, msg.user)
		return a, nil, true
	case screens.StartConversationMsg:
		if msg.Username == "" || msg.Username == a.currentUser.Username {
			return a, nil, true
		}
		a.cmailReturn = a.active
		a.cmail = a.cmail.SetCanGoBack(true)
		return a, a.startConversationCmd(msg.Username), true
	case conversationStartedMsg:
		a.active = screenCMail
		a.cmail = a.cmail.SetActiveConversation(msg.conv)
		convID := msg.conv.ID
		fetchOther := a.cmail.OtherProfileFetchTarget(msg.conv)
		// The new conversation appears in the list via the live
		// user_conversations subscription once the server's write reaches
		// it — no REST refetch here (would race the subscription's own
		// state; see OpenUserConvsSubscription's doc comment).
		return a, tea.Batch(
			a.cmail.ConvOpenCmds(convID),
			func() tea.Msg {
				return screens.CMailConvSelectedMsg{ConversationID: convID, OtherUsername: fetchOther}
			},
		), true
	case screens.CMailReconnectedMsg:
		a, cmd := a.notify(notifyInfo, "reconnected to live chat")
		return a, cmd, true
	case cmailCommandReplyMsg:
		a.cmail = a.cmail.AppendSystemMessage(msg.convID, sanitize.Strip(msg.reply))
		return a, nil, true
	case screens.ShowUserProfileMsg:
		if a.active != screenCMail {
			return a, nil, false
		}
		a.profileReturn = screenCMail
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.LeaveCMailMsg:
		a.active = a.cmailReturn
		return a, nil, true
	default:
		// Keep the open conversation's RTDB subscription (and its typing/
		// reconnect chains) alive while another tab is active — see
		// SetFocused and IsDMStreamMsg. When C-Mail *is* active,
		// delegateUpdate already routes these the normal way, so this only
		// fires while backgrounded.
		if a.active != screenCMail && screens.IsDMStreamMsg(msg) {
			var cmd tea.Cmd
			a.cmail, cmd = a.cmail.Update(msg)
			return a, cmd, true
		}
	}
	return a, nil, false
}

// handleRelativeTimeTick keeps cIRC/C-Mail's "Nm ago"-style timestamps
// current by re-rendering whichever of the two is active — see
// screens.ChatroomsModel.RefreshRelativeTimestamps for why this is a no-op
// everywhere else (wrong screen active, or not in "relative" display mode).
// Always reschedules, mirroring schedulePollCmd's shape.
func (a App) handleRelativeTimeTick(msg tea.Msg) (App, tea.Cmd, bool) {
	t, ok := msg.(relativeTimeTickMsg)
	if !ok {
		return a, nil, false
	}
	if t.gen != a.sessionGen {
		return a, nil, true
	}
	switch a.active {
	case screenChatrooms:
		a.chatrooms = a.chatrooms.RefreshRelativeTimestamps()
	case screenCMail:
		a.cmail = a.cmail.RefreshRelativeTimestamps()
	}
	return a, a.scheduleRelativeTimeTickCmd(), true
}

// handleProfile processes profile load, save, and sub-tab messages.
func (a App) handleProfile(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case profileLoadedMsg:
		a.currentUser = msg.user
		a.profile = a.profile.ClearTabs().SetUser(msg.user).SetCanGoBack(false)
		// Propagate the confirmed username to screens that guard own-content actions.
		a.feed = a.feed.SetCurrentUsername(msg.user.Username).SetCurrentUserIsSupporter(msg.user.IsSupporter)
		a.postDetail = a.postDetail.SetCurrentUsername(msg.user.Username).SetCurrentUserIsSupporter(msg.user.IsSupporter)
		return a, a.loadUserGuildsCmd(msg.user.Username), true
	case userProfileLoadedMsg:
		isOwn := msg.user.Username == a.currentUser.Username
		// Clear stale sub-tab data whenever a different profile is loaded.
		a.profile = a.profile.ClearTabs().SetUser(msg.user).SetReadOnly(!isOwn).SetCanGoBack(true).SetFollowState(msg.isFollowing, msg.followID)
		a.active = screenProfile
		return a, a.loadUserGuildsCmd(msg.user.Username), true

	case userGuildsLoadedMsg:
		if msg.username == a.currentUser.Username {
			a.ownApprenticeSlugs = apprenticeSlugsFrom(msg.guilds)
			a.guilds = a.guilds.SetOwnApprenticeSlugs(a.ownApprenticeSlugs)
		}
		if msg.username != a.profile.Username() {
			return a, nil, true // stale response from a since-abandoned profile switch
		}
		a.profile = a.profile.SetApprenticeships(msg.guilds)
		return a, nil, true
	case screens.BackFromProfileMsg:
		a.active = a.profileReturn
		a.profile = a.profile.SetReadOnly(false).SetCanGoBack(false).SetFollowState(false, "")
		return a, nil, true
	case screens.SaveProfileMsg:
		return a, a.saveProfileCmd(msg), true
	case screens.FollowUserMsg:
		return a, a.followUserCmd(msg.UserID), true
	case screens.UnfollowUserMsg:
		return a, a.unfollowUserCmd(msg.FollowID), true
	case screens.PokeUserMsg:
		return a, a.pokeUserCmd(msg.Username), true
	case followResultMsg:
		a.profile = a.profile.SetFollowState(true, msg.followID).IncrementFollowersCount(1).SetFollowFeedback("following.")
		return a, nil, true
	case unfollowResultMsg:
		a.profile = a.profile.SetFollowState(false, "").IncrementFollowersCount(-1).SetFollowFeedback("unfollowed.")
		return a, nil, true

	// --- sub-tab lazy-load triggers ---
	case screens.ShowUserPostsMsg:
		return a, a.loadUserPostsCmd(msg.Username, ""), true
	case screens.ShowUserRepliesMsg:
		return a, a.loadUserRepliesCmd(msg.Username, ""), true
	case screens.ShowUserFollowingMsg:
		return a, a.loadUserFollowingCmd(msg.UserID, ""), true
	case screens.ShowUserFollowersMsg:
		return a, a.loadUserFollowersCmd(msg.UserID, ""), true

	// --- sub-tab pagination ---
	case screens.LoadMoreUserPostsMsg:
		return a, a.loadUserPostsCmd(msg.Username, msg.Cursor), true
	case screens.LoadMoreUserRepliesMsg:
		return a, a.loadUserRepliesCmd(msg.Username, msg.Cursor), true
	case screens.LoadMoreUserFollowingMsg:
		return a, a.loadUserFollowingCmd(msg.UserID, msg.Cursor), true
	case screens.LoadMoreUserFollowersMsg:
		return a, a.loadUserFollowersCmd(msg.UserID, msg.Cursor), true

	// --- sub-tab data results ---
	case userPostsLoadedMsg:
		a.profile = a.profile.SetUserPosts(msg.posts, msg.cursor)
		return a, nil, true
	case userPostsPageMsg:
		a.profile = a.profile.AppendUserPosts(msg.posts, msg.cursor)
		return a, nil, true
	case userRepliesLoadedMsg:
		a.profile = a.profile.SetUserReplies(msg.replies, msg.cursor)
		return a, nil, true
	case userRepliesPageMsg:
		a.profile = a.profile.AppendUserReplies(msg.replies, msg.cursor)
		return a, nil, true
	case userFollowingLoadedMsg:
		a.profile = a.profile.SetUserFollowing(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowingPageMsg:
		a.profile = a.profile.AppendUserFollowing(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowersLoadedMsg:
		a.profile = a.profile.SetUserFollowers(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowersPageMsg:
		a.profile = a.profile.AppendUserFollowers(msg.follows, msg.cursor)
		return a, nil, true

	// --- navigation from sub-tabs ---
	case screens.ShowProfilePostMsg:
		// Navigate to post detail; return to profile when ESC is pressed.
		a.postDetailReturn = screenProfile
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	case screens.ShowProfileReplyMsg:
		// Navigate to a post thread from the Replies tab; fetch the full post and scroll to the reply.
		a.postDetailReturn = screenProfile
		a.active = screenPostDetail
		a.pendingReplyID = msg.ReplyID
		a.postDetail = a.postDetail.SetPost(model.Post{ID: msg.PostID})
		return a, tea.Batch(a.loadProfilePostCmd(msg.PostID), a.loadRepliesCmd(msg.PostID)), true
	case profilePostLoadedMsg:
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, nil, true
	case screens.ShowUserProfileMsg:
		// Only intercept when the profile screen is active (e.g. from Following/Followers tab).
		if a.active != screenProfile {
			return a, nil, false
		}
		// Navigate to the new user's profile; returning will go to the current profileReturn.
		return a, a.loadUserProfileCmd(msg.Username), true
	}
	return a, nil, false
}

func (a App) handleSettings(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case settingsLoadedMsg:
		a.settings = msg.settings
		a.settingsScreen = a.settingsScreen.SetSettings(msg.settings)
		a.broadcastConfig()
		return a, nil, true

	case screens.SaveSettingsMsg:
		s := msg.Settings
		wl := msg.WanderLust
		fmro := msg.FeedManualRefreshOnly
		tie := msg.TypingIndicatorsEnabled
		td := msg.MaxThreadDepth
		tz := msg.Timezone
		iv := msg.ImageViewer
		gp := msg.GraphicsProtocol
		ii := msg.InlineImages
		dt := msg.Dithering
		ds := msg.DitherSharpness
		ln := msg.LayoutName
		a.settingsSaveSeq++
		seq := a.settingsSaveSeq
		return a, func() tea.Msg {
			if msg.RemoteChanged {
				if err := a.client.UpdateSettings(s); err != nil {
					return actionErrMsg{err}
				}
			}
			return settingsSavedMsg{seq: seq, settings: s, wanderLust: wl, feedManualRefreshOnly: fmro, typingIndicatorsEnabled: tie, maxThreadDepth: td, timezone: tz, imageViewer: iv, graphicsProtocol: gp, inlineImages: ii, dithering: dt, ditherSharpness: ds, layoutName: ln}
		}, true

	case settingsSavedMsg:
		// A second ctrl+s can be dispatched (and land) before an earlier
		// save's UpdateSettings network round-trip finishes. If that earlier
		// save were still allowed to persist once it does complete, it would
		// silently overwrite the newer values — in memory and in
		// ~/.cyber-tui.json — with its own stale snapshot. Only the
		// most-recently-dispatched save (the current seq) is applied; any
		// other completion is dropped.
		if msg.seq != a.settingsSaveSeq {
			return a, nil, true
		}
		a.settings = msg.settings
		a.wanderLust = msg.wanderLust
		// wasManual detects a manual→auto transition so the (possibly dead)
		// feed-poll tea.Tick chain gets restarted immediately below, rather
		// than staying off until the next app restart — see
		// docs/39-feed-background-poll.md.
		wasManual := a.feedManualRefreshOnly
		a.feedManualRefreshOnly = msg.feedManualRefreshOnly
		// No "restart a dead chain" logic needed here, unlike the feed poll
		// above — CMailModel reacts live to every SharedConfigMsg broadcast
		// (broadcastConfig below) to start/stop its own typing-indicator
		// subsystem, see cmail.go's SharedConfigMsg handler.
		a.typingIndicatorsEnabled = msg.typingIndicatorsEnabled
		a.maxThreadDepth = msg.maxThreadDepth
		a.timezone = msg.timezone
		a.imageViewer = msg.imageViewer
		if msg.graphicsProtocol != a.graphicsProtocolName {
			// Only re-resolve when the override actually changed — saving an
			// unrelated setting (dithering, wander mode, etc.) must not touch
			// this. "" (auto) can only be re-resolved via env-var detection
			// here: the Sixel DA1 probe (imgview.ProbeSixel) needs raw
			// terminal access before Bubble Tea takes over stdin, so it can't
			// safely re-run mid-session — see its doc comment. Without this
			// guard, any settings save on a DA1-probed Sixel terminal would
			// silently downgrade a.graphicsProtocol to whatever env-var
			// detection alone finds (often ProtocolNone), breaking inline
			// images and the image viewer until a full restart re-probes
			// Sixel.
			if proto, ok := imgview.ProtocolFromName(msg.graphicsProtocol); ok {
				a.graphicsProtocol = proto
			} else {
				a.graphicsProtocol = imgview.DetectProtocol()
			}
		}
		a.graphicsProtocolName = msg.graphicsProtocol
		a.inlineImages = msg.inlineImages
		a.dithering = msg.dithering
		a.ditherSharpness = msg.ditherSharpness
		a.layoutName = msg.layoutName
		a.layout = layoutFromName(msg.layoutName)
		a.focus = focusMenu
		a.loc = config.ParseTimezoneLabel(msg.timezone)
		a.settingsScreen = a.settingsScreen.SetSaved(msg.wanderLust, msg.feedManualRefreshOnly, msg.typingIndicatorsEnabled, msg.maxThreadDepth, msg.timezone, msg.imageViewer, msg.graphicsProtocol, msg.inlineImages, msg.dithering, msg.ditherSharpness, msg.layoutName)
		a.broadcastConfig()
		a.refreshViewports()
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "settings saved")
		wl, fmro, tie, td, tz, iv, gp, ii, dt, ds, ln := msg.wanderLust, msg.feedManualRefreshOnly, msg.typingIndicatorsEnabled, msg.maxThreadDepth, msg.timezone, msg.imageViewer, msg.graphicsProtocol, msg.inlineImages, msg.dithering, msg.ditherSharpness, msg.layoutName
		saveCmd := func() tea.Msg {
			a.saveConfig(func(cfg *config.Config) {
				cfg.WanderLust = wl
				cfg.FeedManualRefreshOnly = fmro
				cfg.TypingIndicatorsDisabled = !tie
				cfg.MaxThreadDepth = td
				cfg.Timezone = tz
				cfg.ImageViewer = iv
				cfg.GraphicsProtocol = gp
				cfg.InlineImages = ii
				cfg.Dithering = dt
				cfg.DitherSharpness = ds
				cfg.Layout = ln
			})
			return nil
		}
		cmds := []tea.Cmd{notifyCmd, saveCmd}
		if wasManual && !a.feedManualRefreshOnly {
			cmds = append(cmds, a.scheduleFeedPollCmd())
		}
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 {
			if cursor := a.feed.NextCursor(); cursor != "" && a.feed.PostCount() < min {
				cmds = append(cmds, a.loadFeedPageCmd(cursor))
			}
			if a.guilds.IsViewingGuildPosts() {
				if cursor := a.guilds.PostsNextCursor(); cursor != "" && a.guilds.PostCount() < min {
					cmds = append(cmds, a.loadGuildPostsPageCmd(a.guilds.ActiveGuild(), cursor))
				}
			}
			if a.topics.IsViewingTopicPosts() {
				if cursor := a.topics.PostsNextCursor(); cursor != "" && a.topics.PostCount() < min {
					cmds = append(cmds, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), cursor))
				}
			}
		}
		return a, tea.Batch(cmds...), true

	case wanderTickMsg:
		if msg.gen != a.sessionGen {
			return a, nil, true
		}
		return a, tea.Batch(a.checkAndWanderCmd(), a.scheduleWanderCmd()), true

	case wanderDoneMsg:
		if !msg.at.IsZero() {
			a.saveConfig(func(cfg *config.Config) {
				cfg.LastWandered = msg.at
			})
		}
		return a, nil, true

	case updateAvailableMsg:
		var cmd tea.Cmd
		// threatcrush-disable-next-line sql-format-call  UI notification string, no SQL anywhere in this codebase
		a, cmd = a.notify(notifyWarn, fmt.Sprintf("update available: %s (you have %s) — %s", msg.tag, version.Version, msg.url))
		return a, cmd, true
	}
	return a, nil, false
}

// handleThemeEditor processes messages emitted by the theme editor modal:
// live preview on every edit, persisting on save, and reverting on close.
func (a App) handleThemeEditor(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.PreviewPaletteMsg:
		theme.SetCustomPalette(msg.Palette)
		a.refreshViewports()
		return a, nil, true

	case screens.PreviewPostThemeMsg:
		// Same contract as the picker's 'e' key, but prefilled from a
		// detected post theme instead of the current theme/saved custom
		// palette: snapshot for correct revert, preview live, hand off to
		// the theme editor so the user can review/tweak before ctrl+s.
		a.themeEditorOrig = theme.CurrentName()
		a.themeEditorOrigPalette = theme.CurrentPalette()
		theme.SetCustomPalette(msg.Palette)
		theme.Set("custom")
		a.refreshViewports()
		a.themeEditorOpen = true
		a.themeEditor = screens.NewThemeEditorModel(msg.Palette)
		return a, nil, true

	case screens.SaveThemeMsg:
		p := msg.Palette
		a.themeEditor = a.themeEditor.SetSaved(p)
		a.customPalette = &p
		a.themeEditorOpen = false
		return a, func() tea.Msg {
			a.saveConfig(func(cfg *config.Config) {
				cfg.Theme = "custom"
				pp := p
				cfg.CustomPalette = &pp
			})
			return nil
		}, true

	case screens.CloseThemeEditorMsg:
		a.themeEditorOpen = false
		if a.themeEditorOrig == "custom" {
			// Restore the snapshot taken before preview started — the
			// abandoned edit has left theme's package-level customPalette
			// dirty, so Set("custom") alone would reapply that instead of
			// what was actually active before the editor opened.
			theme.SetCustomPalette(a.themeEditorOrigPalette)
		}
		theme.Set(a.themeEditorOrig)
		a.refreshViewports()
		return a, nil, true
	}
	return a, nil, false
}

// handlePathPrompt processes messages emitted by the export/import path
// prompt. Export requires a second identical submit once a target file is
// found to already exist (pathPromptOverwritePending); import validates the
// file via theme.ImportFromFile and, on success, hands off to the theme
// editor exactly like PreviewPostThemeMsg — reviewed and confirmed with
// ctrl+s, never applied blind.
func (a App) handlePathPrompt(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.PathPromptSubmitMsg:
		switch a.pathPromptPurpose {
		case pathPromptExport:
			resolved, _ := theme.ExpandHome(msg.Path)
			if _, err := os.Stat(resolved); err == nil && msg.Path != a.pathPromptOverwritePending {
				a.pathPromptOverwritePending = msg.Path
				a.pathPrompt = a.pathPrompt.SetWarning("file exists — enter again to overwrite")
				return a, nil, true
			}
			a.pathPromptOpen = false
			if err := theme.ExportToFile(msg.Path, a.pathPromptExportPalette); err != nil {
				a2, cmd := a.notify(notifyError, "export failed: "+err.Error())
				return a2, cmd, true
			}
			a2, cmd := a.notify(notifyInfo, "theme exported to "+msg.Path)
			return a2, cmd, true

		case pathPromptImport:
			a.pathPromptOpen = false
			p, err := theme.ImportFromFile(msg.Path)
			if err != nil {
				a2, cmd := a.notify(notifyError, "import failed: "+err.Error())
				return a2, cmd, true
			}
			a.themeEditorOrig = theme.CurrentName()
			a.themeEditorOrigPalette = theme.CurrentPalette()
			theme.SetCustomPalette(p)
			theme.Set("custom")
			a.refreshViewports()
			a.themeEditorOpen = true
			a.themeEditor = screens.NewThemeEditorModel(p)
			return a, nil, true
		}
		return a, nil, true

	case screens.PathPromptCancelMsg:
		a.pathPromptOpen = false
		a.pathPromptOverwritePending = ""
		// The picker's own live preview may have changed the active theme
		// while browsing rows before 'x'/'i' was pressed — restore whatever
		// was active before the picker was ever opened, same as the
		// picker's own esc.
		theme.Set(a.themePickerOrig)
		a.refreshViewports()
		return a, nil, true
	}
	return a, nil, false
}

// handleBookmarks processes bookmark load, create, and delete messages.
func (a App) handleBookmarks(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case bookmarksLoadedMsg:
		a.bookmarks = a.bookmarks.SetBookmarks(msg.items, msg.cursor)
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs = bookmarkIDSets(msg.items)
		a.broadcastBookmarkedIDs()
		return a, nil, true
	case bookmarksPageMsg:
		a.bookmarks = a.bookmarks.AppendBookmarks(msg.items, msg.cursor)
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs = mergeBookmarkIDSets(
			a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs, msg.items)
		a.broadcastBookmarkedIDs()
		return a, nil, true
	case screens.LoadMoreBookmarksMsg:
		return a, a.loadBookmarksPageCmd(msg.Cursor), true
	case screens.OpenBookmarkMsg:
		if msg.PostID != "" {
			return a, a.loadBookmarkPostCmd(msg.PostID), true
		}
		if msg.ReplyID != "" {
			return a, a.loadBookmarkReplyCmd(msg.ReplyID), true
		}
		return a, nil, true
	case bookmarkPostLoadedMsg:
		a.postDetailReturn = screenBookmarks
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, a.loadRepliesCmd(msg.post.ID), true
	case bookmarkReplyLoadedMsg:
		a.postDetailReturn = screenBookmarks
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		a.pendingReplyID = msg.replyID
		return a, a.loadRepliesCmd(msg.post.ID), true
	case screens.CopyLinkMsg:
		// termenv.Copy writes an OSC 52 escape sequence to the host process's
		// os.Stdout — for an SSH/wish session that's the host machine's own
		// terminal, not the connected client's, so it can't reach the SSH
		// user's clipboard at all (same reason inline images are gated on
		// a.ephemeral). Gate it here rather than silently claim success.
		if a.ephemeral {
			a, cmd := a.notify(notifyInfo, "Copying links is disabled in SSH sessions")
			return a, cmd, true
		}
		termenv.Copy(urlutil.PostPermalink(msg.Post.AuthorUsername, msg.Post.Slug))
		a, cmd := a.notify(notifyInfo, "Copied link to clipboard")
		return a, cmd, true
	case screens.CopyMessageTextMsg:
		// Same SSH gate as CopyLinkMsg — see its comment.
		if a.ephemeral {
			a, cmd := a.notify(notifyInfo, "Copying is disabled in SSH sessions")
			return a, cmd, true
		}
		if msg.Text == "" {
			a, cmd := a.notify(notifyInfo, "nothing to copy")
			return a, cmd, true
		}
		termenv.Copy(msg.Text)
		a, cmd := a.notify(notifyInfo, "Copied message to clipboard")
		return a, cmd, true
	case screens.BookmarkPostMsg:
		if msg.ReplyID != "" {
			replyID := msg.ReplyID
			if _, alreadyBookmarked := a.bookmarkedReplyIDs[replyID]; alreadyBookmarked {
				// Toggle off: optimistic remove.
				bookmarkID := a.replyBookmarkIDs[replyID]
				newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
				for k := range a.bookmarkedReplyIDs {
					if k != replyID {
						newReplyIDs[k] = struct{}{}
					}
				}
				a.bookmarkedReplyIDs = newReplyIDs
				delete(a.replyBookmarkIDs, replyID)
				a.broadcastBookmarkedIDs()
				return a, a.deleteBookmarkCmd(bookmarkID, false), true
			}
			// Toggle on: optimistic add.
			newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs)+1)
			for k := range a.bookmarkedReplyIDs {
				newReplyIDs[k] = struct{}{}
			}
			newReplyIDs[replyID] = struct{}{}
			a.bookmarkedReplyIDs = newReplyIDs
			a.broadcastBookmarkedIDs()
			return a, a.createBookmarkCmd("", replyID), true
		}
		postID := msg.PostID
		if _, alreadyBookmarked := a.bookmarkedPostIDs[postID]; alreadyBookmarked {
			// Toggle off: optimistic remove.
			bookmarkID := a.postBookmarkIDs[postID]
			newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
			for k := range a.bookmarkedPostIDs {
				if k != postID {
					newPostIDs[k] = struct{}{}
				}
			}
			a.bookmarkedPostIDs = newPostIDs
			delete(a.postBookmarkIDs, postID)
			a.broadcastBookmarkedIDs()
			return a, a.deleteBookmarkCmd(bookmarkID, false), true
		}
		// Toggle on: optimistic add.
		newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs)+1)
		for k := range a.bookmarkedPostIDs {
			newPostIDs[k] = struct{}{}
		}
		newPostIDs[postID] = struct{}{}
		a.bookmarkedPostIDs = newPostIDs
		a.broadcastBookmarkedIDs()
		return a, a.createBookmarkCmd(postID, ""), true
	case bookmarkCreatedMsg:
		if msg.err != nil {
			// Roll back the optimistic add.
			if msg.replyID != "" {
				newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
				for k := range a.bookmarkedReplyIDs {
					if k != msg.replyID {
						newReplyIDs[k] = struct{}{}
					}
				}
				a.bookmarkedReplyIDs = newReplyIDs
			} else {
				newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
				for k := range a.bookmarkedPostIDs {
					if k != msg.postID {
						newPostIDs[k] = struct{}{}
					}
				}
				a.bookmarkedPostIDs = newPostIDs
			}
			a.broadcastBookmarkedIDs()
			a, cmd := a.notify(notifyError, msg.err.Error())
			return a, cmd, true
		}
		a.bookmarks = a.bookmarks.SetFetching()
		return a, a.loadBookmarksCmd(""), true
	case screens.DeleteBookmarkMsg:
		// Optimistic update already applied in BookmarksModel.Update; remove from sets.
		if msg.PostID != "" {
			newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
			for k := range a.bookmarkedPostIDs {
				if k != msg.PostID {
					newPostIDs[k] = struct{}{}
				}
			}
			a.bookmarkedPostIDs = newPostIDs
			delete(a.postBookmarkIDs, msg.PostID)
		}
		if msg.ReplyID != "" {
			newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
			for k := range a.bookmarkedReplyIDs {
				if k != msg.ReplyID {
					newReplyIDs[k] = struct{}{}
				}
			}
			a.bookmarkedReplyIDs = newReplyIDs
			delete(a.replyBookmarkIDs, msg.ReplyID)
		}
		a.broadcastBookmarkedIDs()
		return a, a.deleteBookmarkCmd(msg.BookmarkID, true), true
	case bookmarkDeletedMsg:
		if !msg.fromBookmarksScreen {
			a.bookmarks = a.bookmarks.SetFetching()
			return a, a.loadBookmarksCmd(""), true
		}
		return a, nil, true
	}
	return a, nil, false
}

// --- Watch messages ---

type watchPageMsg struct {
	postIDs []string
	cursor  string
	err     error
}

type watchResultMsg struct {
	postID string
	err    error
	added  bool // true = watch was added, false = watch was removed
}

// handleWatches processes progressive watch-page loads and watch/unwatch toggle messages.
func (a App) handleWatches(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case watchPageMsg:
		if msg.err != nil {
			// Watches are non-critical; silently ignore load errors.
			return a, nil, true
		}
		newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+len(msg.postIDs))
		for k := range a.watchedPostIDs {
			newIDs[k] = struct{}{}
		}
		for _, id := range msg.postIDs {
			newIDs[id] = struct{}{}
		}
		a.watchedPostIDs = newIDs
		a.broadcastWatchedIDs()
		if msg.cursor != "" {
			return a, a.loadWatchesPageCmd(msg.cursor), true
		}
		return a, nil, true

	case screens.ToggleWatchPostMsg:
		postID := msg.PostID
		if _, alreadyWatched := a.watchedPostIDs[postID]; alreadyWatched {
			// Toggle off: optimistic remove.
			newIDs := make(map[string]struct{}, len(a.watchedPostIDs))
			for k := range a.watchedPostIDs {
				newIDs[k] = struct{}{}
			}
			delete(newIDs, postID)
			a.watchedPostIDs = newIDs
			a.broadcastWatchedIDs()
			return a, a.unwatchPostCmd(postID), true
		}
		// Toggle on: optimistic add.
		newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+1)
		for k := range a.watchedPostIDs {
			newIDs[k] = struct{}{}
		}
		newIDs[postID] = struct{}{}
		a.watchedPostIDs = newIDs
		a.broadcastWatchedIDs()
		return a, a.watchPostCmd(postID), true

	case watchResultMsg:
		if msg.err != nil {
			// Revert the optimistic update.
			newIDs := make(map[string]struct{}, len(a.watchedPostIDs))
			for k := range a.watchedPostIDs {
				newIDs[k] = struct{}{}
			}
			if msg.added {
				delete(newIDs, msg.postID)
			} else {
				newIDs[msg.postID] = struct{}{}
			}
			a.watchedPostIDs = newIDs
			a.broadcastWatchedIDs()
			a2, cmd := a.notify(notifyError, msg.err.Error())
			return a2, cmd, true
		}
		return a, nil, true
	}
	return a, nil, false
}

// bookmarkIDSets builds post/reply ID sets and reverse lookup maps from a fresh bookmark page.
func bookmarkIDSets(items []model.Bookmark) (map[string]struct{}, map[string]struct{}, map[string]string, map[string]string) {
	postIDs := make(map[string]struct{})
	replyIDs := make(map[string]struct{})
	postBookmarks := make(map[string]string)
	replyBookmarks := make(map[string]string)
	for _, b := range items {
		if b.PostID != "" {
			postIDs[b.PostID] = struct{}{}
			postBookmarks[b.PostID] = b.ID
		}
		if b.ReplyID != "" {
			replyIDs[b.ReplyID] = struct{}{}
			replyBookmarks[b.ReplyID] = b.ID
		}
	}
	return postIDs, replyIDs, postBookmarks, replyBookmarks
}

// mergeBookmarkIDSets merges a new page of bookmarks into existing ID sets and reverse maps.
func mergeBookmarkIDSets(postIDs, replyIDs map[string]struct{}, postBookmarks, replyBookmarks map[string]string, items []model.Bookmark) (map[string]struct{}, map[string]struct{}, map[string]string, map[string]string) {
	newPostIDs := make(map[string]struct{}, len(postIDs)+len(items))
	for k := range postIDs {
		newPostIDs[k] = struct{}{}
	}
	newReplyIDs := make(map[string]struct{}, len(replyIDs)+len(items))
	for k := range replyIDs {
		newReplyIDs[k] = struct{}{}
	}
	newPostBookmarks := make(map[string]string, len(postBookmarks)+len(items))
	for k, v := range postBookmarks {
		newPostBookmarks[k] = v
	}
	newReplyBookmarks := make(map[string]string, len(replyBookmarks)+len(items))
	for k, v := range replyBookmarks {
		newReplyBookmarks[k] = v
	}
	for _, b := range items {
		if b.PostID != "" {
			newPostIDs[b.PostID] = struct{}{}
			newPostBookmarks[b.PostID] = b.ID
		}
		if b.ReplyID != "" {
			newReplyIDs[b.ReplyID] = struct{}{}
			newReplyBookmarks[b.ReplyID] = b.ID
		}
	}
	return newPostIDs, newReplyIDs, newPostBookmarks, newReplyBookmarks
}

// handleGuilds processes guild list, guild posts, pagination, and post selection messages.
func (a App) handleGuilds(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.RefreshGuildsMsg:
		return a, a.loadGuildsCmd(""), true

	case guildsLoadedMsg:
		a.guilds = a.guilds.SetGuilds(msg.guilds, msg.cursor)
		var missing []string
		if slug := a.currentUser.GuildSlug; slug != "" && !a.guilds.HasGuild(slug) {
			missing = append(missing, slug)
		}
		for _, slug := range a.ownApprenticeSlugs {
			if slug != "" && !a.guilds.HasGuild(slug) {
				missing = append(missing, slug)
			}
		}
		if len(missing) == 0 {
			return a, nil, true
		}
		cmds := make([]tea.Cmd, len(missing))
		for i, slug := range missing {
			cmds[i] = a.loadOwnGuildIntoListCmd(slug)
		}
		return a, tea.Batch(cmds...), true

	case ownGuildInjectMsg:
		a.guilds = a.guilds.InjectGuild(msg.guild)
		return a, nil, true

	case screens.LoadMoreGuildsMsg:
		return a, a.loadMoreGuildsCmd(msg.Cursor), true

	case guildsPageMsg:
		a.guilds = a.guilds.AppendGuilds(msg.guilds, msg.cursor)
		return a, nil, true

	case screens.LoadGuildPostsMsg:
		return a, tea.Batch(a.loadGuildPostsCmd(msg.Slug), a.loadGuildDetailCmd(msg.Slug)), true

	case guildPostsLoadedMsg:
		if msg.slug != a.guilds.ActiveGuild() {
			return a, nil, true
		}
		a.guilds = a.guilds.SetGuildPosts(msg.posts, msg.cursor)
		var detailCmd tea.Cmd
		a.guilds, detailCmd = a.guilds.CurrentDetailCmd()
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.guilds.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadGuildPostsPageCmd(msg.slug, msg.cursor)), true
		}
		if detailCmd != nil {
			return a, detailCmd, true
		}
		return a, nil, true

	case screens.LoadGuildThreadMsg:
		return a, a.loadGuildThreadCmd(msg.PostID), true

	case screens.GuildThreadRepliesMsg:
		a.guilds, _ = a.guilds.Update(msg)
		return a, nil, true

	case screens.GuildThreadNavMsg:
		a.guilds, _ = a.guilds.Update(msg)
		return a, nil, true

	case screens.LoadMoreGuildPostsMsg:
		return a, a.loadGuildPostsPageCmd(msg.Slug, msg.Cursor), true

	case guildPostsPageMsg:
		if msg.slug != a.guilds.ActiveGuild() {
			return a, nil, true
		}
		a.guilds = a.guilds.AppendGuildPosts(msg.posts, msg.cursor)
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.guilds.PostCount() < min {
			return a, a.loadGuildPostsPageCmd(msg.slug, msg.cursor), true
		}
		return a, nil, true

	case screens.RefreshGuildPostsMsg:
		return a, a.loadGuildPostsCmd(msg.Slug), true

	case screens.ShowGuildPostMsg:
		a.postDetailReturn = screenGuilds
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true

	case screens.SubmitGuildPostMsg:
		return a, a.createGuildPostCmd(msg.Slug, msg.Content, msg.Title, msg.PostSlug, msg.Topics), true

	case guildPostCreatedMsg:
		return a, a.loadGuildPostsCmd(msg.slug), true

	case screens.ShowUserProfileMsg:
		if a.active != screenGuilds {
			return a, nil, false
		}
		a.profileReturn = screenGuilds
		return a, a.loadUserProfileCmd(msg.Username), true

	case screens.LoadGuildMembersMsg:
		return a, a.loadGuildMembersCmd(msg.Slug, ""), true

	case guildMembersLoadedMsg:
		a.guilds = a.guilds.SetGuildMembers(msg.members, msg.cursor)
		return a, nil, true

	case screens.LoadMoreGuildMembersMsg:
		return a, a.loadGuildMembersCmd(msg.Slug, msg.Cursor), true

	case guildMembersPageMsg:
		a.guilds = a.guilds.AppendGuildMembers(msg.members, msg.cursor)
		return a, nil, true

	case guildDetailLoadedMsg:
		a.guilds = a.guilds.SetGuildDetail(msg.guild)
		return a, nil, true

	case screens.JoinGuildMsg:
		return a, a.joinGuildCmd(msg.Slug, a.guilds.GuildDetail().Name), true

	case screens.LeaveGuildMsg:
		return a, a.leaveGuildCmd(msg.Slug, a.guilds.GuildDetail().Name), true

	case screens.PromoteGuildMsg:
		return a, a.promoteGuildCmd(msg.Slug, a.guilds.GuildDetail().Name), true

	case guildJoinedMsg:
		detail := a.guilds.GuildDetail()
		detail.IsMember = true
		detail.Role = msg.role
		a.guilds = a.guilds.SetGuildDetail(detail)
		verb := "Apprenticed to"
		if msg.role == "member" {
			verb = "Joined"
			a.currentUser.GuildSlug = msg.slug
			a.currentUser.GuildID = detail.ID
			a.currentUser.GuildName = detail.Name
			a.currentUser.GuildIcon = detail.Icon
			a.guilds = a.guilds.SetOwnGuildSlug(msg.slug)
		}
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "✓ "+verb+" #"+msg.name)
		return a, tea.Batch(notifyCmd, a.loadGuildsCmd(""), a.loadUserGuildsCmd(a.currentUser.Username)), true

	case guildLeftMsg:
		a.guilds = a.guilds.BackToGuildList()
		if msg.slug == a.currentUser.GuildSlug {
			a.currentUser.GuildSlug = ""
			a.currentUser.GuildID = ""
			a.currentUser.GuildName = ""
			a.currentUser.GuildIcon = ""
			a.guilds = a.guilds.SetOwnGuildSlug("")
		}
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "✓ Left #"+msg.name)
		return a, tea.Batch(notifyCmd, a.loadGuildsCmd(""), a.loadUserGuildsCmd(a.currentUser.Username)), true

	case guildPromotedMsg:
		detail := a.guilds.GuildDetail()
		detail.Role = msg.role
		a.guilds = a.guilds.SetGuildDetail(detail)
		a.currentUser.GuildSlug = msg.slug
		a.currentUser.GuildID = detail.ID
		a.currentUser.GuildName = detail.Name
		a.currentUser.GuildIcon = detail.Icon
		a.guilds = a.guilds.SetOwnGuildSlug(msg.slug)
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "✓ #"+msg.name+" is now your guild badge")
		return a, tea.Batch(notifyCmd, a.loadGuildsCmd(""), a.loadUserGuildsCmd(a.currentUser.Username)), true
	}
	return a, nil, false
}

// handleTopics processes topic list, topic posts, pagination, and post selection messages.
func (a App) handleTopics(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.RefreshTopicsMsg:
		return a, a.loadTopicsCmd(), true

	case topicsLoadedMsg:
		a.topics = a.topics.SetTopics(msg.topics, msg.cursor)
		return a, nil, true

	case screens.LoadMoreTopicsMsg:
		return a, a.loadMoreTopicsCmd(msg.Cursor), true

	case topicsPageMsg:
		a.topics = a.topics.AppendTopics(msg.topics, msg.cursor)
		return a, nil, true

	case screens.LoadTopicPostsMsg:
		return a, a.loadTopicPostsCmd(msg.Slug), true

	case topicPostsLoadedMsg:
		a.topics = a.topics.SetTopicPosts(msg.posts, msg.cursor)
		var detailCmd tea.Cmd
		a.topics, detailCmd = a.topics.CurrentDetailCmd()
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.topics.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), msg.cursor)), true
		}
		if detailCmd != nil {
			return a, detailCmd, true
		}
		return a, nil, true

	case screens.LoadTopicThreadMsg:
		return a, a.loadTopicThreadCmd(msg.PostID), true

	case screens.TopicThreadRepliesMsg:
		a.topics, _ = a.topics.Update(msg)
		return a, nil, true

	case screens.TopicThreadNavMsg:
		a.topics, _ = a.topics.Update(msg)
		return a, nil, true

	case screens.LoadMoreTopicPostsMsg:
		return a, a.loadTopicPostsPageCmd(msg.Slug, msg.Cursor), true

	case topicPostsPageMsg:
		a.topics = a.topics.AppendTopicPosts(msg.posts, msg.cursor)
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.topics.PostCount() < min {
			return a, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), msg.cursor), true
		}
		return a, nil, true

	case screens.RefreshTopicPostsMsg:
		return a, a.loadTopicPostsCmd(msg.Slug), true

	case screens.ShowTopicPostMsg:
		a.postDetailReturn = screenTopics
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	}
	return a, nil, false
}

// handleJournal processes journal (Notes) load, save, delete, and publish messages.
func (a App) handleJournal(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case journalLoadedMsg:
		a.journal = a.journal.SetNotes(msg.notes, msg.cursor)
		return a, nil, true
	case journalPageMsg:
		a.journal = a.journal.AppendNotes(msg.notes, msg.cursor)
		return a, nil, true
	case screens.LoadMoreJournalMsg:
		return a, a.loadJournalPageCmd(msg.Cursor), true
	case screens.SubmitSaveNoteMsg:
		return a, a.saveNoteCmd(msg.NoteID, msg.Content, msg.Topics), true
	case noteCreatedMsg:
		a.journal = a.journal.PrependNote(msg.note)
		return a, nil, true
	case noteUpdatedMsg:
		a.journal = a.journal.UpdateNoteContent(msg.noteID, msg.content, msg.topics)
		return a, nil, true
	case screens.SubmitDeleteNoteMsg:
		return a, a.deleteNoteCmd(msg.NoteID), true
	case noteDeletedMsg:
		a.journal = a.journal.DeleteNote(msg.noteID)
		return a, nil, true
	case screens.SubmitPublishNoteMsg:
		return a, a.publishNoteCmd(msg.Content, msg.Topics), true
	case notePublishedMsg:
		return a, nil, true
	case screens.LoadNoteRevisionsMsg:
		return a, a.loadNoteRevisionsCmd(msg.NoteID, ""), true
	case screens.LoadNoteRevisionMsg:
		return a, a.loadNoteRevisionCmd(msg.NoteID, msg.RevisionNumber), true
	case noteRevisionsLoadedMsg:
		a.journal = a.journal.SetRevisions(msg.noteID, msg.revisions, msg.cursor)
		return a, nil, true
	case noteRevisionPreviewMsg:
		a.journal = a.journal.SetRevisionPreview(msg.note)
		return a, nil, true
	}
	return a, nil, false
}

// handleSearch processes search query, preview, drill-down, and pagination messages.
func (a App) handleSearch(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SubmitSearchMsg:
		return a, a.searchCmd(msg.Query), true

	case searchPreviewLoadedMsg:
		a.search = a.search.SetPreview(msg.preview, msg.query)
		return a, nil, true

	case screens.DrillSearchTypeMsg:
		return a, a.searchTypeCmd(msg.Type, a.search.LastQuery()), true

	case searchTypeLoadedMsg:
		a.search = a.search.SetTypeResults(msg.hitType, msg.posts, msg.replies, msg.users, msg.cursor)
		return a, nil, true

	case screens.LoadMoreSearchMsg:
		return a, a.searchTypePageCmd(msg.Type, a.search.LastQuery(), msg.Cursor), true

	case searchTypePageMsg:
		a.search = a.search.AppendTypeResults(msg.hitType, msg.posts, msg.replies, msg.users, msg.cursor)
		return a, nil, true

	case screens.ShowSearchPostMsg:
		a.postDetailReturn = screenSearch
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true

	case screens.ShowSearchReplyMsg:
		a.postDetailReturn = screenSearch
		a.active = screenPostDetail
		a.pendingReplyID = msg.ReplyID
		a.postDetail = a.postDetail.SetPost(model.Post{ID: msg.PostID})
		return a, tea.Batch(a.loadProfilePostCmd(msg.PostID), a.loadRepliesCmd(msg.PostID)), true

	case screens.ShowUserProfileMsg:
		if a.active != screenSearch {
			return a, nil, false
		}
		a.profileReturn = screenSearch
		return a, a.loadUserProfileCmd(msg.Username), true

	case screens.LeaveSearchMsg:
		a.active = a.searchReturn
		return a, nil, true
	}
	return a, nil, false
}

// handleErr routes API error messages to the active screen's error display.
// notifyTTL is how long a global notification banner stays before auto-dismissing.
const notifyTTL = 4 * time.Second

// notify sets the global banner and returns the timed-expire command. Each call
// bumps notifyGen and captures it in the tick closure, so only the newest
// notification's expire can clear the banner.
func (a App) notify(level notifyLevel, text string) (App, tea.Cmd) {
	a.notifyGen++
	a.notifyText = text
	a.notifyLevel = level
	gen := a.notifyGen
	return a, tea.Tick(notifyTTL, func(time.Time) tea.Msg {
		return notifyExpireMsg{gen: gen}
	})
}

func (a App) handleLogoAnim(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case logoAnimTickMsg:
		if m.gen != a.sessionGen {
			return a, nil, true
		}
		positions := make([]int, len(logoOrigRunes))
		for i := range positions {
			positions[i] = i
		}
		rand.Shuffle(len(positions), func(i, j int) { positions[i], positions[j] = positions[j], positions[i] })
		a.logoPositions = positions
		a.logoPhase = logoPhaseScrambling
		a.logoFrame = 0
		return a, logoFrameTickCmd(), true
	case logoFrameTickMsg:
		switch a.logoPhase {
		case logoPhaseScrambling:
			pos := a.logoPositions[a.logoFrame]
			runes := []rune(a.logoText)
			runes[pos] = randomCyberRune(logoOrigRunes[pos])
			a.logoText = string(runes)
			a.logoFrame++
			if a.logoFrame >= len(logoOrigRunes) {
				a.logoPhase = logoPhaseHold
				a.logoFrame = 0
			}
			return a, logoFrameTickCmd(), true
		case logoPhaseHold:
			a.logoFrame++
			if a.logoFrame >= logoHoldFrames {
				a.logoPhase = logoPhaseUnscrambling
				a.logoFrame = 0
			}
			return a, logoFrameTickCmd(), true
		case logoPhaseUnscrambling:
			pos := a.logoPositions[a.logoFrame]
			runes := []rune(a.logoText)
			runes[pos] = logoOrigRunes[pos]
			a.logoText = string(runes)
			a.logoFrame++
			if a.logoFrame >= len(logoOrigRunes) {
				a.logoPhase = logoPhaseIdle
				a.logoFrame = 0
				a.logoText = logoOrig
				return a, a.scheduleLogoAnimCmd(), true
			}
			return a, logoFrameTickCmd(), true
		default: // logoPhaseIdle — consume stale in-flight tick
			return a, nil, true
		}
	}
	return a, nil, false
}

func (a App) handleNotify(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case actionErrMsg:
		a, cmd := a.notify(notifyError, m.err.Error())
		return a, cmd, true
	case notifyMsg:
		a, cmd := a.notify(m.level, m.text)
		return a, cmd, true
	case notifyExpireMsg:
		if m.gen == a.notifyGen {
			a.notifyText = ""
		}
		return a, nil, true
	}
	return a, nil, false
}

// handleUnauthorized intercepts an errMsg or actionErrMsg carrying the
// ErrUnauthorized sentinel — returned by the API client after a token refresh
// fails — and routes the user back to the login screen instead of leaving them
// stranded on an errored screen. The dead refresh token is cleared so the next
// launch starts at the login form rather than retrying a doomed auto-login.
func (a App) handleUnauthorized(msg tea.Msg) (App, tea.Cmd, bool) {
	var err error
	switch m := msg.(type) {
	case errMsg:
		err = m.err
	case actionErrMsg:
		err = m.err
	case notifPostLoadErrMsg:
		err = m.err
	default:
		return a, nil, false
	}
	if !errors.Is(err, api.ErrUnauthorized) || a.active == screenLogin {
		return a, nil, false
	}

	_ = a.client.Logout()
	a.tokens = model.Tokens{}
	a.cmail = a.cmail.CancelUserConvsSubscription()
	a.cmail = a.cmail.CancelSubscription()
	a.chatrooms = a.chatrooms.CancelSubscription()
	a.saveConfig(func(cfg *config.Config) { cfg.RefreshToken = "" })
	// Invalidates the poll/wander/logo-idle tea.Tick chains started by
	// afterLoginCmd: each carries the gen it was scheduled under, so once
	// this no longer matches, they drop themselves on their next fire
	// instead of rescheduling — see sessionGen's doc comment.
	a.sessionGen++

	a.active = screenLogin
	a.focus = focusMenu
	a.login = screens.NewLoginModel(a.currentUser.Email)

	a, cmd := a.notify(notifyWarn, "session expired — please log in again")
	return a, cmd, true
}

func (a App) handleErr(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(errMsg)
	if !ok {
		return a, nil, false
	}
	switch a.active {
	case screenFeed:
		a.feed = a.feed.SetError(m.err)
	case screenCMail:
		a.cmail = a.cmail.SetError(m.err)
	case screenProfile:
		a.profile = a.profile.SetError(m.err)
	case screenPostDetail:
		a.postDetail = a.postDetail.SetError(m.err)
	case screenNotifications:
		a.notifications = a.notifications.SetError(m.err)
	case screenSettings:
		a.settingsScreen = a.settingsScreen.SetError(m.err)
	case screenBookmarks:
		a.bookmarks = a.bookmarks.SetError(m.err)
	case screenGuilds:
		a.guilds = a.guilds.SetError(m.err)
	case screenTopics:
		a.topics = a.topics.SetError(m.err)
	case screenJournal:
		a.journal = a.journal.SetError(m.err)
	case screenSearch:
		a.search = a.search.SetError(m.err)
	}
	// Errors never block a screen: the per-screen SetError above only feeds an
	// inline "couldn't load" empty-state, while the failure is announced in the
	// transient global banner so it is visible even when content is already shown.
	a, cmd := a.notify(notifyError, friendlyErr(m.err))
	return a, cmd, true
}

// friendlyErr converts an API error into human-facing banner text, softening the
// raw "API error NOT_FOUND (404): …" wording for the common deleted-resource case.
func friendlyErr(err error) string {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 404:
			return "Not found — it may have been deleted."
		case apiErr.Code == "EMAIL_NOT_VERIFIED":
			return "Please verify your email — check your inbox for the verification link."
		}
	}
	return err.Error()
}

func (a *App) delegateUpdate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	*a, cmd = a.layout.DelegateUpdate(msg, *a)
	return cmd
}

// --- view ---

func (a App) View() string {
	if a.active == screenLogin {
		return a.login.View()
	}
	return a.layout.View(a)
}

// activeScreenHasFocusedInput returns true when the current screen has a
// text input that is focused, preventing arrow keys from being consumed by
// the tab navigator instead.
func (a App) activeScreenHasFocusedInput() bool { return a.layout.HasFocusedInput(a) }

// --- theme picker ---

// handleThemePickerKey processes keyboard input while the theme picker is open.
func (a App) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	refreshCmd := func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
	// preview applies the theme at the cursor for a live preview. "custom"
	// only previews if a palette has already been saved — otherwise there's
	// nothing to show yet, so the current theme stays put until 'e' builds one.
	preview := func() {
		name := availableThemes[a.themePickerCursor]
		if name == "custom" {
			if a.customPalette == nil {
				return
			}
			theme.SetCustomPalette(*a.customPalette)
		}
		theme.Set(name)
		a.refreshViewports()
	}
	// rowPalette resolves the full palette for the picker's currently
	// highlighted row: a built-in's literal colors, or the saved custom
	// palette (ok=false if the row is "custom" and nothing's been saved yet
	// — there's nothing to edit-as-a-starting-point-from or export there).
	rowPalette := func() (theme.Palette, bool) {
		name := availableThemes[a.themePickerCursor]
		if name == "custom" {
			if a.customPalette == nil {
				return theme.Palette{}, false
			}
			return *a.customPalette, true
		}
		return theme.BuiltinPalette(name)
	}
	switch msg.String() {
	case "up", "k":
		a.themePickerCursor = (a.themePickerCursor - 1 + len(availableThemes)) % len(availableThemes)
		preview()
	case "down", "j":
		a.themePickerCursor = (a.themePickerCursor + 1) % len(availableThemes)
		preview()
	case "e":
		prefill, ok := rowPalette()
		if !ok {
			prefill = theme.CurrentPalette() // "custom" row, nothing saved yet — fall back like before
		}
		a.themeEditorOrig = a.themePickerOrig
		a.themeEditorOrigPalette = theme.CurrentPalette() // snapshot before preview, for correct revert
		theme.SetCustomPalette(prefill)
		theme.Set("custom")
		a.refreshViewports()
		a.themePickerOpen = false
		a.themeEditorOpen = true
		a.themeEditor = screens.NewThemeEditorModel(prefill)
		return a, nil
	case "x":
		p, ok := rowPalette()
		if !ok {
			return a, nil // "custom" row, nothing saved yet to export
		}
		a.themePickerOpen = false
		a.pathPromptOpen = true
		a.pathPromptPurpose = pathPromptExport
		a.pathPromptOverwritePending = ""
		a.pathPromptExportPalette = p
		var cmd tea.Cmd
		a.pathPrompt, cmd = a.pathPrompt.Open("export theme to", defaultThemeFilePath)
		return a, cmd
	case "i":
		a.themePickerOpen = false
		a.pathPromptOpen = true
		a.pathPromptPurpose = pathPromptImport
		a.pathPromptOverwritePending = ""
		var cmd tea.Cmd
		a.pathPrompt, cmd = a.pathPrompt.Open("import theme from", defaultThemeFilePath)
		return a, cmd
	case "enter":
		selected := availableThemes[a.themePickerCursor]
		if selected == "custom" && a.customPalette == nil {
			return a, nil // nothing saved yet — press 'e' to build one
		}
		a.themePickerOpen = false
		return a, tea.Batch(
			refreshCmd,
			func() tea.Msg {
				a.saveConfig(func(cfg *config.Config) {
					cfg.Theme = selected
				})
				return nil
			},
		)
	case "esc":
		theme.Set(a.themePickerOrig)
		a.themePickerOpen = false
		return a, refreshCmd
	}
	return a, nil
}

// handleThemeEditorKey forwards keyboard input to the theme editor model
// while it's open.
func (a App) handleThemeEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.themeEditor, cmd = a.themeEditor.Update(msg)
	return a, cmd
}

// handlePathPromptKey forwards keyboard input to the path prompt model
// while it's open.
func (a App) handlePathPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.pathPrompt, cmd = a.pathPrompt.Update(msg)
	return a, cmd
}

// refreshViewports forces all screen viewports to re-render with the current
// theme by re-broadcasting the current terminal size. Called synchronously so
// View() sees fresh content in the same frame.
func (a *App) refreshViewports() {
	msg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(a.width), Height: a.layout.ContentHeight(a.height)}
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.journal, _ = a.journal.Update(msg)
}

// --- help modal ---

// handleHelpModalKey closes the help modal on any keypress.
func (a App) handleHelpModalKey(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	a.helpModalOpen = false
	return a, nil
}

// getFocusedURLs returns URLs from the currently selected item on the active screen.
func (a App) getFocusedURLs() []string {
	var p screens.URLProvider
	switch a.active {
	case screenFeed:
		p = a.feed
	case screenPostDetail:
		p = a.postDetail
	case screenProfile:
		p = a.profile
	case screenBookmarks:
		p = a.bookmarks
	case screenGuilds:
		p = a.guilds
	case screenTopics:
		p = a.topics
	case screenJournal:
		p = a.journal
	case screenChatrooms:
		p = a.chatrooms
	case screenCMail:
		p = a.cmail
	}
	if p == nil {
		return nil
	}
	return p.GetFocusedURLs()
}

// handleOpenURL opens the given URLs: nothing if empty, direct open if one,
// or shows the picker if multiple.
func (a App) handleOpenURL(urls []string) (App, tea.Cmd) {
	if len(urls) == 0 {
		return a, nil
	}
	if len(urls) == 1 {
		return a.routeURL(urls[0])
	}
	a.urlPickerOpen = true
	a.urlPickerItems = urls
	a.urlPickerCursor = 0
	return a, nil
}

// reservedTopLevelPaths are real, distinct top-level pages on
// cyberspace.online (confirmed live) that must never be misread as a bare
// `/{username}` profile path. None of them map to a TUI screen, so they fall
// through to the OS browser like any other non-cyberspace URL.
var reservedTopLevelPaths = map[string]bool{
	"topics": true, "guilds": true, "chat": true,
	"search": true, "notifications": true, "bookmarks": true, "settings": true,
	"jukebox": true, "globe": true, "fortune": true, "wiki": true,
	"webring": true, "faq": true, "netiquette": true, "contact": true,
	"impressum": true, "support": true, "terms": true, "privacy-policy": true,
	"changelog": true, "admin": true,
}

// routeURL navigates to an internal screen for known cyberspace.online paths,
// opens images in the terminal viewer when supported, or falls through to the
// OS default browser.
func (a App) routeURL(rawURL string) (App, tea.Cmd) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		if a.ephemeral {
			return a.notify(notifyInfo, "Opening links is disabled in SSH sessions")
		}
		return a, openExternalURL(rawURL)
	}
	if parsed.Host == "cyberspace.online" || parsed.Host == "www.cyberspace.online" {
		parts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 3)
		switch {
		case parts[0] == "topics":
			if len(parts) >= 2 && parts[1] != "" {
				a.topics = a.topics.OpenTopic(parts[1])
				a, cmd := activateScreen(a, screenTopics)
				return a, tea.Batch(cmd, a.loadTopicPostsCmd(parts[1]))
			}
			return activateScreen(a, screenTopics)
		case parts[0] == "guilds":
			if len(parts) >= 2 && parts[1] != "" {
				a.guilds = a.guilds.OpenGuild(parts[1])
				a, cmd := activateScreen(a, screenGuilds)
				return a, tea.Batch(cmd, a.loadGuildPostsCmd(parts[1]), a.loadGuildDetailCmd(parts[1]))
			}
			return activateScreen(a, screenGuilds)
		case parts[0] == "chat":
			if len(parts) >= 2 && parts[1] != "" {
				slug := parts[1]
				return a, func() tea.Msg { return screens.OpenRoomMsg{RoomSlug: slug} }
			}
			return activateScreen(a, screenChatrooms)
		case len(parts) >= 2 && parts[0] == "u" && parts[1] != "":
			a.profileReturn = a.active
			return a, a.loadUserProfileCmd(parts[1])
		case len(parts) == 3 && parts[1] == "blog" && parts[0] != "" && parts[2] != "":
			// /{username}/blog/{slug} — the canonical-form post permalink.
			return a, a.loadPostBySlugCmd(parts[0], parts[2], a.active)
		case len(parts) == 2 && parts[0] != "" && parts[1] != "" && !reservedTopLevelPaths[parts[0]]:
			// /{username}/{slug} — the short-form post permalink.
			return a, a.loadPostBySlugCmd(parts[0], parts[1], a.active)
		case len(parts) == 1 && parts[0] != "" && !reservedTopLevelPaths[parts[0]]:
			// /{username} — profile.
			a.profileReturn = a.active
			return a, a.loadUserProfileCmd(parts[0])
		}
	}
	// Ephemeral (SSH-hosted) sessions must never launch host processes or make
	// the host fetch remote-chosen URLs (browser spawn / SSRF).
	if a.ephemeral {
		return a.notify(notifyInfo, "Opening links is disabled in SSH sessions")
	}
	if a.canRenderImageInline(rawURL) || a.canProbeImageInline() {
		return a.openImageInTerminal(rawURL)
	}
	return a, openExternalURL(rawURL)
}

// canRenderImageInline reports whether u should be displayed in the inline
// terminal image viewer rather than the OS browser: it must look like an
// image, the terminal must support a graphics protocol, the user's image
// viewer setting must not be "browser", and the session must not be an
// ephemeral SSH-hosted one (which must never have the host fetch a
// remote-chosen URL — see the SSRF guard in routeURL).
func (a App) canRenderImageInline(u string) bool {
	return !a.ephemeral &&
		urlutil.IsImageURL(u) &&
		a.graphicsProtocol != imgview.ProtocolNone &&
		a.imageViewer != "browser"
}

// canProbeImageInline reports whether u is worth an inline-open attempt even
// though its extension doesn't look like an image: /gif <url> in circ/C-Mail
// forwards whatever URL the user typed verbatim (chatrooms.go's slash-command
// handling does no URL validation), and real gif links routinely lack a
// literal ".gif" suffix (Tenor share pages, extensionless CDN links). Rather
// than guess further from the URL text, this defers to openImageInTerminal's
// real fetch+decode (via imgview.FetchAny, which sniffs the downloaded
// bytes) as the source of truth, falling back to the browser via
// handleImageViewer's existing error path when the content genuinely isn't a
// decodable image. Same gates as canRenderImageInline minus the extension
// check.
func (a App) canProbeImageInline() bool {
	return !a.ephemeral &&
		a.graphicsProtocol != imgview.ProtocolNone &&
		a.imageViewer != "browser"
}

// canInlineImages reports whether Feed/PostDetail should render post
// attachments inline: the user's InlineImages preference is on, plus the
// same protocol/imageViewer/ephemeral gates as the fullscreen image viewer
// (see canRenderImageInline). There's no URL to check yet here — this gates
// the feature as a whole, per-attachment checks happen when rendering.
func (a App) canInlineImages() bool {
	return a.inlineImages &&
		!a.ephemeral &&
		a.graphicsProtocol != imgview.ProtocolNone &&
		a.imageViewer != "browser"
}

// ditheringEnabled reports whether images encoded for the terminal should
// have the RasterImage-style duotone dither effect applied (see Dither):
// the user's Dithering preference is on, gated by the same imageViewer
// check as graphicsProtocol/inlineImages — dithering is meaningless when
// images open in the OS browser instead.
func (a App) ditheringEnabled() bool {
	return a.dithering && a.imageViewer != "browser"
}

// ditherOptions builds the imgview.DitherOptions to pass to Encode* when
// ditheringEnabled is true, resolving the current theme's colors. Returns
// nil when dithering is off.
func (a App) ditherOptions() *imgview.DitherOptions {
	if !a.ditheringEnabled() {
		return nil
	}
	fg, _ := imgview.ParseHexColor(theme.CurrentPalette().Foreground)
	bg, ok := imgview.ParseHexColor(theme.CurrentPalette().Background)
	if !ok {
		// Palette.Background is reserved/usually empty (see theme.go's Palette
		// doc comment) — fall back to the theme's actual rendered background.
		bg, _ = imgview.ParseHexColor(string(theme.ColorBackground))
	}
	return &imgview.DitherOptions{
		PixelSize: imgview.PixelSizeForSharpness(a.ditherSharpness),
		FgColor:   fg,
		BgColor:   bg,
	}
}

// activeInlineImageSlots returns the active screen's currently visible
// inline image slots, or nil for screens that don't support inline images.
// Used by TabsLayout.InlineImageSlots; MillerLayout has its own equivalent
// since its screen geometry (and, for Feed/Guilds/Topics, which screen
// method even has the current content — see FeedModel.VisibleDetailInlineImages)
// differs from Tabs'.
func (a App) activeInlineImageSlots() []screens.InlineImageSlot {
	switch a.active {
	case screenPostDetail:
		return a.postDetail.VisibleInlineImages()
	case screenFeed:
		return a.feed.VisibleInlineImages()
	case screenSearch:
		return a.search.VisibleInlineImages()
	case screenTopics:
		return a.topics.VisibleInlineImages()
	case screenGuilds:
		return a.guilds.VisibleInlineImages()
	case screenCMail:
		return a.cmail.VisibleInlineImages()
	case screenChatrooms:
		return a.chatrooms.VisibleInlineImages()
	case screenProfile:
		return a.profile.VisibleInlineImages()
	default:
		return nil
	}
}

// activeSelectionKey returns the ID of whatever's currently selected on the
// active screen (a post or reply ID, matching the format embedded in
// InlineImageSlot.Key) — used by syncInlineImages/selectionTouchesSlot to
// detect a selection change that recolors a card hosting a currently-visible
// inline image: selection changes the (de)selected card's border color
// across every one of its lines, including its inline-image band rows,
// which erases the image pixels there without moving anything
// inlineImageSignature (position-only) tracks.
func (a App) activeSelectionKey() string {
	switch a.active {
	case screenPostDetail:
		// SelectedReplyID returns "" when the post itself is selected (not
		// a reply) — but selectionTouchesSlot treats "" as "nothing
		// selected" and always returns false for it (its own doc comment:
		// an id that's merely a substring of another id must not match, and
		// "" would match everything). Without this fallback, toggling
		// between the post and a reply selected — recoloring the post's
		// own border, including the rows its inline image sits in — never
		// registers as touching that image at all, so imageRepaintGen
		// never bumps and the image can be silently wiped by the border's
		// own legitimate resend with nothing forcing it back. Confirmed
		// live and via debug logging (docs/plan-inline-images-
		// improvements.md Round 14) — this fired once on entering
		// PostDetail (Feed's own non-empty selection key) but never again
		// for any subsequent post/reply toggle within PostDetail itself.
		if id := a.postDetail.SelectedReplyID(); id != "" {
			return id
		}
		return a.postDetail.PostID()
	case screenFeed:
		return a.feed.SelectedPostID()
	case screenSearch:
		return a.search.SelectedPostID()
	case screenTopics:
		return a.topics.SelectedPostID()
	case screenGuilds:
		return a.guilds.SelectedPostID()
	case screenCMail:
		return a.cmail.SelectedMessageID()
	case screenChatrooms:
		return a.chatrooms.SelectedMessageID()
	default:
		return ""
	}
}

// selectionTouchesSlot reports whether id (from activeSelectionKey or
// FeedModel.DetailSelectionKey) belongs to any currently-visible
// inline-image slot — i.e. whether a selection change involving id could
// have recolored a card that hosts an on-screen image. Slot keys are always
// "post:<id>:<n>" or "reply:<id>:<n>" (see feed.go/postdetail.go's
// VisibleInlineImages), so a ":id:" substring match catches both without
// needing to know which, and the leading/trailing ":" delimiters keep an id
// that's merely a substring of another id from matching.
func selectionTouchesSlot(id string, slots []screens.InlineImageSlot) bool {
	if id == "" {
		return false
	}
	needle := ":" + id + ":"
	for _, s := range slots {
		if strings.Contains(s.Key, needle) {
			return true
		}
	}
	return false
}

// inlineImageRect is an absolute-screen-position rectangle an iTerm2/Sixel
// inline image occupies or occupied. Cols/Rows come from
// InlineImageSlot.MaxCols/MaxRows (the reserved band bounds), which are
// always >= the image's actual displayed size (see imgview's fitBox), so
// Row/Rows always cover the image's full footprint regardless of the
// image's real aspect-ratio-fitted extent — see syncInlineImageErasures,
// which uses that range to compute which screen rows to force redrawn.
type inlineImageRect struct {
	Row, Col, Cols, Rows int
}

// syncInlineImageErasures computes this frame's key->rect map from slots
// (resolving each slot's viewport-relative position to an absolute one via
// rowOrigin/colOrigin) and diffs it against prevRects, the previous frame's
// map, to find rows whose previously-drawn image content is now stale: any
// key that's gone or moved contributes its OLD rect's row range. Rather than
// guessing "blank" is the right replacement for those rows — an out-of-band
// write invisible to Bubble Tea's own per-line diff cache, and wrong the
// moment different real content later renders there — the caller
// (forceRowsDirty in layout.go) uses the returned row numbers to force
// Bubble Tea's own diff to resend each row's real, always-correct content,
// the same technique inlineImagePaintGen already uses for the
// selection-touch case. That makes this safe to recompute fresh every call
// with no carry-forward: even losing a transition to Bubble Tea's renderer
// coalescing several Update() calls before a flush just leaves a row stale
// a little longer, self-healing as soon as that row's content next changes
// — never the permanent corruption of unrelated content an unclaimed
// out-of-band blank-fill risked.
func syncInlineImageErasures(slots []screens.InlineImageSlot, rowOrigin, colOrigin int, prevRects map[string]inlineImageRect) (current map[string]inlineImageRect, staleRows []int) {
	current = make(map[string]inlineImageRect, len(slots))
	for _, s := range slots {
		current[s.Key] = inlineImageRect{Row: rowOrigin + s.Row, Col: colOrigin + s.ColIndent, Cols: s.MaxCols, Rows: s.MaxRows}
	}
	seen := make(map[int]bool)
	for key, oldRect := range prevRects {
		newRect, stillThere := current[key]
		if !stillThere || newRect != oldRect {
			for r := oldRect.Row; r < oldRect.Row+oldRect.Rows; r++ {
				if !seen[r] {
					seen[r] = true
					staleRows = append(staleRows, r)
				}
			}
		}
	}
	sort.Ints(staleRows)
	return current, staleRows
}

// inlineImageCacheKey identifies one encoded rendering of a slot's image —
// slot key, URL, column budget, protocol, and dithering state. Keying by
// slot (not just URL) matters for Kitty, whose encoded bytes embed a
// placement id specific to one on-screen instance (see imgview.EncodeKitty)
// — two slots showing the same URL must never share an encode. Also matches
// how a resize (which changes MaxCols) or a protocol change naturally
// invalidates old entries — and, via ditherKeyFragment, how toggling
// dithering, changing sharpness, or switching themes must too: without the
// dither component, an already-cached encode from before such a change
// would keep being served as-is (see syncInlineImages/fetchInlineImageCmd),
// making the change appear to require a restart to take effect.
func inlineImageCacheKey(slot screens.InlineImageSlot, proto imgview.GraphicsProtocol, dither *imgview.DitherOptions) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s", slot.Key, slot.URL, slot.MaxCols, proto, ditherKeyFragment(dither))
}

// ditherKeyFragment returns a compact, deterministic string identifying
// dither's effect on the encoded output, for use as an inlineImageCacheKey
// component — "-" when dithering is off, otherwise a value that changes
// whenever the resulting pixels would: sharpness (PixelSize) or the active
// theme's colors (FgColor/BgColor).
func ditherKeyFragment(dither *imgview.DitherOptions) string {
	if dither == nil {
		return "-"
	}
	return fmt.Sprintf("d%d,%02x%02x%02x,%02x%02x%02x", dither.PixelSize,
		dither.FgColor.R, dither.FgColor.G, dither.FgColor.B,
		dither.BgColor.R, dither.BgColor.G, dither.BgColor.B)
}

// kittyModalPlacementID is a fixed, reserved Kitty placement/image id used
// exclusively by the fullscreen image modal — distinct from inline
// rendering's per-slot ids (see App.kittyPlacementIDs), which start from 1
// and grow with normal use. Giving the modal a dedicated id (rather than
// the anonymous placementID==0 mode EncodeKitty also supports) means its
// own open/close/cycle lifecycle never has to blunt-delete-all placements —
// which would erase every inline image's placement too, now that both
// features can be on screen at once. Re-sending a=T with this same id on
// each open/cycle replaces any previous modal placement per the protocol
// spec, so no separate self-heal delete is needed there either. Deliberately
// NOT the top of Kitty's 32-bit id range (4294967295 / 0xFFFFFFFF): that
// exact value is a conventional "no id"/sentinel value in a lot of unsigned-
// integer code, and terminals implementing the protocol are free to special-
// case it — consistent with the modal flashing open then immediately
// vanishing once this id was used, rather than staying on screen like any
// other named placement. 999000000 is an arbitrary value nowhere near any
// common sentinel (0, 1, INT32_MAX, UINT32_MAX) yet still astronomically far
// from the small incrementing counter inline rendering uses, so it can never
// collide with it either.
const kittyModalPlacementID = 999000000

// syncKittyPlacements assigns a stable id (used as both image id and
// placement id, see imgview.EncodeKitty) to every slot ever seen — new keys
// get the next counter value, permanently; already-tracked keys keep theirs
// forever, whether currently visible or not (see kittyPlacementIDs' doc
// comment on the App struct for why). Comparing this sync's visible set
// against kittyVisibleKeys (last sync's) reports exactly two transitions:
// toDelete, ids for keys that just became invisible, and revived, ids for
// keys that just became visible again — the caller cancels any pending
// delete for a revived id before it can wipe out that key's freshly redrawn
// placement. Returns the updated App, the current key->id mapping (for
// fetchInlineImageCmd to look up), toDelete, and revived.
func (a App) syncKittyPlacements(slots []screens.InlineImageSlot) (App, map[string]int, []int, []int) {
	if a.kittyPlacementIDs == nil {
		a.kittyPlacementIDs = make(map[string]int)
	}
	current := make(map[string]bool, len(slots))
	for _, s := range slots {
		current[s.Key] = true
		if _, ok := a.kittyPlacementIDs[s.Key]; !ok {
			a.kittyNextPlacementID++
			a.kittyPlacementIDs[s.Key] = a.kittyNextPlacementID
		}
	}
	var toDelete, revived []int
	for key := range a.kittyVisibleKeys {
		if !current[key] {
			toDelete = append(toDelete, a.kittyPlacementIDs[key])
		}
	}
	for key := range current {
		if _, wasVisible := a.kittyVisibleKeys[key]; !wasVisible {
			revived = append(revived, a.kittyPlacementIDs[key])
		}
	}
	visible := make(map[string]struct{}, len(current))
	for key := range current {
		visible[key] = struct{}{}
	}
	a.kittyVisibleKeys = visible
	return a, a.kittyPlacementIDs, toDelete, revived
}

// accumulateKittyDeletes merges newlyDropped ids into pending. Merging
// (never overwriting or expiring) is what makes a delete robust against
// Bubble Tea's throttled renderer processing several Updates before it
// actually writes a frame — see pendingKittyDeletes' doc comment on the App
// struct for why even a resend countdown could still lose a delete that was
// never rendered.
func accumulateKittyDeletes(pending map[int]struct{}, newlyDropped []int) map[int]struct{} {
	if pending == nil {
		pending = make(map[int]struct{})
	}
	for _, id := range newlyDropped {
		pending[id] = struct{}{}
	}
	return pending
}

// accumulateStaleRows unions fresh into pending instead of replacing it — the
// same "never lose a transition to Bubble Tea's throttled renderer coalescing
// several Updates before a flush" reasoning as accumulateKittyDeletes, applied
// to inlineImageStaleRows. Without this, a row that goes stale in an Update
// whose View() never actually gets flushed (superseded by a later Update
// before the next render tick) is silently lost: the *next* Update diffs
// against its own freshly tracked position, not against whatever was last
// really painted on the terminal, so the row nothing ever resends can be a
// completely different one than the row that's actually still showing stale
// image pixels on the real screen. Bounded and cheap regardless — row indices
// are capped at the terminal's height — and the caller (syncInlineImages)
// clears the accumulated set once a quiet Update reports no new staleRows,
// so this doesn't grow or persist forever.
func accumulateStaleRows(pending, fresh []int) []int {
	if len(fresh) == 0 {
		return pending
	}
	seen := make(map[int]bool, len(pending)+len(fresh))
	merged := make([]int, 0, len(pending)+len(fresh))
	for _, r := range pending {
		if !seen[r] {
			seen[r] = true
			merged = append(merged, r)
		}
	}
	for _, r := range fresh {
		if !seen[r] {
			seen[r] = true
			merged = append(merged, r)
		}
	}
	sort.Ints(merged)
	return merged
}

// syncInlineImages diffs the active screen's current VisibleInlineImages()
// against inlineImageCache and returns a command that fetches+encodes
// anything missing. It's called once per Update, after the active screen
// has already processed the message. Kitty gets precise per-image
// create/delete via kittyPlacementIDs/pendingKittyDeletes (see
// syncKittyPlacements); Sixel/iTerm2 have no placement-delete primitive, so
// they need two different repaint strategies depending on what changed:
//
//   - A moved or removed image (a scroll changed a slot's Row, or the slot
//     disappeared) means its old and new screen position can differ, and
//     injectInlineImages only ever draws at the *current* position
//     (layout.go, "row := rowOrigin + slot.Row") — nothing there fixes up
//     the old one. syncInlineImageErasures tracks each image's actual
//     on-screen rectangle frame to frame and returns the stale rows (see
//     its doc comment); forceRowsDirty (layout.go) forces Bubble Tea's own
//     per-line diff to resend those rows' real, always-correct content — no
//     reliance on incidental line-diff overwrite, no full-screen
//     tea.ClearScreen (an earlier version used one; flashed on every
//     scroll), and no out-of-band blank-fill guess (a later version used
//     one; could never be safely dropped once unclaimed, corrupting
//     whatever unrelated content later rendered at that screen position).
//   - A selection change that touches a visible image (see
//     selectionTouchesSlot) never moves any slot's Row, so old and new
//     position are always identical — reissuing the same paint in place is
//     guaranteed to fully recover whatever the border recolor incidentally
//     erased, with no risk of a partial-footprint gap. Bumping
//     inlineImagePaintGen (which injectInlineImages reads to force its
//     trailing paint line dirty) gets Bubble Tea's own per-line diff to
//     repaint just that line — the same seamless mechanism an ordinary
//     scroll already uses — without a full-screen flash. Selection changes
//     that don't touch any visible image skip this entirely — bumping on
//     every selection move regardless caused a visible blink on every
//     arrow-key step through a feed with any inline image on screen (an
//     earlier, since-fixed regression). A scroll needs no paint-gen bump
//     of its own: a moved slot's own cursor-jump text already differs in
//     bytes purely because its row/col changed, so Bubble Tea's line-diff
//     already resends it, and a changed inlineImageStaleRows set
//     independently forces the affected rows dirty too.
func (a App) syncInlineImages() (App, tea.Cmd) {
	var slots []screens.InlineImageSlot
	var rowOrigin, colOrigin int
	var selKey string
	if a.canInlineImages() {
		slots, rowOrigin, colOrigin, selKey = a.layout.InlineImageSlots(a)
	}
	var cmds []tea.Cmd

	isKitty := a.graphicsProtocol == imgview.ProtocolKitty
	var placementIDs map[string]int
	if isKitty {
		var toDelete, revived []int
		a, placementIDs, toDelete, revived = a.syncKittyPlacements(slots)
		for _, id := range revived {
			delete(a.pendingKittyDeletes, id)
		}
		a.pendingKittyDeletes = accumulateKittyDeletes(a.pendingKittyDeletes, toDelete)
		a.inlineImageStaleRows = nil
	} else {
		// iTerm2 and Sixel both need to know which rows just went stale (a
		// moved or removed image) — the two protocols just do different
		// things with that fact in View() (see injectInlineImages,
		// layout.go): iTerm2 resends the row's real content in place with no
		// erase (forceRowsDirty). Sixel needs an actual full-screen erase —
		// proven, on real Konsole hardware across three rounds of live
		// testing, to be the only thing that reliably clears its raster
		// pixels at all — done as a single write from View()
		// (ansi.EraseEntireScreen prepended to the frame plus every row
		// forced dirty) rather than a tea.ClearScreen Cmd, whose immediate
		// erase-write followed by a delayed content flush on the next render
		// tick is what caused the bad flicker in the tea.ClearScreen attempt
		// (confirmed by reading bubbletea's
		// standardRenderer.clearScreen()/flush(), v1.3.10). Both protocols
		// need imageRepaintGen bumped here regardless: whichever repaint
		// mechanism runs, it must be collision-proof against Bubble Tea
		// dropping intermediate View() calls under fast input — see
		// imageRepaintGen's doc comment (App struct).
		// Accumulate rather than overwrite a.inlineImageStaleRows: see
		// accumulateStaleRows' doc comment for why a fresh staleRows computed
		// against only the immediately preceding Update's tracked position
		// can miss the row that's actually still stale on the real,
		// last-flushed terminal screen if several Updates get coalesced into
		// one flush. Cleared only once a quiet Update (no new staleRows) AND
		// inlineImageStaleGrace has elapsed since inlineImageStaleSince — see
		// its doc comment (App struct) for why "the very next quiet Update"
		// alone isn't a safe clear signal in this app.
		current, staleRows := syncInlineImageErasures(slots, rowOrigin, colOrigin, a.inlineImageVisibleRects)
		a.inlineImageVisibleRects = current
		if len(staleRows) > 0 {
			a.inlineImageStaleRows = accumulateStaleRows(a.inlineImageStaleRows, staleRows)
			a.inlineImageStaleSince = time.Now()
			a.imageRepaintGen++
		} else if time.Since(a.inlineImageStaleSince) > inlineImageStaleGrace {
			a.inlineImageStaleRows = nil
		}

		selChanged := selKey != a.inlineImageLastSelKey
		touchesVisible := selChanged && (selectionTouchesSlot(selKey, slots) || selectionTouchesSlot(a.inlineImageLastSelKey, slots))
		a.inlineImageLastSelKey = selKey
		if touchesVisible {
			a.inlineImagePaintGen++
			// See imageRepaintGen's doc comment (App struct): a selection
			// recolor without a position change never touches
			// inlineImageStaleRows, so this is the only trigger for that
			// case — needed so injectInlineImages' trailing-line marker
			// (layout.go) is collision-proof here too, not just for the
			// stale-row case.
			a.imageRepaintGen++
		}
	}

	if a.inlineImageFetching == nil {
		a.inlineImageFetching = make(map[string]bool)
	}
	ditherOpts := a.ditherOptions()
	for _, slot := range slots {
		key := inlineImageCacheKey(slot, a.graphicsProtocol, ditherOpts)
		if _, cached := a.inlineImageCache[key]; cached {
			a.touchInlineImageCache(key)
			continue
		}
		if a.inlineImageFetching[key] {
			continue
		}
		if failedAt, failed := a.inlineImageFailedAt[key]; failed && time.Since(failedAt) < inlineImageFailureCooldown {
			continue
		}
		a.inlineImageFetching[key] = true
		placementID := 0
		if isKitty {
			placementID = placementIDs[slot.Key]
		}
		cmds = append(cmds, a.fetchInlineImageCmd(slot, key, placementID, ditherOpts))
	}
	if len(cmds) == 0 {
		return a, nil
	}
	return a, tea.Batch(cmds...)
}

// inlineImageFetchedMsg reports the result of one fetchInlineImageCmd.
// slotKey/rows are only meaningful on success: slotKey is the originating
// slot's stable Key (screens.InlineImageSlot.Key, e.g. "circmsg:m1:0"),
// used to route the real row count back to the screen that owns it (see
// handleInlineImageFetched); rows is the actual terminal rows the image was
// fit into — from the encoder's own aspect-ratio-preserving fit-box
// calculation, always <= the maxRows requested.
type inlineImageFetchedMsg struct {
	key     string
	slotKey string
	rows    int
	encoded string
	err     error
}

// fetchInlineImageCmd fetches and encodes slot's image for the currently
// detected protocol. placementID is only used for Kitty (see
// imgview.EncodeKitty); callers pass 0 for Sixel/iTerm2. ditherOpts is
// passed in (rather than recomputed via a.ditherOptions()) so it always
// matches exactly what the caller used to compute key via
// inlineImageCacheKey — a mismatch there would cache the encode under one
// dither state while the bytes reflect another. Unlike the fullscreen
// modal, there's no user-visible loading state to manage: a miss this frame
// just means the reserved blank band stays blank until the result lands.
func (a App) fetchInlineImageCmd(slot screens.InlineImageSlot, key string, placementID int, ditherOpts *imgview.DitherOptions) tea.Cmd {
	proto := a.graphicsProtocol
	maxCols, maxRows := slot.MaxCols, slot.MaxRows
	url := slot.URL
	slotKey := slot.Key
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		img, err := imgview.Fetch(ctx, url)
		if err != nil {
			return inlineImageFetchedMsg{key: key, err: err}
		}
		var encoded string
		var rows int
		cellW, cellH, _ := imgview.TerminalCellPixelSize(int(os.Stdout.Fd()))
		switch proto {
		case imgview.ProtocolITerm2:
			encoded, _, rows, err = imgview.EncodeITerm2(img, maxCols, maxRows, cellW, cellH, false, ditherOpts)
		case imgview.ProtocolSixel:
			encoded, _, rows, err = imgview.EncodeSixel(img, maxCols, maxRows, cellW, cellH, false, ditherOpts)
		case imgview.ProtocolKitty:
			encoded, _, rows, err = imgview.EncodeKitty(img, maxCols, maxRows, cellW, cellH, placementID, false, ditherOpts)
		default:
			err = fmt.Errorf("inline images: unsupported protocol %v", proto)
		}
		if err != nil {
			return inlineImageFetchedMsg{key: key, err: err}
		}
		return inlineImageFetchedMsg{key: key, slotKey: slotKey, rows: rows, encoded: encoded}
	}
}

// inlineImageStaleGrace bounds how soon a.inlineImageStaleRows can be
// cleared after its last addition — see inlineImageStaleSince's doc comment
// (App struct) for why a single quiet Update isn't a safe clear signal on
// its own. Chosen comfortably larger than the feed's known multi-Update gap
// (feedMergeAnimDelay, 200ms — the top-of-feed background-poll merge
// animation) so the accumulator survives however many unrelated ticks land
// in between, while still clearing well within a second of the last real
// change rather than accumulating indefinitely.
const inlineImageStaleGrace = 500 * time.Millisecond

// inlineImageFailureCooldown bounds how often a permanently-broken image URL
// gets refetched — without it, a dead link left visible while the user is
// idle-scrolling elsewhere on the same screen fires a fresh HTTP request on
// every single Update (keystroke, tick, anything) that includes its slot in
// the visible set. Not tied to a resize/navigation-triggered retry: the slot
// naturally gets another attempt on its own once the cooldown lapses.
const inlineImageFailureCooldown = 60 * time.Second

// handleInlineImageFetched processes an inlineImageFetchedMsg: on success,
// caches the encoded result so the next frame's render picks it up, and
// clears any earlier failure record for the key; on failure, records when it
// failed (see inlineImageFailureCooldown) so syncInlineImages doesn't retry
// it on every subsequent Update — there's no modal to fall back to here, the
// slot simply stays blank until either the cooldown lapses or something
// invalidates the key outright (e.g. a resize changing its column budget).
func (a App) handleInlineImageFetched(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(inlineImageFetchedMsg)
	if !ok {
		return a, nil, false
	}
	delete(a.inlineImageFetching, m.key)
	if m.err != nil {
		if a.inlineImageFailedAt == nil {
			a.inlineImageFailedAt = make(map[string]time.Time)
		}
		a.inlineImageFailedAt[m.key] = time.Now()
		return a, nil, true
	}
	delete(a.inlineImageFailedAt, m.key)
	a = a.cacheInlineImage(m.key, m.encoded)
	a = a.recordInlineImageRealRows(m.slotKey, m.rows)
	return a, nil, true
}

// recordInlineImageRealRows tells whichever screen owns slotKey how many
// rows its image actually rendered into, so that screen can shrink its
// reserved band down from the fallback-max placeholder to the image's real
// size — see CMailModel/ChatroomsModel.SetImageRealRows. Only chat screens
// support this today (Feed/PostDetail/Search/Topics/Guilds/Profile still use
// a fixed fallback band); a slotKey belonging to one of those, or rows <= 0
// (encode error/degenerate case), is a no-op.
func (a App) recordInlineImageRealRows(slotKey string, rows int) App {
	if rows <= 0 {
		return a
	}
	switch {
	case strings.HasPrefix(slotKey, "cmailmsg:"):
		a.cmail = a.cmail.SetImageRealRows(slotKey, rows)
	case strings.HasPrefix(slotKey, "circmsg:"):
		a.chatrooms = a.chatrooms.SetImageRealRows(slotKey, rows)
	}
	return a
}

// inlineImageCacheMaxBytes bounds inlineImageCache's total payload size —
// see cacheInlineImage. A long scrolling session or a terminal resized many
// times would otherwise grow the cache for the life of the process. Chosen
// as a reasonable starting cap, not derived from measurement (ponytail:
// revisit if real sessions show it's too tight or too loose in practice).
const inlineImageCacheMaxBytes = 16 << 20 // 16 MiB

// touchInlineImageCache marks key as most-recently-used without changing its
// cached value — called for every cache hit on a currently-visible slot
// (syncInlineImages), so a still-on-screen image is never the oldest entry
// and therefore never the one cacheInlineImageBounded evicts first. Without
// this, inlineImageCacheOrder only ever moved on write (cacheInlineImage),
// making eviction FIFO-by-insertion rather than truly LRU: an image that's
// been sitting on screen the whole time could still get evicted purely
// because other images were fetched more recently, leaving its row a cache
// miss (injectInlineImages, layout.go) — a real, no-user-action image
// blackout distinct from any of the scroll/redraw-timing bugs fixed
// earlier. inlineImageCacheOrder/inlineImageCacheElems are reference types
// (*list.List, map), so mutating through the pointer/map here is visible
// without needing to reassign or return App.
func (a App) touchInlineImageCache(key string) {
	if a.inlineImageCacheOrder == nil {
		return
	}
	if elem, ok := a.inlineImageCacheElems[key]; ok {
		a.inlineImageCacheOrder.MoveToBack(elem)
	}
}

// cacheInlineImage stores encoded under key in inlineImageCache, evicting
// the oldest-inserted entries first once inlineImageCacheMaxBytes is
// exceeded. Eviction only removes the cache entry, never the corresponding
// kittyPlacementIDs id (see that field's doc comment) — cache keys don't
// embed the id, so a slot whose entry gets evicted just re-fetches and
// re-encodes using its already-stable id on the next sync, the same as any
// other cache miss.
func (a App) cacheInlineImage(key, encoded string) App {
	return a.cacheInlineImageBounded(key, encoded, inlineImageCacheMaxBytes)
}

// cacheInlineImageBounded is cacheInlineImage with an injectable cap, so
// tests can exercise eviction with a small maxBytes instead of megabytes of
// fixture data.
func (a App) cacheInlineImageBounded(key, encoded string, maxBytes int) App {
	if a.inlineImageCache == nil {
		a.inlineImageCache = make(map[string]string)
		a.inlineImageCacheOrder = list.New()
		a.inlineImageCacheElems = make(map[string]*list.Element)
	}
	if old, exists := a.inlineImageCache[key]; exists {
		a.inlineImageCacheBytes -= len(old)
		a.inlineImageCacheOrder.MoveToBack(a.inlineImageCacheElems[key])
	} else {
		a.inlineImageCacheElems[key] = a.inlineImageCacheOrder.PushBack(key)
	}
	a.inlineImageCache[key] = encoded
	a.inlineImageCacheBytes += len(encoded)

	for a.inlineImageCacheBytes > maxBytes && a.inlineImageCacheOrder.Len() > 1 {
		oldest := a.inlineImageCacheOrder.Front()
		oldestKey := oldest.Value.(string)
		a.inlineImageCacheBytes -= len(a.inlineImageCache[oldestKey])
		delete(a.inlineImageCache, oldestKey)
		delete(a.inlineImageCacheElems, oldestKey)
		a.inlineImageCacheOrder.Remove(oldest)
	}
	return a
}

// openExternalURL opens u in the OS default browser as a fire-and-forget command.
func openExternalURL(u string) tea.Cmd {
	return func() tea.Msg {
		_ = urlutil.OpenURL(u)
		return nil
	}
}

// imageScaleStep is the fraction of the image's own native size that +/-
// nudges App.imageScale by per keypress while the fullscreen image modal is
// open — scale is relative to native size (see openImageInTerminal), not
// the terminal window, so this is normally a visible change. adjustImageScale
// additionally guarantees at least a 1-cell step regardless of this fraction,
// for images small enough that 10% of their native size would otherwise
// round away to nothing. Bounds match config.Config.GetImageScale's clamp.
const imageScaleStep = 0.1

// modalScreenMarginFrac caps the image modal at 80% of the terminal's
// width/height on every layout, regardless of scale — the exact fraction
// the modal was hard-capped to before native-relative scaling and
// upscaling existed (the original "a.width*4/5" box). Raw Sixel/iTerm2/
// Kitty image payloads are spliced directly into Bubble Tea's rendered
// frame via cursor-jump sequences (compositeOverlays, layout.go) and are a
// documented source of terminal-side rendering desync when they get large
// — see sixelFullRepaint's and forceRowsDirty's doc comments (layout.go)
// for the same class of bug fought before scale ever let the modal approach
// full-screen size. Keeping this headroom is a mitigation, not a fix for
// that underlying fragility — restores the margin that kept it from
// surfacing in practice.
const modalScreenMarginFrac = 0.8

// adjustImageScale nudges the live modal image scale by delta (positive to
// grow, negative to shrink) and re-runs openImageInTerminal for the
// currently open image so the modal re-renders at the new scale
// immediately. Unlike a flat float add, the step is computed in terminal
// cells against the image's own native size (imgview.NativeCellBox) and
// floored at 1 cell — so every press changes the rendered size by at least
// one cell until the true min (config.MinImageScale) or max
// (config.MaxImageScale) bound is hit, rather than being silently absorbed
// by rounding (or, before this design, by fitBox's never-upscale native-size
// cap — see docs/46-image-modal-scale.md for the investigation that found
// this). No-op if the modal's image isn't cached yet, which shouldn't happen
// while it's open (the cache is populated the moment it opens) but avoids a
// lookup panic.
//
// The step is based on a.imageModalCols — the box actually on screen right
// now — not on a value recomputed from a.imageScale. Those two can diverge:
// openImageInTerminal additionally clamps the rendered box to
// modalScreenMarginFrac/Layout.ModalMaxWidth, which is a tighter ceiling
// than config.MaxImageScale in a small terminal. Stepping from the
// unclamped scale let repeated "+" presses at that ceiling keep inflating
// a.imageScale with no visible effect (correctly, since the box was already
// maxed), but then the first "-" press only walked the invisible excess
// back down one step, so it visibly did nothing either — reported live as
// "+ does nothing at max size, but then - does nothing too, the first
// time." Anchoring to the real rendered box instead means the very next
// "-" press always immediately shrinks it.
func (a App) adjustImageScale(delta float64) (App, tea.Cmd) {
	cached, hit := a.imageCache[a.imageModalURL]
	if !hit || len(cached.frames) == 0 {
		return a, nil
	}
	cellW, cellH, _ := imgview.TerminalCellPixelSize(int(os.Stdout.Fd()))
	b := cached.frames[0].Bounds()
	nativeCols, _ := imgview.NativeCellBox(b.Dx(), b.Dy(), cellW, cellH)

	currentCols := a.imageModalCols
	if currentCols < 1 {
		scale := a.imageScale
		if scale <= 0 {
			scale = 1.0
		}
		currentCols = int(float64(nativeCols)*scale + 0.5)
	}
	step := int(float64(nativeCols)*imageScaleStep + 0.5)
	if step < 1 {
		step = 1
	}
	targetCols := currentCols + step
	if delta < 0 {
		targetCols = currentCols - step
	}
	minCols := int(float64(nativeCols) * config.MinImageScale)
	if minCols < 1 {
		minCols = 1
	}
	maxCols := int(float64(nativeCols) * config.MaxImageScale)
	if maxCols < minCols {
		maxCols = minCols
	}
	if targetCols < minCols {
		targetCols = minCols
	}
	if targetCols > maxCols {
		targetCols = maxCols
	}

	a.imageScale = float64(targetCols) / float64(nativeCols)
	return a.openImageInTerminal(a.imageModalURL)
}

// openImageInTerminal fetches rawURL, encodes it for the detected graphics
// protocol, and returns a command that sends an imageFetchedMsg when done.
// GIF URLs are decoded and encoded frame-by-frame so the modal can animate.
//
// The target display box is a.imageScale multiplied against the image's own
// native size in cells (imgview.NativeCellBox), not a fraction of the
// terminal window — deliberately, so a scale change always has a
// proportional, visible effect: a box expressed as "80% of the terminal"
// mostly went unnoticed for typical post images, which are usually much
// smaller than that box to begin with, so fitBox's never-upscale cap
// silently absorbed every "+" press and "-" needed many presses before
// crossing below the image's own native size (see
// docs/46-image-modal-scale.md). Upscaling past native size is allowed here
// (encoders called with allowUpscale=true) — unlike inline thumbnails, this
// is a user-driven zoom the caller explicitly asked for.
//
// width is clamped through a.layout.ModalMaxWidth rather than raw a.width:
// the modal is centered against the full terminal width by
// compositeOverlays regardless of layout, so in Miller layout a box wide
// enough to approach a.width would have its left edge splice into the nav
// sidebar — see ModalMaxWidth's doc comment. Both width and height are then
// additionally capped at modalScreenMarginFrac of the terminal size — see
// its doc comment for why the modal must stay well clear of the real screen
// edges, not just merely within them.
func (a App) openImageInTerminal(rawURL string) (App, tea.Cmd) {
	proto := a.graphicsProtocol
	ditherOpts := a.ditherOptions()
	scale := a.imageScale
	if scale <= 0 {
		scale = 1.0
	}
	width := min(a.layout.ModalMaxWidth(a.width), int(float64(a.width)*modalScreenMarginFrac))
	height := min(a.height-2, int(float64(a.height)*modalScreenMarginFrac)) // reserve 2 rows for the modal border
	a.imageFetchGen++
	gen := a.imageFetchGen
	cached, hit := a.imageCache[rawURL]
	return a, func() tea.Msg {
		frames := cached.frames
		delays := cached.delays
		if !hit {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var err error
			frames, delays, err = imgview.FetchAny(ctx, rawURL)
			if err != nil {
				return imageFetchedMsg{rawURL: rawURL, gen: gen, err: err}
			}
		}
		cellW, cellH, _ := imgview.TerminalCellPixelSize(int(os.Stdout.Fd()))
		nativeBounds := frames[0].Bounds()
		nativeCols, nativeRows := imgview.NativeCellBox(nativeBounds.Dx(), nativeBounds.Dy(), cellW, cellH)
		displayCols := int(float64(nativeCols) * scale)
		displayRows := int(float64(nativeRows) * scale)
		if displayCols > width {
			displayCols = width
		}
		if displayRows > height {
			displayRows = height
		}
		if displayCols < 1 {
			displayCols = 1
		}
		if displayRows < 1 {
			displayRows = 1
		}
		encodedFrames := make([]string, len(frames))
		var cols, rows int
		for i, img := range frames {
			switch proto {
			case imgview.ProtocolKitty:
				var err error
				encodedFrames[i], cols, rows, err = imgview.EncodeKitty(img, displayCols, displayRows, cellW, cellH, kittyModalPlacementID, true, ditherOpts)
				if err != nil {
					return imageFetchedMsg{rawURL: rawURL, gen: gen, err: err}
				}
			case imgview.ProtocolITerm2:
				var err error
				encodedFrames[i], cols, rows, err = imgview.EncodeITerm2(img, displayCols, displayRows, cellW, cellH, true, ditherOpts)
				if err != nil {
					return imageFetchedMsg{rawURL: rawURL, gen: gen, err: err}
				}
			case imgview.ProtocolSixel:
				var err error
				encodedFrames[i], cols, rows, err = imgview.EncodeSixel(img, displayCols, displayRows, cellW, cellH, true, ditherOpts)
				if err != nil {
					return imageFetchedMsg{rawURL: rawURL, gen: gen, err: err}
				}
			default:
				return imageFetchedMsg{rawURL: rawURL, gen: gen, err: fmt.Errorf("no graphics protocol")}
			}
		}
		return imageFetchedMsg{
			rawURL: rawURL, gen: gen,
			frames: frames, delays: delays,
			encodedFrames: encodedFrames, encoded: encodedFrames[0],
			cols: cols, rows: rows,
		}
	}
}

// handleImageViewer processes image fetch results. On success it opens the
// inline modal overlay; on failure it falls back to the browser, unless a
// carousel is already showing an image, in which case it just notifies and
// leaves the current image displayed rather than surprising the user with a
// browser tab mid-cycle.
func (a App) handleImageViewer(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case imageFetchedMsg:
		if m.gen != a.imageFetchGen {
			return a, nil, true // superseded by a later cycle or a close
		}
		if m.err != nil {
			if a.imageModalOpen {
				a2, cmd := a.notify(notifyInfo, "couldn't load image")
				return a2, cmd, true
			}
			return a, openExternalURL(m.rawURL), true
		}
		// wasOpen is true only for a genuine carousel cycle (the modal was
		// already open), not a fresh o-open or reopen-after-close, which
		// have no previous on-screen box to worry about.
		wasOpen := a.imageModalOpen
		// Snapshot the outgoing box size only for a genuine cycle.
		// compositeOverlays uses this, when the new image is a different
		// size, to force the previous box's row range dirty (iTerm2) or
		// trigger a full single-write repaint (Sixel) — see its doc comment.
		if wasOpen {
			a.imageModalPrevRows = a.imageModalRows
			a.imageModalPrevCols = a.imageModalCols
		} else {
			a.imageModalPrevRows = 0
			a.imageModalPrevCols = 0
		}
		// Bumped unconditionally, not just when the box size changed: the
		// "cycled" prev-box cleanup below still reads this same counter (see
		// compositeOverlays' doc comment), and compositeOverlays' modal
		// image-draw line also needs it on every change so it can use
		// imageDirtyMarker(a.imageRepaintGen) rather than relying only on the
		// payload bytes happening to differ — see imageDirtyMarker's doc
		// comment (App struct) for why a fixed/absent marker isn't enough.
		a.imageRepaintGen++
		a.imageModalEncoded = m.encoded
		a.imageModalCols = m.cols
		a.imageModalRows = m.rows
		a.imageModalURL = m.rawURL
		a.imageModalOpen = true
		a.chatrooms = a.chatrooms.SetAnimPaused(true)
		a.cmail = a.cmail.SetAnimPaused(true)
		a.imageNeedsCleanup = false
		if a.imageCache == nil {
			a.imageCache = make(map[string]cachedImage)
		}
		a.imageCache[m.rawURL] = cachedImage{frames: m.frames, delays: m.delays}
		var cmds []tea.Cmd
		if len(m.encodedFrames) > 1 {
			cmds = append(cmds, gifFrameTickCmd(m.encodedFrames, m.delays, 1, m.delays[0], m.gen))
		}
		return a, tea.Batch(cmds...), true
	case gifFrameTickMsg:
		if m.gen != a.imageFetchGen {
			return a, nil, true // modal closed, cycled, or replaced since this tick was scheduled
		}
		a.imageModalEncoded = m.encodedFrames[m.idx]
		nextIdx := (m.idx + 1) % len(m.encodedFrames)
		return a, gifFrameTickCmd(m.encodedFrames, m.delays, nextIdx, m.delays[m.idx], m.gen), true
	case carouselCycleSettledMsg:
		if m.gen != a.carouselCycleGen {
			return a, nil, true // superseded by a later left/right press
		}
		a2, cmd := a.openImageInTerminal(a.imageCarouselItems[a.imageCarouselIndex])
		return a2, cmd, true
	}
	return a, nil, false
}

// carouselCycleDebounce bounds how long cycleImageCarousel waits after the
// last left/right press before actually starting a fetch — see
// carouselCycleGen's doc comment (App struct) for why. 300ms rather than the
// original 120ms: live-reported on real iTerm2, fast/held cycling (but never
// slow deliberate presses) occasionally leaves the modal black. No erase is
// ever sent on the iTerm2 path (confirmed — the only tea.ClearScreen/
// EraseEntireScreen in this file are Sixel-gated), so this isn't a Bubble
// Tea diff/flush bug; the most plausible mechanism is the terminal itself —
// iTerm2 decoding one large, unchunked base64 OSC 1337 payload per image
// with no pacing, and the next debounced write landing before it's finished.
// A larger debounce gives it more real wall-clock headroom between actual
// image writes under sustained key-repeat. This is a mitigation for a
// terminal-side timing constraint this app can't measure directly, not a
// structural fix — revisit if it's still reported after this change.
const carouselCycleDebounce = 300 * time.Millisecond

// inlineImageSwitchSettleDelay bounds how long injectInlineImages
// (layout.go) holds back inline image draws after a screen switch — see
// App.screenSwitchedAt's doc comment for the live-log evidence this
// responds to. Between carouselCycleDebounce (300ms) and
// inlineImageStaleGrace (500ms): long enough to give the terminal real
// headroom after a large screen-redraw write, short enough that the delay
// before an image reappears on switching to its screen stays barely
// perceptible. A mitigation for a suspected terminal-side timing
// constraint this app can't measure directly, not a structural fix —
// revisit if it's still reported after this change.
const inlineImageSwitchSettleDelay = 250 * time.Millisecond

// carouselCycleSettledMsg fires carouselCycleDebounce after a left/right
// carousel press; only acted on if gen still matches a.carouselCycleGen
// (handleImageViewer), i.e. no further press has happened since.
type carouselCycleSettledMsg struct{ gen int }

// cycleImageCarousel moves to the next/prev image in imageCarouselItems
// (wrapping around) — the counter/displayed index updates immediately, so
// holding the key down feels responsive — but doesn't start fetching it
// directly. See carouselCycleGen's doc comment (App struct): fetching is
// real, non-trivial work (openImageInTerminal decodes+encodes even on an
// image-bytes cache hit), so firing one per keypress while a key is held
// wastes almost all of it on results imageFetchGen's newest-wins guard
// would just discard anyway. Debouncing via carouselCycleDebounce means a
// held key only ever fetches the image the user actually lands on.
func (a App) cycleImageCarousel(delta int) (App, tea.Cmd) {
	n := len(a.imageCarouselItems)
	a.imageCarouselIndex = (a.imageCarouselIndex + delta + n) % n
	a.carouselCycleGen++
	gen := a.carouselCycleGen
	return a, tea.Tick(carouselCycleDebounce, func(time.Time) tea.Msg {
		return carouselCycleSettledMsg{gen: gen}
	})
}

// handleURLPickerKey processes keyboard input while the URL picker overlay is open.
func (a App) handleURLPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(a.urlPickerItems)
	switch msg.String() {
	case "up", "k":
		a.urlPickerCursor = (a.urlPickerCursor - 1 + n) % n
	case "down", "j":
		a.urlPickerCursor = (a.urlPickerCursor + 1) % n
	case "enter":
		u := a.urlPickerItems[a.urlPickerCursor]
		if a.canRenderImageInline(u) {
			var images []string
			idx := 0
			for _, item := range a.urlPickerItems {
				if a.canRenderImageInline(item) {
					if item == u {
						idx = len(images)
					}
					images = append(images, item)
				}
			}
			a.urlPickerOpen = false
			a.urlPickerItems = nil
			if len(images) > 1 {
				a.imageCarouselItems = images
				a.imageCarouselIndex = idx
			}
			return a.openImageInTerminal(u)
		}
		a.urlPickerOpen = false
		a.urlPickerItems = nil
		return a.routeURL(u)
	case "esc":
		a.urlPickerOpen = false
		a.urlPickerItems = nil
	}
	return a, nil
}

// handleIconPickerKey processes keyboard input while the icon picker overlay
// is open. Esc, enter, and ctrl+n are handled here directly (mirrors
// handleURLPickerKey); enter and ctrl+n both emit an InsertIconMsg that
// falls through to delegateScreenUpdate and lands on whichever screen is
// currently active — enter also closes the picker, ctrl+n leaves it open so
// several icons can be inserted in a row. Every other key (tab/shift+tab/
// up/down/search typing) is forwarded to IconPickerModel.Update.
func (a App) handleIconPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.iconPickerOpen = false
		return a, nil
	case "enter":
		glyph, ok := a.iconPicker.Selected()
		if !ok {
			return a, nil
		}
		a.iconPickerOpen = false
		return a, func() tea.Msg { return screens.InsertIconMsg{Icon: glyph} }
	case "ctrl+n":
		// Same as enter, but leaves the picker open so several icons can be
		// inserted in a row without reopening it each time.
		glyph, ok := a.iconPicker.Selected()
		if !ok {
			return a, nil
		}
		return a, func() tea.Msg { return screens.InsertIconMsg{Icon: glyph} }
	}
	var cmd tea.Cmd
	a.iconPicker, cmd = a.iconPicker.Update(msg)
	return a, cmd
}

// handleAttachURLPromptKey processes keyboard input while the attach-image-URL
// prompt is open. Enter/esc are handled directly here (mirrors
// handleIconPickerKey) rather than routed through
// PathPromptSubmitMsg/PathPromptCancelMsg — those are already claimed by the
// theme export/import prompt's handlePathPrompt, which switches purely on
// message type, so reusing them here would misroute into the export/import
// logic. Every other key (typing, cursor movement) is forwarded to
// PathPromptModel.Update.
func (a App) handleAttachURLPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.attachURLPromptOpen = false
		return a, nil
	case "enter":
		url := strings.TrimSpace(a.attachURLPrompt.Value())
		if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			a.attachURLPrompt = a.attachURLPrompt.SetWarning("must be an http(s) URL")
			return a, nil
		}
		a.attachURLPromptOpen = false
		return a.applyAttachURL(url)
	}
	var cmd tea.Cmd
	a.attachURLPrompt, cmd = a.attachURLPrompt.Update(msg)
	return a, cmd
}

// applyAttachURL dispatches a submitted attach-URL prompt result, per what
// was confirmed live this session: posts get a native attachment (routed
// through the create/edit request); circ/C-Mail can only embed an animated
// GIF, via the server's own /gif command — visually confirmed on the
// website, whereas markdown in a message's content does not render there;
// and replies have no attachment mechanism at all (no attachments field in
// the reply API). Where nothing can actually render, this warns instead of
// silently inserting text that looks like it worked but won't show up
// anywhere but cyber-tui's own inline-image view.
func (a App) applyAttachURL(url string) (App, tea.Cmd) {
	switch {
	case a.active == screenFeed && a.feed.PanelActive():
		a.feed = a.feed.SetPanelAttachment(url)
		return a, nil
	case a.active == screenPostDetail && a.postDetail.EditPanelActive():
		a.postDetail = a.postDetail.SetEditPanelAttachment(url)
		return a, nil
	}
	if url == "" {
		return a, nil
	}
	switch {
	case a.active == screenChatrooms, a.active == screenCMail:
		if !urlutil.IsGIFURL(url) {
			return a.notify(notifyWarn, "cyberspace.online can only embed a GIF in chat (via /gif) — a static image needs a file upload the API doesn't support")
		}
		return a, func() tea.Msg { return screens.SetComposeValueMsg{Value: "/gif " + url} }
	case a.active == screenPostDetail && a.postDetail.ReplyComposeActive():
		return a.notify(notifyWarn, "replies don't support image attachments — there's no attachments field in the reply API")
	}
	return a, func() tea.Msg { return screens.InsertIconMsg{Icon: "![](" + url + ")"} }
}

// handleSongPromptKey processes keyboard input while the ctrl+j song-attach
// prompt is open. Esc/tab/shift+tab/ctrl+s are handled directly here; Enter's
// behavior depends on which field has focus (mirrors the modal's own field
// order): on the url field it validates and kicks off the async oEmbed
// fetch (see fetchSongMetadataCmd), on artist/title it just advances focus
// like tab, and on genre (the last field) it submits, same as ctrl+s. Every
// other key is forwarded to SongPromptModel.Update.
func (a App) handleSongPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.songPromptOpen = false
		return a, nil
	case "tab", "shift+tab":
		var cmd tea.Cmd
		a.songPrompt, cmd = a.songPrompt.Update(msg)
		return a, cmd
	case "ctrl+s":
		return a.submitSongPrompt()
	case "enter":
		if a.songPrompt.OnURLField() {
			url := a.songPrompt.URLValue()
			if _, ok := youtube.ExtractVideoID(url); !ok {
				a.songPrompt = a.songPrompt.SetWarning("not a recognized YouTube URL")
				return a, nil
			}
			var advanceCmd tea.Cmd
			a.songPrompt, advanceCmd = a.songPrompt.NextField()
			a.songPrompt = a.songPrompt.SetLoading(true)
			return a, tea.Batch(advanceCmd, a.fetchSongMetadataCmd(url))
		}
		if !a.songPrompt.OnLastField() {
			var cmd tea.Cmd
			a.songPrompt, cmd = a.songPrompt.NextField()
			return a, cmd
		}
		return a.submitSongPrompt()
	}
	var cmd tea.Cmd
	a.songPrompt, cmd = a.songPrompt.Update(msg)
	return a, cmd
}

// submitSongPrompt validates the song prompt's fields and, if valid, closes
// the modal and hands the built "/song ..." command to the chatrooms
// composer via SetComposeValueMsg — the same handoff applyAttachURL uses
// for ctrl+g's "/gif <url>". The user still presses Enter in the composer
// itself to actually send it.
func (a App) submitSongPrompt() (App, tea.Cmd) {
	cmd, ok := a.songPrompt.BuildCommand()
	if !ok {
		a.songPrompt = a.songPrompt.SetWarning("enter a YouTube URL, artist, and title")
		return a, nil
	}
	a.songPromptOpen = false
	return a, func() tea.Msg { return screens.SetComposeValueMsg{Value: cmd} }
}

// songMetadataFetchedMsg reports the result of one fetchSongMetadataCmd.
type songMetadataFetchedMsg struct {
	title  string
	artist string
	err    error
}

// fetchSongMetadataCmd looks up rawURL's title/channel via YouTube's public
// oEmbed endpoint (internal/youtube) — best-effort autofill, never blocking
// submission (see SongPromptModel.FetchFailed).
func (a App) fetchSongMetadataCmd(rawURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		title, artist, err := youtube.FetchMetadata(ctx, rawURL)
		return songMetadataFetchedMsg{title: title, artist: artist, err: err}
	}
}

// handleSongMetadataFetched applies a resolved songMetadataFetchedMsg to the
// still-open song prompt. If the prompt was closed (cancelled) before the
// fetch resolved, the result is simply dropped.
func (a App) handleSongMetadataFetched(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(songMetadataFetchedMsg)
	if !ok {
		return a, nil, false
	}
	if !a.songPromptOpen {
		return a, nil, true
	}
	if m.err != nil {
		a.songPrompt = a.songPrompt.FetchFailed()
	} else {
		a.songPrompt = a.songPrompt.ApplyMetadata(m.title, m.artist)
	}
	return a, nil, true
}

// --- commands ---

// loginSuccessMsg carries the authenticated session back to the update loop so
// App fields are set there rather than mutated from the command goroutine.
type loginSuccessMsg struct {
	tokens model.Tokens
	user   model.User
}

// loginErrMsgFor builds a LoginErrMsg from a login/profile-fetch failure,
// detecting EMAIL_NOT_VERIFIED so the login screen can offer a resend action
// using the already-issued idToken instead of showing a generic error. Login
// itself succeeds regardless of verification status (per the API docs, an
// idToken is required to call resend-verification, which only makes sense
// if login itself didn't already block on this) — the 403 shows up on the
// profile fetch that follows.
func loginErrMsgFor(err error, idToken string) screens.LoginErrMsg {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Code == "EMAIL_NOT_VERIFIED" {
		return screens.LoginErrMsg{Err: err, EmailNotVerified: true, IDToken: idToken}
	}
	return screens.LoginErrMsg{Err: err}
}

// resendVerificationCmd calls POST /v1/auth/resend-verification for idToken,
// obtained from a login that succeeded but hit EMAIL_NOT_VERIFIED on the
// follow-up profile fetch.
func (a *App) resendVerificationCmd(idToken string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		return screens.ResendVerificationResultMsg{Err: client.ResendVerification(idToken)}
	}
}

func (a *App) loginCmd(email, password string) tea.Cmd {
	return func() tea.Msg {
		tokens, err := a.client.Login(email, password)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		// Initialise the RTDB client using the URL returned by the API (best effort).
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.IDToken, tokens.RTDBUrl)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return loginErrMsgFor(err, tokens.IDToken)
		}
		// Wire the user ID into the HTTP client for RTDB path construction.
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		// Persist the refresh token so subsequent launches auto-login.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		a.saveConfig(func(cfg *config.Config) {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.Email = email
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
		})
		return loginSuccessMsg{tokens: tokens, user: user}
	}
}

// tokenLoginCmd resumes a saved session by exchanging the stored refresh token
// for fresh API tokens, then fetches the user profile. On failure it falls back
// to the login screen by returning a LoginErrMsg.
func (a *App) tokenLoginCmd(refreshToken string) tea.Cmd {
	return func() tea.Msg {
		tokens, err := a.client.LoginWithRefreshToken(refreshToken)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.IDToken, tokens.RTDBUrl)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return loginErrMsgFor(err, tokens.IDToken)
		}
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		// Update savedAt so we know when the session was last used.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		a.saveConfig(func(cfg *config.Config) {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
		})
		return loginSuccessMsg{tokens: tokens, user: user}
	}
}

func (a *App) afterLoginCmd() tea.Cmd {
	a.active = screenFeed
	a.profile = a.profile.SetUser(a.currentUser)
	a.feed = a.feed.SetCurrentUsername(a.currentUser.Username).SetCurrentUserIsSupporter(a.currentUser.IsSupporter)
	a.feed = a.feed.SetFetching()
	a.bookmarks = a.bookmarks.SetFetching()
	a.topics = a.topics.SetFetching()
	a.postDetail = a.postDetail.SetCurrentUsername(a.currentUser.Username).SetCurrentUserIsSupporter(a.currentUser.IsSupporter)
	a.broadcastConfig()
	// Conversation list has no REST seed — the live subscription's own first
	// event (a full snapshot, like chat_presence's) populates it. Seeding via
	// GetConversations first and then opening the subscription would leave
	// two independent writers to CMailModel.conversations (see
	// OpenUserConvsSubscription's doc comment).
	var cmailCmd tea.Cmd
	a.cmail, cmailCmd = a.cmail.OpenUserConvsSubscription()
	// Don't start the feed-poll tea.Tick chain at all when the user has
	// opted out — nothing else would revive it (see feedPollTickMsg and the
	// settingsSavedMsg manual→auto restart above).
	var feedPollCmd tea.Cmd
	if !a.feedManualRefreshOnly {
		feedPollCmd = a.scheduleFeedPollCmd()
	}
	return tea.Batch(
		a.loadFeedCmd(),
		a.loadBookmarksCmd(""),
		a.loadWatchesPageCmd(""),
		a.loadTopicsCmd(),
		a.loadProfileCmd(),
		cmailCmd,
		a.fetchUnreadCountCmd(),
		a.schedulePollCmd(),
		feedPollCmd,
		a.loadSettingsCmd(),
		a.scheduleWanderCmd(),
		a.checkAndWanderCmd(),
		a.scheduleLogoAnimCmd(),
		a.checkForUpdateCmd(),
		a.scheduleRelativeTimeTickCmd(),
	)
}

type feedLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type feedPageMsg struct {
	posts  []model.Post
	cursor string
}
type roomsLoadedMsg struct{ rooms []model.Room }

// roomCommandReplyMsg/cmailCommandReplyMsg carry a reply-only slash command's
// text (e.g. /help) back from the send response, for local display only —
// nothing was posted, so nothing arrives via the RTDB subscription.
type roomCommandReplyMsg struct {
	roomID string
	reply  string
}
type roomMessageDeletedMsg struct{ messageID string }

// userMutedMsg confirms a successful /mute send — needed because, per the
// server ("/mute ... posts nothing"), a successful mute has no visible
// effect in the room to tell the user it worked; see muteUserCmd.
type userMutedMsg struct{ username string }
type cmailCommandReplyMsg struct {
	convID string
	reply  string
}
type conversationStartedMsg struct{ conv model.Conversation }
type profileLoadedMsg struct{ user model.User }
type userProfileLoadedMsg struct {
	user        model.User
	isFollowing bool
	followID    string
}

// cmailOtherProfileLoadedMsg carries a C-Mail conversation partner's full
// profile, fetched (see loadCMailOtherProfileCmd) so their real
// SupporterIcon is available for the detail header badge — conversation
// data alone never carries it.
type cmailOtherProfileLoadedMsg struct {
	username string
	user     model.User
}
type followResultMsg struct{ followID string }
type unfollowResultMsg struct{}

// repliesLoadedMsg carries postID so a request superseded by the user
// navigating to a different post before it resolves can be detected and
// dropped instead of silently overwriting the now-current post's reply tree
// — see loadRepliesCmd and its handler in handlePostDetail.
type repliesLoadedMsg struct {
	postID  string
	replies []model.Reply
}
type replyCreatedMsg struct{ postID, replyID string }
type replyDeletedMsg struct{ replyID string }
type postCreatedMsg struct{}

// postConvertedToNoteMsg is returned instead of postCreatedMsg when the server
// silently turned a too-soon post into a journal entry — see createPostCmd.
type postConvertedToNoteMsg struct{}
type postDeletedMsg struct {
	postID   string
	fromFeed bool // true = delete was triggered from the feed; false = from post detail
}

// postEditedMsg carries the fields submitted in a successful post edit. The
// PATCH response echoes back no fields worth using (same as CreatePost), so
// editedAt is set optimistically to the time the request completed.
type postEditedMsg struct {
	postID           string
	content, title   string
	topics           []string
	isPublic, isNSFW bool
	editedAt         time.Time
	// attachments/attachmentsTouched mirror EditPost's own pair: Feed/
	// PostDetail's local copy only overwrites its cached Attachments when
	// attachmentsTouched — otherwise the edit didn't touch them server-side
	// either, so the stale local copy is already correct.
	attachments        []model.Attachment
	attachmentsTouched bool
}

type replyEditedMsg struct {
	replyID, content string
	editedAt         time.Time
}
type settingsLoadedMsg struct{ settings model.Settings }
type settingsSavedMsg struct {
	seq                     int
	settings                model.Settings
	wanderLust              bool
	feedManualRefreshOnly   bool
	typingIndicatorsEnabled bool
	maxThreadDepth          int
	timezone                string
	imageViewer             string
	graphicsProtocol        string
	inlineImages            bool
	dithering               bool
	ditherSharpness         string
	layoutName              string
}
type wanderTickMsg struct{ gen int }
type wanderDoneMsg struct{ at time.Time } // zero At means no update was made
type updateAvailableMsg struct{ tag, url string }
type errMsg struct{ err error }

// notifPostLoadErrMsg is the failure of opening a post from the Notifications
// screen. It is handled in handleNotifications so a deleted target surfaces as a
// friendly transient banner ("This post has been deleted") instead of routing
// through handleErr and blanking the list.
type notifPostLoadErrMsg struct{ err error }

// urlPostLoadErrMsg is the failure of opening a post permalink URL (see
// routeURL). Handled the same way as notifPostLoadErrMsg — a friendly
// transient banner rather than routing through handleErr.
type urlPostLoadErrMsg struct{ err error }

// imageFetchedMsg carries the result of fetching and encoding an image for
// terminal display. err is non-nil when the download or decode failed; rawURL
// is retained so a failed decode can fall back to opening the browser. frames
// holds every decoded frame (len 1 for a static image); encodedFrames holds
// each frame pre-encoded for the current graphics protocol, with encoded ==
// encodedFrames[0] kept for the existing single-frame call sites.
type imageFetchedMsg struct {
	rawURL        string
	gen           int
	frames        []image.Image
	delays        []time.Duration
	encoded       string
	encodedFrames []string
	cols          int
	rows          int
	err           error
}

// cachedImage holds a fetched/composited image's frames, keyed by URL in
// imageCache. A static image has len(frames) == 1 and nil delays.
type cachedImage struct {
	frames []image.Image
	delays []time.Duration
}

// gifFrameTickMsg advances the open image modal to its next pre-encoded GIF
// frame. gen must match the current imageFetchGen or the tick is dropped —
// this is how closing the modal, cycling the carousel, or opening a new
// image stops an in-flight GIF animation with no extra bookkeeping (mirrors
// the imageFetchedMsg staleness guard above).
type gifFrameTickMsg struct {
	gen           int
	encodedFrames []string
	delays        []time.Duration
	idx           int
}

// gifFrameTickCmd schedules the display of encodedFrames[idx] after delay.
func gifFrameTickCmd(encodedFrames []string, delays []time.Duration, idx int, delay time.Duration, gen int) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return gifFrameTickMsg{gen: gen, encodedFrames: encodedFrames, delays: delays, idx: idx}
	})
}

// notifyLevel selects the color of a global notification banner.
type notifyLevel int

const (
	notifyInfo notifyLevel = iota
	notifyWarn
	notifyError
)

type logoAnimPhase int

const (
	logoPhaseIdle logoAnimPhase = iota
	logoPhaseScrambling
	logoPhaseHold
	logoPhaseUnscrambling
)

// actionErrMsg is a non-fatal failure from a user-initiated action (post, reply,
// delete, follow, …). Like errMsg it surfaces as a transient global banner and
// never blocks a tab; unlike errMsg it does not set any screen's inline
// "couldn't load" empty-state, since there is no load in flight.
type actionErrMsg struct{ err error }

// notifyMsg sets the global banner directly; used for success/info surfacing.
type notifyMsg struct {
	text  string
	level notifyLevel
}

// notifyExpireMsg clears the banner iff gen still matches a.notifyGen.
type notifyExpireMsg struct{ gen int }
type bookmarksLoadedMsg struct {
	items  []model.Bookmark
	cursor string
}
type bookmarksPageMsg struct {
	items  []model.Bookmark
	cursor string
}
type bookmarkCreatedMsg struct {
	bookmarkID string
	postID     string
	replyID    string
	err        error
}
type bookmarkDeletedMsg struct {
	bookmarkID          string
	fromBookmarksScreen bool
}
type bookmarkPostLoadedMsg struct{ post model.Post }
type bookmarkReplyLoadedMsg struct {
	post    model.Post
	replyID string
}

type journalLoadedMsg struct {
	notes  []model.Note
	cursor string
}
type journalPageMsg struct {
	notes  []model.Note
	cursor string
}
type noteCreatedMsg struct{ note model.Note }
type noteUpdatedMsg struct {
	noteID  string
	content string
	topics  []string
}
type noteDeletedMsg struct{ noteID string }
type notePublishedMsg struct{}

// --- profile sub-tab result messages ---

type userPostsLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type userPostsPageMsg struct {
	posts  []model.Post
	cursor string
}
type userRepliesLoadedMsg struct {
	replies []model.Reply
	cursor  string
}
type userRepliesPageMsg struct {
	replies []model.Reply
	cursor  string
}
type userFollowingLoadedMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowingPageMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowersLoadedMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowersPageMsg struct {
	follows []model.Follow
	cursor  string
}

// --- note revision result messages ---

type noteRevisionsLoadedMsg struct {
	noteID    string
	revisions []model.NoteRevision
	cursor    string
}
type noteRevisionPreviewMsg struct{ note model.Note }

type topicsLoadedMsg struct {
	topics []model.Topic
	cursor string
}
type topicsPageMsg struct {
	topics []model.Topic
	cursor string
}
type topicPostsLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type topicPostsPageMsg struct {
	posts  []model.Post
	cursor string
}

type searchPreviewLoadedMsg struct {
	preview model.SearchPreview
	query   string
}

// searchTypeLoadedMsg/searchTypePageMsg carry one paginated search category's
// results. Exactly one of posts/replies/users is populated, matching hitType.
type searchTypeLoadedMsg struct {
	hitType string
	posts   []model.Post
	replies []model.Reply
	users   []model.User
	cursor  string
}

type searchTypePageMsg struct {
	hitType string
	posts   []model.Post
	replies []model.Reply
	users   []model.User
	cursor  string
}

type guildsLoadedMsg struct {
	guilds []model.Guild
	cursor string
}
type ownGuildInjectMsg struct{ guild model.Guild }
type guildsPageMsg struct {
	guilds []model.Guild
	cursor string
}
type guildPostsLoadedMsg struct {
	slug   string
	posts  []model.Post
	cursor string
}
type guildPostsPageMsg struct {
	slug   string
	posts  []model.Post
	cursor string
}
type guildPostCreatedMsg struct{ slug string }
type guildMembersLoadedMsg struct {
	members []model.GuildMember
	cursor  string
}
type guildMembersPageMsg struct {
	members []model.GuildMember
	cursor  string
}
type guildDetailLoadedMsg struct{ guild model.Guild }
type guildJoinedMsg struct{ slug, name, role string }
type guildLeftMsg struct{ slug, name string }
type guildPromotedMsg struct{ slug, name, role string }

type notifsLoadedMsg struct {
	notifs []model.Notification
	cursor string
}
type notifsPageMsg struct {
	notifs []model.Notification
	cursor string
}
type notifPostLoadedMsg struct{ post model.Post }

// urlPostLoadedMsg carries the result of opening a post permalink URL (see
// routeURL). origin travels with the message rather than being read from
// a.active on arrival, since a.active can change while the fetch is in flight.
type urlPostLoadedMsg struct {
	post   model.Post
	origin screen
}
type profilePostLoadedMsg struct{ post model.Post }
type pollUnreadTickMsg struct{ gen int }

// relativeTimeTickMsg drives the periodic re-render that keeps cIRC/C-Mail's
// "Nm ago"-style timestamps current — see
// screens.ChatroomsModel.RefreshRelativeTimestamps.
type relativeTimeTickMsg struct{ gen int }
type unreadCountMsg struct {
	count int
	exact bool
}
type feedPollTickMsg struct{ gen int }
type feedPeekMsg struct{ posts []model.Post }
type logoAnimTickMsg struct{ gen int } // 30s idle trigger — begins the scramble animation
type logoFrameTickMsg struct{}         // 60ms per-frame tick during scramble/hold/unscramble

func (a *App) loadFeedCmd() tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetFeed("")
		if err != nil {
			return errMsg{err}
		}
		return feedLoadedMsg{posts: posts, cursor: cursor}
	}
}

// fetchFeedPeekCmd fetches the newest page of the feed for the background
// poll to diff against the currently loaded posts. Errors are swallowed
// (nil msg) — a missed poll just tries again on the next tick.
func (a *App) fetchFeedPeekCmd() tea.Cmd {
	return func() tea.Msg {
		posts, _, err := a.client.GetFeed("")
		if err != nil {
			return nil
		}
		return feedPeekMsg{posts: posts}
	}
}

// feedPollInterval is how often the background poll (fetchFeedPeekCmd)
// checks for new posts — see docs/39-feed-background-poll.md. Matches the
// notifications unread-count poll's cadence (schedulePollCmd).
const feedPollInterval = 60 * time.Second

func (a *App) scheduleFeedPollCmd() tea.Cmd {
	gen := a.sessionGen
	return tea.Tick(feedPollInterval, func(time.Time) tea.Msg { return feedPollTickMsg{gen: gen} })
}

func (a *App) loadFeedPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetFeed(cursor)
		if err != nil {
			return errMsg{err}
		}
		return feedPageMsg{posts: posts, cursor: nextCursor}
	}
}

func (a *App) loadRoomsCmd() tea.Cmd {
	return func() tea.Msg {
		rooms, err := a.client.GetRooms()
		if err != nil {
			return errMsg{err}
		}
		return roomsLoadedMsg{rooms}
	}
}

func (a *App) loadSettingsCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := a.client.GetSettings()
		if err != nil {
			return errMsg{err}
		}
		return settingsLoadedMsg{s}
	}
}

func (a *App) loadProfileCmd() tea.Cmd {
	return func() tea.Msg {
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return errMsg{err}
		}
		return profileLoadedMsg{user}
	}
}

func (a *App) loadUserProfileCmd(username string) tea.Cmd {
	return func() tea.Msg {
		// Skip the API call if this is the logged-in user's own profile.
		if username == a.currentUser.Username {
			return userProfileLoadedMsg{user: a.currentUser}
		}
		user, err := a.client.GetProfile(username)
		if err != nil {
			return errMsg{err}
		}
		// Detect whether the logged-in user follows this profile by scanning
		// the first page of the following list (up to 50 entries).
		var isFollowing bool
		var followID string
		follows, _, err := a.client.GetFollowing("")
		if err == nil {
			for _, f := range follows {
				if f.FollowedID == user.ID {
					isFollowing = true
					followID = f.ID
					break
				}
			}
		}
		return userProfileLoadedMsg{user: user, isFollowing: isFollowing, followID: followID}
	}
}

// loadCMailOtherProfileCmd fetches username's full profile so the C-Mail
// detail header can show their real SupporterIcon (see
// CMailConvSelectedMsg's doc comment — conversation data never carries it).
// A failure just means the badge silently stays absent; that's an
// acceptable outcome here, not worth a user-facing error toast.
func (a *App) loadCMailOtherProfileCmd(username string) tea.Cmd {
	return func() tea.Msg {
		user, err := a.client.GetProfile(username)
		if err != nil {
			return nil
		}
		return cmailOtherProfileLoadedMsg{username: username, user: user}
	}
}

// userGuildsLoadedMsg delivers a profile's apprenticeship list. Fetch errors
// are non-fatal — the profile still renders without the apprenticeships row.
type userGuildsLoadedMsg struct {
	username string
	guilds   []model.GuildMembership
}

// apprenticeSlugsFrom extracts the apprentice-role guild slugs from a user's
// GetUserGuilds result, used to order the Guilds tab.
func apprenticeSlugsFrom(memberships []model.GuildMembership) []string {
	var slugs []string
	for _, gm := range memberships {
		if gm.Role == "apprentice" {
			slugs = append(slugs, gm.Slug)
		}
	}
	return slugs
}

func (a *App) loadUserGuildsCmd(username string) tea.Cmd {
	return func() tea.Msg {
		guilds, err := a.client.GetUserGuilds(username)
		if err != nil {
			return userGuildsLoadedMsg{username: username}
		}
		return userGuildsLoadedMsg{username: username, guilds: guilds}
	}
}

func (a *App) followUserCmd(userID string) tea.Cmd {
	return func() tea.Msg {
		followID, err := a.client.Follow(userID)
		if err != nil {
			return actionErrMsg{err}
		}
		return followResultMsg{followID: followID}
	}
}

func (a *App) unfollowUserCmd(followID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.Unfollow(followID); err != nil {
			return actionErrMsg{err}
		}
		return unfollowResultMsg{}
	}
}

func (a *App) pokeUserCmd(username string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.Poke(username); err != nil {
			return pokeErrorMsg(err)
		}
		return notifyMsg{level: notifyInfo, text: "poked @" + username}
	}
}

// pokeErrorMsg converts a poke-action error into the message to emit. A 429
// is expected here far more than on other actions (1/hour, 8/day cap across
// all users) so it gets a friendly banner instead of the raw API error text;
// a 403 (blocked either direction) also gets a friendlier message. Anything
// else falls through to actionErrMsg's normal handling.
func pokeErrorMsg(err error) tea.Msg {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case 429:
			return notifyMsg{level: notifyError, text: "poke limit reached — try again later"}
		case 403:
			return notifyMsg{level: notifyError, text: "can't poke this user"}
		}
	}
	return actionErrMsg{err}
}

func (a *App) loadUserPostsCmd(username, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, next, err := a.client.GetUserPosts(username, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userPostsLoadedMsg{posts: posts, cursor: next}
		}
		return userPostsPageMsg{posts: posts, cursor: next}
	}
}

func (a *App) loadUserRepliesCmd(username, cursor string) tea.Cmd {
	return func() tea.Msg {
		replies, next, err := a.client.GetUserReplies(username, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userRepliesLoadedMsg{replies: replies, cursor: next}
		}
		return userRepliesPageMsg{replies: replies, cursor: next}
	}
}

func (a *App) loadUserFollowingCmd(userID, cursor string) tea.Cmd {
	return func() tea.Msg {
		follows, next, err := a.client.GetUserFollows(userID, "following", cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userFollowingLoadedMsg{follows: follows, cursor: next}
		}
		return userFollowingPageMsg{follows: follows, cursor: next}
	}
}

func (a *App) loadUserFollowersCmd(userID, cursor string) tea.Cmd {
	return func() tea.Msg {
		follows, next, err := a.client.GetUserFollows(userID, "followers", cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userFollowersLoadedMsg{follows: follows, cursor: next}
		}
		return userFollowersPageMsg{follows: follows, cursor: next}
	}
}

func (a *App) sendRoomMessageCmd(roomID, body string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.SendRoomMessage(roomID, body)
		if err != nil {
			return actionErrMsg{err}
		}
		if reply != "" {
			return roomCommandReplyMsg{roomID: roomID, reply: reply}
		}
		return nil
	}
}

// muteUserCmd sends "/mute <username>" as a normal room message — the only
// way to mute is server-side, via the same slash command a user would type
// by hand (see docs/00-latest-api-reference.md's Commands section). Unlike
// sendRoomMessageCmd, success always reports back (userMutedMsg) since the
// command posts nothing to the room for the user to see otherwise.
func (a *App) muteUserCmd(roomID, username string) tea.Cmd {
	return func() tea.Msg {
		if _, err := a.client.SendRoomMessage(roomID, "/mute "+username); err != nil {
			return actionErrMsg{err}
		}
		return userMutedMsg{username: username}
	}
}

func (a *App) markRoomReadCmd(roomID string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkRoomRead(roomID) // fire-and-forget
		return nil
	}
}

func (a *App) sendCMailCmd(convID, body string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.SendMessage(convID, body)
		if err != nil {
			return actionErrMsg{err}
		}
		if reply != "" {
			return cmailCommandReplyMsg{convID: convID, reply: reply}
		}
		return nil
	}
}

func (a App) startConversationCmd(username string) tea.Cmd {
	return func() tea.Msg {
		conv, err := a.client.StartConversation(username)
		if err != nil {
			return actionErrMsg{err}
		}
		return conversationStartedMsg{conv: conv}
	}
}

func (a *App) markCMailReadCmd(convID string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkCMailRead(convID)
		return nil
	}
}

func (a *App) loadRepliesCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return errMsg{err}
		}
		return repliesLoadedMsg{postID: postID, replies: replies}
	}
}

func (a *App) loadFeedDetailCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.FeedDetailRepliesMsg{PostID: postID}
		}
		return screens.FeedDetailRepliesMsg{PostID: postID, Replies: replies}
	}
}

func (a *App) loadGuildThreadCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.GuildThreadRepliesMsg{PostID: postID}
		}
		return screens.GuildThreadRepliesMsg{PostID: postID, Replies: replies}
	}
}

func (a *App) loadTopicThreadCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.TopicThreadRepliesMsg{PostID: postID}
		}
		return screens.TopicThreadRepliesMsg{PostID: postID, Replies: replies}
	}
}

func (a *App) createReplyCmd(postID, content, parentReplyID string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.CreateReply(postID, content, parentReplyID)
		if err != nil {
			return actionErrMsg{err}
		}
		return replyCreatedMsg{postID: postID, replyID: reply.ID}
	}
}

// maxAttachmentDim is the width/height cap cyberspace.online enforces on a
// post's native image/gif attachment — confirmed live (POST /v1/posts 400s
// above it: "Image width must be an integer between 1 and 640px"). The API
// requires the client to supply both dimensions and does not compute or
// resize the image itself.
const maxAttachmentDim = 640

// attachmentTypeForURL infers a wireAttachment's "type" from the URL's path
// extension (via urlutil.IsGIFURL — ignores query string/fragment, unlike a
// raw suffix check, so a signed CDN link like ".gif?token=..." still
// resolves correctly). This only covers native post/reply attachments,
// which are always this client's own upload URLs with a known extension;
// circ/C-Mail's /gif <url> accepts arbitrary user-typed URLs instead, so
// openImageInTerminal no longer relies on extension sniffing — see
// imgview.FetchAny.
func attachmentTypeForURL(rawURL string) string {
	if urlutil.IsGIFURL(rawURL) {
		return "gif"
	}
	return "image"
}

// resolveAttachment fetches attachmentURL's declared dimensions and builds
// the model.Attachment CreatePost/EditPost send. Returns nil, nil for an
// empty URL (no attachment). Returns an error — surfaced to the user like any
// other actionErrMsg/editErrorMsg — when the image can't be fetched or
// exceeds maxAttachmentDim; this app has no way to resize it (no upload
// endpoint to host the result), so the accurate move is to fail clearly here
// rather than let the server reject a request the user already spent an
// interaction composing.
func resolveAttachment(attachmentURL string) (*model.Attachment, error) {
	if attachmentURL == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	width, height, err := imgview.Dimensions(ctx, attachmentURL)
	if err != nil {
		return nil, fmt.Errorf("attach image: %w", err)
	}
	if width > maxAttachmentDim || height > maxAttachmentDim {
		return nil, fmt.Errorf("attach image: %dx%d exceeds the site's %dx%d limit for post attachments — try a smaller image, or paste the link in the body text instead", width, height, maxAttachmentDim, maxAttachmentDim)
	}
	return &model.Attachment{Type: attachmentTypeForURL(attachmentURL), Src: attachmentURL, Width: width, Height: height}, nil
}

func (a *App) createPostCmd(content, title, slug string, topics []string, isPublic, isNSFW bool, attachmentURL string) tea.Cmd {
	return func() tea.Msg {
		attachment, err := resolveAttachment(attachmentURL)
		if err != nil {
			return actionErrMsg{err}
		}
		post, err := a.client.CreatePost(content, title, slug, topics, isPublic, isNSFW, attachment)
		if err != nil {
			return actionErrMsg{err}
		}
		// The server silently converts a post submitted too soon after a
		// previous one into a journal entry instead of rejecting it — the
		// response still looks like a normal success (postId/slug), but the
		// post doesn't actually exist. No notification is ever generated for
		// this server-side, so the only way to detect it client-side is to
		// check whether the returned ID resolves.
		if _, err := a.client.GetPost(post.ID); err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && apiErr.Status == 404 {
				return postConvertedToNoteMsg{}
			}
		}
		return postCreatedMsg{}
	}
}

func (a *App) saveProfileCmd(msg screens.SaveProfileMsg) tea.Cmd {
	return func() tea.Msg {
		update := model.ProfileUpdate{
			Bio:          &msg.Bio,
			WebsiteName:  &msg.WebsiteName,
			LocationName: &msg.LocationName,
		}
		// URL fields: send only when non-empty — the API rejects empty strings
		// as invalid URLs. Leaving them nil means the existing value is unchanged.
		if msg.WebsiteUrl != "" {
			update.WebsiteUrl = &msg.WebsiteUrl
		}
		if msg.WebsiteImageUrl != "" {
			update.WebsiteImageUrl = &msg.WebsiteImageUrl
		}
		if msg.Latitude != "" {
			if lat, err := strconv.ParseFloat(msg.Latitude, 64); err == nil {
				update.LocationLatitude = &lat
			}
		}
		if msg.Longitude != "" {
			if lon, err := strconv.ParseFloat(msg.Longitude, 64); err == nil {
				update.LocationLongitude = &lon
			}
		}
		if err := a.client.UpdateProfile(update); err != nil {
			return actionErrMsg{err}
		}
		a.currentUser.Bio = msg.Bio
		a.currentUser.WebsiteName = msg.WebsiteName
		a.currentUser.WebsiteUrl = msg.WebsiteUrl
		a.currentUser.WebsiteImageUrl = msg.WebsiteImageUrl
		a.currentUser.LocationName = msg.LocationName
		if update.LocationLatitude != nil {
			a.currentUser.LocationLatitude = *update.LocationLatitude
		}
		if update.LocationLongitude != nil {
			a.currentUser.LocationLongitude = *update.LocationLongitude
		}
		return profileLoadedMsg{a.currentUser}
	}
}

// --- notifications ---

// handleNotifications processes notification load, mark-read, jump-to-post, and poll messages.
func (a App) handleNotifications(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case notifsLoadedMsg:
		a, cmd := a.suppressActiveRoomMentions(msg.notifs)
		a.notifications = a.notifications.SetNotifs(msg.notifs, msg.cursor)
		return a, cmd, true
	case notifsPageMsg:
		a, cmd := a.suppressActiveRoomMentions(msg.notifs)
		a.notifications = a.notifications.AppendNotifs(msg.notifs, msg.cursor)
		return a, cmd, true
	case screens.RefreshNotifsMsg:
		return a, a.loadNotifsCmd(), true
	case screens.LoadMoreNotifsMsg:
		return a, a.loadNotifsPageCmd(msg.Cursor), true
	case screens.MarkNotifReadMsg:
		// Optimistic update already applied in NotificationsModel.Update; fire API call.
		if a.polledUnreadCount > 0 {
			a.polledUnreadCount--
		}
		return a, a.markNotifReadCmd(msg.ID), true
	case screens.MarkAllNotifsReadMsg:
		// Optimistic update already applied in NotificationsModel.Update; fire API call.
		a.polledUnreadCount = 0
		a.polledUnreadCountExact = true
		return a, a.markAllNotifsReadCmd(), true
	case screens.ShowNotificationPostMsg:
		// Optimistic mark-read already applied in NotificationsModel.Update; confirm with API.
		if a.polledUnreadCount > 0 {
			a.polledUnreadCount--
		}
		a.pendingReplyID = msg.ReplyID
		return a, tea.Batch(a.markNotifReadCmd(msg.NotifID), a.loadPostAndShowCmd(msg.PostID)), true
	case notifPostLoadedMsg:
		a.postDetailReturn = screenNotifications
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, a.loadRepliesCmd(msg.post.ID), true
	case notifPostLoadErrMsg:
		// A dead session must still redirect to login — let handleUnauthorized
		// (which runs later in the dispatch chain) claim it.
		if errors.Is(msg.err, api.ErrUnauthorized) {
			return a, nil, false
		}
		// The target post is gone (or otherwise unfetchable): announce it in the
		// transient banner and leave the notifications list untouched.
		var apiErr *api.APIError
		if errors.As(msg.err, &apiErr) && apiErr.Status == 404 {
			a, cmd := a.notify(notifyWarn, "This post has been deleted")
			return a, cmd, true
		}
		a, cmd := a.notify(notifyError, msg.err.Error())
		return a, cmd, true
	case urlPostLoadedMsg:
		if a.active == screenPostDetail {
			// Already viewing a post and the link opened another one:
			// push the current post so Esc returns to it instead of
			// clobbering postDetailReturn with screenPostDetail itself.
			a.postDetailStack = append(a.postDetailStack, a.postDetail)
		} else {
			a.postDetailReturn = msg.origin
			a.postDetailStack = nil
		}
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, a.loadRepliesCmd(msg.post.ID), true
	case urlPostLoadErrMsg:
		if errors.Is(msg.err, api.ErrUnauthorized) {
			return a, nil, false
		}
		var apiErr *api.APIError
		if errors.As(msg.err, &apiErr) && apiErr.Status == 404 {
			a, cmd := a.notify(notifyWarn, "This post has been deleted")
			return a, cmd, true
		}
		a, cmd := a.notify(notifyError, msg.err.Error())
		return a, cmd, true
	case screens.ShowUserProfileMsg:
		if a.active != screenNotifications {
			return a, nil, false
		}
		a.profileReturn = screenNotifications
		return a, a.loadUserProfileCmd(msg.Username), true
	case pollUnreadTickMsg:
		// C-Mail's conversation list no longer polls here — it updates live via
		// the RTDB subscription opened in afterLoginCmd (see
		// OpenUserConvsSubscription). This ticker now only drives the
		// notifications unread-count badge.
		if msg.gen != a.sessionGen {
			return a, nil, true
		}
		return a, tea.Batch(a.fetchUnreadCountCmd(), a.schedulePollCmd()), true
	case unreadCountMsg:
		prev := a.polledUnreadCount
		a.polledUnreadCount = msg.count
		a.polledUnreadCountExact = msg.exact
		if msg.count > prev && !a.notifications.HasPaginated() {
			return a, a.loadNotifsCmd(), true
		}
		return a, nil, true
	case feedPollTickMsg:
		if msg.gen != a.sessionGen {
			return a, nil, true
		}
		if a.feedManualRefreshOnly {
			return a, nil, true // user disabled auto-refresh mid-session — chain dies here
		}
		if !a.feed.IsLoaded() || a.feed.IsRefreshing() {
			return a, a.scheduleFeedPollCmd(), true
		}
		return a, tea.Batch(a.fetchFeedPeekCmd(), a.scheduleFeedPollCmd()), true
	case feedPeekMsg:
		a.feed = a.feed.SetPendingNew(msg.posts)
		return a, nil, true
	}
	return a, nil, false
}

// suppressActiveRoomMentions marks read (locally + via API) any unread
// chat_mention notifications for the cIRC room the user currently has open,
// so being mentioned in a room you're already reading doesn't also notify.
func (a App) suppressActiveRoomMentions(notifs []model.Notification) (App, tea.Cmd) {
	roomSlug := a.chatrooms.ActiveRoomSlug()
	if a.active != screenChatrooms || roomSlug == "" {
		return a, nil
	}
	var cmds []tea.Cmd
	for i, n := range notifs {
		if n.Type == "chat_mention" && !n.Read && n.RoomSlug == roomSlug {
			notifs[i].Read = true
			if a.polledUnreadCount > 0 {
				a.polledUnreadCount--
			}
			cmds = append(cmds, a.markNotifReadCmd(n.ID))
		}
	}
	return a, tea.Batch(cmds...)
}

func (a *App) loadNotifsCmd() tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	types := a.notifications.ActiveTypeFilter()
	return func() tea.Msg {
		notifs, cursor, err := a.client.GetNotifications("", unreadOnly, types)
		if err != nil {
			return errMsg{err}
		}
		return notifsLoadedMsg{notifs: notifs, cursor: cursor}
	}
}

func (a *App) loadNotifsPageCmd(cursor string) tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	types := a.notifications.ActiveTypeFilter()
	return func() tea.Msg {
		notifs, nextCursor, err := a.client.GetNotifications(cursor, unreadOnly, types)
		if err != nil {
			return errMsg{err}
		}
		return notifsPageMsg{notifs: notifs, cursor: nextCursor}
	}
}

func (a *App) markNotifReadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkNotificationRead(id) // fire-and-forget; UI already updated
		return nil
	}
}

// markAllNotifsReadMaxCalls bounds the read-all loop below in case a server
// bug ever reports hasMore indefinitely; 20 calls covers 100,000 notifications
// (5,000/call), far beyond any real account's unread backlog.
const markAllNotifsReadMaxCalls = 20

func (a *App) markAllNotifsReadCmd() tea.Cmd {
	return func() tea.Msg {
		// Fire-and-forget; UI already updated. read-all caps at 5,000/call — loop
		// while the server says more remain.
		for i := 0; i < markAllNotifsReadMaxCalls; i++ {
			hasMore, err := a.client.MarkAllNotificationsRead()
			if err != nil || !hasMore {
				break
			}
		}
		return nil
	}
}

func (a *App) loadBookmarkPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return errMsg{err}
		}
		return bookmarkPostLoadedMsg{post: post}
	}
}

func (a *App) loadBookmarkReplyCmd(replyID string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.GetReply(replyID)
		if err != nil {
			return errMsg{err}
		}
		post, err := a.client.GetPost(reply.PostID)
		if err != nil {
			return errMsg{err}
		}
		return bookmarkReplyLoadedMsg{post: post, replyID: replyID}
	}
}

// enrichBookmarks fetches embedded post/reply content for any bookmark that the
// list API returned without it. All fetches run in parallel; failures are silently
// skipped so the list still shows with whatever data is available.
func enrichBookmarks(client api.Client, items []model.Bookmark) []model.Bookmark {
	type result struct {
		idx   int
		post  *model.Post
		reply *model.Reply
	}
	ch := make(chan result, len(items))
	var wg sync.WaitGroup
	for i, b := range items {
		if b.Post != nil || b.Reply != nil {
			continue
		}
		wg.Add(1)
		i, b := i, b
		go func() {
			defer wg.Done()
			if b.PostID != "" {
				if p, err := client.GetPost(b.PostID); err == nil {
					ch <- result{idx: i, post: &p}
				}
			} else if b.ReplyID != "" {
				if r, err := client.GetReply(b.ReplyID); err == nil {
					ch <- result{idx: i, reply: &r}
				}
			}
		}()
	}
	wg.Wait()
	close(ch)
	out := make([]model.Bookmark, len(items))
	copy(out, items)
	for r := range ch {
		if r.post != nil {
			out[r.idx].Post = r.post
		}
		if r.reply != nil {
			out[r.idx].Reply = r.reply
		}
	}
	return out
}

func (a *App) loadWatchesPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		watches, nextCursor, err := a.client.GetWatches(cursor)
		if err != nil {
			return watchPageMsg{err: err}
		}
		ids := make([]string, len(watches))
		for i, w := range watches {
			ids[i] = w.PostID
		}
		return watchPageMsg{postIDs: ids, cursor: nextCursor}
	}
}

func (a *App) watchPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.WatchPost(postID)
		return watchResultMsg{postID: postID, err: err, added: true}
	}
}

func (a *App) unwatchPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.UnwatchPost(postID)
		return watchResultMsg{postID: postID, err: err, added: false}
	}
}

func (a *App) loadBookmarksCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		items, nextCursor, err := a.client.GetBookmarks(cursor)
		if err != nil {
			return errMsg{err}
		}
		items = enrichBookmarks(a.client, items)
		return bookmarksLoadedMsg{items: items, cursor: nextCursor}
	}
}

func (a *App) loadBookmarksPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		items, nextCursor, err := a.client.GetBookmarks(cursor)
		if err != nil {
			return errMsg{err}
		}
		items = enrichBookmarks(a.client, items)
		return bookmarksPageMsg{items: items, cursor: nextCursor}
	}
}

func (a *App) createBookmarkCmd(postID, replyID string) tea.Cmd {
	return func() tea.Msg {
		id, err := a.client.CreateBookmark(postID, replyID)
		return bookmarkCreatedMsg{bookmarkID: id, postID: postID, replyID: replyID, err: err}
	}
}

func (a *App) deleteBookmarkCmd(id string, fromBookmarksScreen bool) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.DeleteBookmark(id) // fire-and-forget; UI already updated
		return bookmarkDeletedMsg{bookmarkID: id, fromBookmarksScreen: fromBookmarksScreen}
	}
}

// --- Topics commands ---

func (a *App) loadTopicsCmd() tea.Cmd {
	return func() tea.Msg {
		topics, cursor, err := a.client.GetTopics("")
		if err != nil {
			return errMsg{err}
		}
		return topicsLoadedMsg{topics: topics, cursor: cursor}
	}
}

func (a *App) loadMoreTopicsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		topics, nextCursor, err := a.client.GetTopics(cursor)
		if err != nil {
			return errMsg{err}
		}
		return topicsPageMsg{topics: topics, cursor: nextCursor}
	}
}

func (a *App) loadTopicPostsCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetTopicPosts(slug, "")
		if err != nil {
			return errMsg{err}
		}
		return topicPostsLoadedMsg{posts: posts, cursor: cursor}
	}
}

func (a *App) loadTopicPostsPageCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetTopicPosts(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		return topicPostsPageMsg{posts: posts, cursor: nextCursor}
	}
}

// --- Search commands ---

func (a *App) searchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		preview, err := a.client.Search(query)
		if err != nil {
			return errMsg{err}
		}
		return searchPreviewLoadedMsg{preview: preview, query: query}
	}
}

// searchTypeCmd fetches the first page of one search category (a "see all" drill-down).
func (a *App) searchTypeCmd(hitType, query string) tea.Cmd {
	return func() tea.Msg {
		posts, replies, users, cursor, err := a.searchByType(hitType, query, "")
		if err != nil {
			return errMsg{err}
		}
		return searchTypeLoadedMsg{hitType: hitType, posts: posts, replies: replies, users: users, cursor: cursor}
	}
}

// searchTypePageCmd fetches a subsequent page of one search category.
func (a *App) searchTypePageCmd(hitType, query, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, replies, users, nextCursor, err := a.searchByType(hitType, query, cursor)
		if err != nil {
			return errMsg{err}
		}
		return searchTypePageMsg{hitType: hitType, posts: posts, replies: replies, users: users, cursor: nextCursor}
	}
}

// searchByType dispatches to the matching typed search client method. Exactly
// one of the three returned slices is populated, matching hitType.
func (a *App) searchByType(hitType, query, cursor string) (posts []model.Post, replies []model.Reply, users []model.User, nextCursor string, err error) {
	switch hitType {
	case "posts":
		posts, nextCursor, err = a.client.SearchPosts(query, cursor)
	case "replies":
		replies, nextCursor, err = a.client.SearchReplies(query, cursor)
	case "users":
		users, nextCursor, err = a.client.SearchUsers(query, cursor)
	}
	return
}

// --- Guilds commands ---

func (a *App) loadGuildsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		guilds, nextCursor, err := a.client.GetGuilds(cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildsLoadedMsg{guilds: guilds, cursor: nextCursor}
	}
}

func (a *App) loadMoreGuildsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		guilds, nextCursor, err := a.client.GetGuilds(cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildsPageMsg{guilds: guilds, cursor: nextCursor}
	}
}

func (a *App) loadGuildPostsCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetGuildPosts(slug, "")
		if err != nil {
			return errMsg{err}
		}
		return guildPostsLoadedMsg{slug: slug, posts: posts, cursor: cursor}
	}
}

func (a *App) loadGuildPostsPageCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetGuildPosts(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildPostsPageMsg{slug: slug, posts: posts, cursor: nextCursor}
	}
}

func (a *App) createGuildPostCmd(slug, content, title, postSlug string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreateGuildPost(slug, content, title, postSlug, topics)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildPostCreatedMsg{slug: slug}
	}
}

func (a *App) loadGuildDetailCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		g, err := a.client.GetGuild(slug)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildDetailLoadedMsg{guild: g}
	}
}

// loadOwnGuildIntoListCmd fetches a single guild (the user's badge guild or
// one of their apprenticeships) directly when the paginated,
// most-populated-first guild list didn't include it on the first page —
// otherwise sortGuildsForDisplay has nothing to float to the top. Failure is
// silent: the list still works, it just won't show that guild pinned until
// pagination reaches it.
func (a *App) loadOwnGuildIntoListCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		g, err := a.client.GetGuild(slug)
		if err != nil {
			return nil
		}
		return ownGuildInjectMsg{guild: g}
	}
}

func (a *App) joinGuildCmd(slug, name string) tea.Cmd {
	return func() tea.Msg {
		role, err := a.client.JoinGuild(slug)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildJoinedMsg{slug: slug, name: name, role: role}
	}
}

func (a *App) leaveGuildCmd(slug, name string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.LeaveGuild(slug); err != nil {
			return actionErrMsg{err}
		}
		return guildLeftMsg{slug: slug, name: name}
	}
}

func (a *App) promoteGuildCmd(slug, name string) tea.Cmd {
	return func() tea.Msg {
		role, err := a.client.PromoteGuild(slug)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildPromotedMsg{slug: slug, name: name, role: role}
	}
}

func (a *App) loadGuildMembersCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		members, nextCursor, err := a.client.GetGuildMembers(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return guildMembersLoadedMsg{members: members, cursor: nextCursor}
		}
		return guildMembersPageMsg{members: members, cursor: nextCursor}
	}
}

// --- Journal (Notes) commands ---

func (a *App) loadJournalCmd() tea.Cmd {
	return func() tea.Msg {
		notes, cursor, err := a.client.GetNotes("")
		if err != nil {
			return errMsg{err}
		}
		return journalLoadedMsg{notes: notes, cursor: cursor}
	}
}

func (a *App) loadJournalPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		notes, nextCursor, err := a.client.GetNotes(cursor)
		if err != nil {
			return errMsg{err}
		}
		return journalPageMsg{notes: notes, cursor: nextCursor}
	}
}

// saveNoteCmd creates a new note (noteID == "") or updates an existing one.
func (a *App) saveNoteCmd(noteID, content string, topics []string) tea.Cmd {
	if noteID == "" {
		return func() tea.Msg {
			note, err := a.client.CreateNote(content, topics)
			if err != nil {
				return actionErrMsg{err}
			}
			return noteCreatedMsg{note: note}
		}
	}
	id := noteID // capture for closure
	return func() tea.Msg {
		if err := a.client.UpdateNote(id, content, topics); err != nil {
			return actionErrMsg{err}
		}
		return noteUpdatedMsg{noteID: id, content: content, topics: topics}
	}
}

func (a *App) deleteNoteCmd(noteID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteNote(noteID); err != nil {
			return actionErrMsg{err}
		}
		return noteDeletedMsg{noteID: noteID}
	}
}

func (a *App) deletePostCmd(postID string, fromFeed bool) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeletePost(postID); err != nil {
			return actionErrMsg{err}
		}
		return postDeletedMsg{postID: postID, fromFeed: fromFeed}
	}
}

// editPostCmd sends a post edit. otherAttachments are attachments the edit
// panel found on the post but doesn't manage (e.g. an audio one) — when
// attachmentTouched, they're resent alongside the resolved attachmentURL
// since EditPost replaces the whole attachments array wholesale.
func (a *App) editPostCmd(postID, content, title string, topics []string, isPublic, isNSFW bool, attachmentURL string, attachmentTouched bool, otherAttachments []model.Attachment) tea.Cmd {
	return func() tea.Msg {
		var attachments []model.Attachment
		if attachmentTouched {
			attachment, err := resolveAttachment(attachmentURL)
			if err != nil {
				return editErrorMsg(err)
			}
			attachments = append(attachments, otherAttachments...)
			if attachment != nil {
				attachments = append(attachments, *attachment)
			}
		}
		if err := a.client.EditPost(postID, content, title, topics, isPublic, isNSFW, attachments, attachmentTouched); err != nil {
			return editErrorMsg(err)
		}
		return postEditedMsg{
			postID: postID, content: content, title: title, topics: topics,
			isPublic: isPublic, isNSFW: isNSFW, editedAt: time.Now(),
			attachments: attachments, attachmentsTouched: attachmentTouched,
		}
	}
}

func (a *App) deleteReplyCmd(replyID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteReply(replyID); err != nil {
			return actionErrMsg{err}
		}
		return replyDeletedMsg{replyID: replyID}
	}
}

func (a *App) editReplyCmd(replyID, content string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.EditReply(replyID, content); err != nil {
			return editErrorMsg(err)
		}
		return replyEditedMsg{replyID: replyID, content: content, editedAt: time.Now()}
	}
}

// editErrorMsg converts a post/reply-edit error into the message to emit. A
// 403 here means outside the 5-minute window or not a supporter — the
// CanEditSelected gate should prevent most of these, but a race is possible
// (window expires or supporter status lapses between opening the editor and
// submitting). Anything else falls through to actionErrMsg's normal handling.
func editErrorMsg(err error) tea.Msg {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 403 {
		return notifyMsg{level: notifyError, text: "can't edit — outside the 5-minute window or not a supporter"}
	}
	return actionErrMsg{err}
}

// flagResultText picks the banner text for a completed report, distinguishing
// a fresh report from one the caller had already filed (idempotent replay).
func flagResultText(alreadyFlagged bool) string {
	if alreadyFlagged {
		return "already reported"
	}
	return "reported"
}

// flagErrorMsg converts a flag-action error into the message to emit. The API's
// only documented 403 for these endpoints is reporting your own content — the
// client-side guard (see FeedModel/PostDetailModel's "!" handler) should make
// this unreachable, but a stale currentUsername could still race past it, so
// it gets a friendly banner instead of the raw "API error FORBIDDEN (403): …"
// text. Anything else falls through to actionErrMsg's normal handling
// (including the session-expiry redirect in handleUnauthorized).
func flagErrorMsg(err error) tea.Msg {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 403 {
		return notifyMsg{level: notifyError, text: "you can't report your own content"}
	}
	return actionErrMsg{err}
}

func (a *App) flagPostCmd(postID, reason string) tea.Cmd {
	return func() tea.Msg {
		_, alreadyFlagged, err := a.client.FlagPost(postID, reason)
		if err != nil {
			return flagErrorMsg(err)
		}
		return notifyMsg{level: notifyInfo, text: flagResultText(alreadyFlagged)}
	}
}

func (a *App) flagReplyCmd(replyID, reason string) tea.Cmd {
	return func() tea.Msg {
		_, alreadyFlagged, err := a.client.FlagReply(replyID, reason)
		if err != nil {
			return flagErrorMsg(err)
		}
		return notifyMsg{level: notifyInfo, text: flagResultText(alreadyFlagged)}
	}
}

func (a *App) flagRoomMessageCmd(roomID, messageID, reason string) tea.Cmd {
	return func() tea.Msg {
		_, alreadyFlagged, err := a.client.FlagRoomMessage(roomID, messageID, reason)
		if err != nil {
			return flagErrorMsg(err)
		}
		return notifyMsg{level: notifyInfo, text: flagResultText(alreadyFlagged)}
	}
}

func (a *App) deleteRoomMessageCmd(roomID, messageID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteRoomMessage(roomID, messageID); err != nil {
			return actionErrMsg{err}
		}
		return roomMessageDeletedMsg{messageID: messageID}
	}
}

// publishNoteCmd creates a post from the note's content and topics.
// Published notes have no title, are private, and not marked NSFW.
func (a *App) publishNoteCmd(content string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, "", "", topics, false, false, nil)
		if err != nil {
			return actionErrMsg{err}
		}
		return notePublishedMsg{}
	}
}

func (a *App) loadNoteRevisionsCmd(noteID, cursor string) tea.Cmd {
	return func() tea.Msg {
		revisions, next, err := a.client.GetNoteRevisions(noteID, cursor)
		if err != nil {
			return errMsg{err}
		}
		return noteRevisionsLoadedMsg{noteID: noteID, revisions: revisions, cursor: next}
	}
}

func (a *App) loadNoteRevisionCmd(noteID string, revision int) tea.Cmd {
	return func() tea.Msg {
		note, err := a.client.GetNoteRevision(noteID, revision)
		if err != nil {
			return errMsg{err}
		}
		return noteRevisionPreviewMsg{note: note}
	}
}

func (a *App) schedulePollCmd() tea.Cmd {
	gen := a.sessionGen
	return tea.Tick(60*time.Second, func(time.Time) tea.Msg { return pollUnreadTickMsg{gen: gen} })
}

// scheduleRelativeTimeTickCmd self-reschedules every 20s, close enough to
// relative time's coarsest visible increment ("just now" -> "1m ago") that
// the lag is imperceptible. handleRelativeTimeTick only acts on it when the
// active screen is cIRC or C-Mail and its time-display setting is
// "relative" — otherwise this is a no-op tick, so it always runs rather than
// starting/stopping around room/conversation open-close lifecycle.
func (a *App) scheduleRelativeTimeTickCmd() tea.Cmd {
	gen := a.sessionGen
	return tea.Tick(20*time.Second, func(time.Time) tea.Msg { return relativeTimeTickMsg{gen: gen} })
}

func (a *App) scheduleWanderCmd() tea.Cmd {
	gen := a.sessionGen
	return tea.Tick(1*time.Hour, func(time.Time) tea.Msg { return wanderTickMsg{gen: gen} })
}

func (a *App) scheduleLogoAnimCmd() tea.Cmd {
	gen := a.sessionGen
	return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return logoAnimTickMsg{gen: gen} })
}

func logoFrameTickCmd() tea.Cmd {
	return tea.Tick(logoFrameInterval, func(time.Time) tea.Msg { return logoFrameTickMsg{} })
}

// checkAndWanderCmd fires a profile location update if wander mode is enabled
// and at least 12 hours have elapsed since the last update. All failures are
// silent — the user is never notified.
func (a *App) checkAndWanderCmd() tea.Cmd {
	return func() tea.Msg {
		if a.ephemeral {
			return wanderDoneMsg{}
		}
		cfg, err := config.Load()
		if err != nil {
			return wanderDoneMsg{}
		}
		if !config.ShouldWanderNow(cfg) {
			return wanderDoneMsg{}
		}
		lat := math.Round((rand.Float64()*180-90)*1e4) / 1e4
		lon := math.Round((rand.Float64()*360-180)*1e4) / 1e4
		name := "Wandering the world..."
		update := model.ProfileUpdate{
			LocationLatitude:  &lat,
			LocationLongitude: &lon,
			LocationName:      &name,
		}
		if err := a.client.UpdateProfile(update); err != nil {
			return wanderDoneMsg{}
		}
		return wanderDoneMsg{at: time.Now().UTC()}
	}
}

// checkForUpdateCmd checks GitHub for a newer release once at startup, only
// on an actual released build (version.Version != "dev"). All failures —
// including "already on the latest release" — are silent; the user is only
// notified when a strictly newer tag is found.
func (a *App) checkForUpdateCmd() tea.Cmd {
	if a.ephemeral || version.Version == "dev" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		rel, err := update.Latest(ctx)
		if err != nil || rel.TagName == "" || rel.TagName == version.Version {
			return nil
		}
		return updateAvailableMsg{tag: rel.TagName, url: rel.HTMLURL}
	}
}

func (a *App) fetchUnreadCountCmd() tea.Cmd {
	return func() tea.Msg {
		count, exact, err := a.client.GetUnreadNotificationCount()
		if err != nil {
			return nil
		}
		return unreadCountMsg{count: count, exact: exact}
	}
}

// notifBadgeText formats the tab-bar unread count, rendering "99+" once the
// server-reported count is inexact (v0.8.5+: true count exceeds 100).
func notifBadgeText(count int, exact bool) string {
	if !exact {
		return "99+"
	}
	return strconv.Itoa(count)
}

func (a *App) loadPostAndShowCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return notifPostLoadErrMsg{err}
		}
		return notifPostLoadedMsg{post: post}
	}
}

// loadPostBySlugCmd fetches a post permalink (see routeURL) by its author's
// username and per-author slug. origin is the screen to return to when
// PostDetail closes; it travels through urlPostLoadedMsg rather than being
// read from a.active when the response arrives.
func (a *App) loadPostBySlugCmd(username, slug string, origin screen) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPostBySlug(username, slug)
		if err != nil {
			return urlPostLoadErrMsg{err}
		}
		return urlPostLoadedMsg{post: post, origin: origin}
	}
}

// loadProfilePostCmd fetches a post for display when navigating from a profile Replies tab.
func (a *App) loadProfilePostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return errMsg{err}
		}
		return profilePostLoadedMsg{post: post}
	}
}
