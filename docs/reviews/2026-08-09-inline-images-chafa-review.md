# Inline-Images Review — chafa as a Complete Replacement

**Reviewed branch:** `feature/inline-images` (5 commits over `main`, merge-base `2adf4c9`) · **Perspective:** feasibility assessment — could [chafa](https://github.com/hpjansson/chafa) (a general-purpose C terminal-image renderer) replace `internal/ui/imgview` entirely? Not covered by either prior review (`docs/reviews/2026-08-09-inline-images-architecture-review.md`, `docs/reviews/2026-08-09-inline-images-greenfield-critique.md`).

Scope: current `internal/ui/imgview` package (pure Go, 974 LOC: `kitty.go`, `sixel.go`, `iterm2.go`, `fetch.go`, `frames.go`, `scale.go`, `protocol.go`, `cellsize_unix.go`/`cellsize_windows.go`) and this project's build/distribution setup, compared against chafa's actual capabilities and packaging.

---

## Executive Summary

| Severity | Count |
|---|---|
| Blocker | 1 |
| Medium | 2 |
| Informational | 2 |

The blocking issue isn't a code-quality gap in chafa — it's that chafa is a C library with no pure-Go implementation, and this project's entire distribution model (`install.sh` fetching one prebuilt static binary per OS/arch, 5 targets, no runtime dependencies) is built on being pure Go. Every integration path available breaks something the project currently gets for free. Recommendation: do not adopt chafa as a replacement for `imgview`.

---

## What chafa actually is

chafa is [hpjansson/chafa](https://github.com/hpjansson/chafa), a C library plus CLI tool, licensed LGPLv3+ for both. It detects terminal capabilities and renders to Sixel, Kitty, iTerm2, or Unicode block/Braille-character fallback, and reads a substantially wider set of input formats than `imgview` currently handles — JPEG, PNG, GIF (including animation), WebP, AVIF, SVG, TIFF, and JPEG XL, versus `imgview`'s JPEG/PNG/GIF/WebP (`imgview/fetch.go`). There is no official Go port. The one community Go binding found, [ploMP4/chafa-go](https://github.com/ploMP4/chafa-go), avoids cgo at build time via [`purego`](https://github.com/ebitengine/purego): it embeds precompiled `libchafa` shared objects in the Go binary via `//go:embed` and `dlopen`s the correct one for the running platform at startup.

Sources: [hpjansson/chafa](https://github.com/hpjansson/chafa), [hpjansson.org/chafa](https://hpjansson.org/chafa/), [ploMP4/chafa-go](https://github.com/ploMP4/chafa-go).

---

## Findings

### BLOCKER — No integration path exists that preserves this project's distribution model

**Files:** `Makefile:17-23`, `.github/workflows/release.yml:59-65`, `install.sh:21-24` (5 statically cross-compiled targets: linux/darwin amd64+arm64, windows amd64; `grep -rn "cgo\|import \"C\""` across the repo returns zero hits)

**The problem:** cyber-tui is currently pure Go with `CGO_ENABLED` never set, cross-compiled to 5 targets with plain `go build`, and distributed as a single downloaded static binary with no runtime dependencies. Every way to call chafa from Go breaks one side of that:

- `exec.Command("chafa", ...)` — turns an optional terminal capability into a hard external dependency. A user would need chafa separately installed (via their OS package manager) before inline images work at all, and the app needs a "chafa not found" degraded path. This is a materially worse install story than "download one binary," for a feature that today works out of the box wherever a supported terminal is detected.
- cgo bindings to `libchafa` — requires `CGO_ENABLED=1` and a C toolchain (and target sysroot) for every cross-compile target in `release.yml:59-65`, which the current build has never needed.
- `ploMP4/chafa-go` (purego + embedded `.so`, no cgo at build time) — closest to viable, but see the two Medium findings below; it isn't a drop-in fix for this Blocker, it trades a build-time problem for a packaging/coverage one.

**Recommendation:** Do not integrate chafa via any of these paths for the core rendering pipeline. If a specific missing capability (see Informational below) is wanted later, evaluate it narrowly rather than reopening this tradeoff wholesale.

---

### MEDIUM — The only cgo-free Go binding is an immature single-maintainer project with narrower platform coverage than cyber-tui ships today

**Source:** [ploMP4/chafa-go](https://github.com/ploMP4/chafa-go) — 54 GitHub stars at time of review, community project, not affiliated with the official chafa project.

**The problem:** `chafa-go`'s README states it embeds precompiled `libchafa` binaries for exactly four platform/arch combinations: linux/amd64, linux/386, darwin/arm64, windows/amd64. cyber-tui's current release matrix (`release.yml:59-65`) ships linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 — five targets. Adopting `chafa-go` would drop linux/arm64 and darwin/amd64 (Intel Mac) support outright, since no precompiled `libchafa` exists for them in that project. This is a platform-coverage regression, not a neutral tradeoff, and it comes from a project with no version-stability track record to lean on if a target needs to be added later.

**Recommendation:** If this path is ever reconsidered, treat "does it cover every platform we currently ship" as a hard gate before any other evaluation criteria.

---

### MEDIUM — LGPLv3+ embedding pattern is an open compliance question, not a resolved one

**Source:** [hpjansson/chafa](https://github.com/hpjansson/chafa) license (LGPLv3+, confirmed for both library and CLI).

**The problem:** Dynamically loading an LGPL shared library at runtime (classic `dlopen`) is the long-established "safe" pattern for linking LGPL code into a differently-licensed application. `chafa-go`'s approach — embedding the compiled `.so` as a Go `//go:embed` asset inside a statically distributed binary, then `dlopen`-ing it from an extracted temp copy at runtime — is a less common variant of that pattern. It's very likely fine (the library is still loaded dynamically, not statically linked into the Go binary), but "very likely fine" based on this review's reading is an inference, not a verified conclusion, and it's the kind of question worth an explicit answer before shipping rather than an assumed one.

**Recommendation:** Not a reason to rule chafa out on its own, but if this path is ever pursued, get an explicit answer on the embedding pattern before shipping, rather than inheriting the assumption from this review.

---

### INFORMATIONAL — What chafa would genuinely add that `imgview` doesn't have today

**Files:** `imgview/protocol.go` (`DetectProtocol`, `ProbeSixel`) — has exactly three positive outcomes (Kitty, iTerm2, Sixel) and one `ProtocolNone`; there is no rendering path at all for `ProtocolNone`, so a terminal with no detected graphics protocol gets no inline images, full stop.

chafa's Unicode block/Braille-character fallback is a real, maintained answer to exactly that gap — a way to show *something* image-like in a terminal with no graphics protocol, instead of nothing. It also reads more input formats (AVIF, SVG, TIFF, JPEG XL) than `imgview.Fetch` currently decodes. Both are genuine capabilities cyber-tui doesn't have. Neither requires adopting chafa specifically — a native Unicode-block renderer is a small, self-contained, pure-Go addition (no protocol detection changes needed, it's just another arm of the existing switch) if this gap is ever prioritized as its own feature.

---

### INFORMATIONAL — chafa doesn't touch any of the actual bugs found in the two prior reviews

**Files:** see `docs/reviews/2026-08-09-inline-images-architecture-review.md` and `docs/reviews/2026-08-09-inline-images-greenfield-critique.md` in full.

The MillerLayout wiring gap, the unbounded `inlineImageCache`, uncancelled fetch goroutines, silent fetch-failure caching, and the missing golden-output test coverage are all in `internal/ui/app.go`'s orchestration layer — fetch scheduling, caching, compositing into `View()` — not in the encoder layer chafa would replace (`fetchInlineImageCmd`, `app.go:2572-2600`, dispatches to `EncodeITerm2`/`EncodeSixel`/`EncodeKitty` and returns a ready-to-print string via `handleInlineImageFetched`, `app.go:2607-2621`; chafa would only ever sit behind that same call). Swapping the encoder backend is orthogonal to fixing any of them.

---

## Recommendation

Do not adopt chafa as a replacement for `internal/ui/imgview`. The current package is pure Go, dependency-free at runtime, and covers every platform the project ships today — properties chafa cannot match through any available integration path without giving something up (a hard external binary dependency, a C toolchain in the cross-compile pipeline, or a smaller, less-proven community binding with narrower platform coverage). The genuine capability gap chafa would close — a fallback for terminals with no detected graphics protocol — is real but small enough to solve natively if it's ever prioritized, without reopening this tradeoff.
