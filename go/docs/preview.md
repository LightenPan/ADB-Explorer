# File preview

Details pane preview for the native Go client. This document is the implementation contract: every kind listed here is in scope.

Do not edit `third/mygo` for these features. Drive verification with `ui.NewTester` and a PNG under `cmd/adbexplorer/testdata/`. Do not add helper-only unit tests.

## Goal

Selecting one file and opening Details (or submitting a non-directory row) must show a useful preview of that file, not only name / size / mode. Folders stay metadata-only. Binary that is not a known preview kind shows a hex dump, never a wall of replacement characters.

## Current behavior (replace this)

| Surface | What it does today |
|---|---|
| Details pane (`detailPanel`, width 280) | Metadata plus at most 32 KiB of pulled bytes. If `printable`, dump as plain `ui.Text`. Else `"Not a text file"`. Directories and files `> 256 KiB` skip the pull. |
| Double-click / Table submit | Directory → browse. Archive (zip/tar/tgz) → `openArchive` replaces the pane listing. Anything else → `showDetail`. |
| Icon view | Pulls `.png/.jpg/.jpeg/.gif/.webp/.bmp/.apk` up to 8 MiB into `fileIcons` (APK: largest `res/**icon*.png`). Details pane does not use that cache. |
| Edit overlay (`preview.go`) | Separate modal. Files `≤ 512 KiB`: text editor or 4 KiB hex dump. Save is text-only. |
| APK | `apkInfo` shells `unzip -l` + `META-INF/MANIFEST.MF` into a global `apkText` the details pane never shows. |
| Archives | Pane-level browse/extract/zip only. Details treats them as binary text. |

WPF already: photos in the pane, PDF pages (WinRT, password prompt), small files as AvalonEdit with syntax / hex, refuses archives / APK / photos-as-text / NUL bytes / files over `MaxPreviewFileSize` KB (default 300).

## Non-goals

- Play video or audio in the pane.
- Decode HEIC/HEIF/AVIF (no CGO-free decoder in this tree). Detect and label them.
- Adaptive APK icons (XML/AXML). Keep PNG-in-zip as today.
- Edit-and-save from the details pane. Editing stays the Edit overlay.
- Recycle-bin entries (when that lands, preview is off, matching WPF).
- Changing `third/mygo`.

## Surface

The details pane is the only preview surface.

```
[ Details          Close ]
  name
  full path
  kind · size
  modified · mode
  --- preview body (Grow 1, Clip, Scroll) ---
```

- Open: **Details** (exactly one selected item) or submit on a non-directory, non-archive row. Archives still open in the pane on submit; Details on an archive shows the TOC without leaving the folder.
- Close: Close button, or selection becomes empty / not exactly one file. Changing the single selection while open reloads preview (`detail.gen`).
- Width: 360 DIP (up from 280). Do not let the pane steal the file list; list column keeps `Grow(1).Clip()`.
- The pane must never paint over the toolbar or tab strip (existing `FillHeight` / `Clip` rules stay).
- Loading: a spinner or `"Loading preview…"` in the body until the job for this `gen` finishes.
- Error: `d.err` in danger color; metadata still visible.

## Lifecycle

`showDetail` already bumps `detail.gen`. Keep that.

1. Snapshot focused pane, serial, selected entry, remote path, size, mode.
2. Classify (below). Directories: metadata only, no pull.
3. Start one background job. The job must:
   - Use `context.WithTimeout` (12s images/PDF/APK, 8s text/hex/archive/media).
   - Pull through `Client.Pull` into a `limitedWriter` (or a sized buffer). Never pull past the kind’s byte cap.
   - Ignore the result when `a.detail.gen` has moved on (`win.Update` check, same as today).
4. Device poll / browse jobs must not start or cancel preview. Preview cancellation is only: new selection, Close, device change (`serial` mismatch).
5. Reuse `fileIcons[remote]` for raster/APK when the cache already holds a bitmap **and** the kind is icon-sized. Full-size photo preview always pulls (or uses a separate `previewBitmaps[remote]` cache keyed by path+size).

### Failure modes (write these down before the isolated bits)

These are the ways an isolated preview job can fail; E2E of the real UI would miss them:

- Stale `gen`: a slow pull for file A must not paint over file B.
- Byte cap: `Pull` that would exceed N must stop (`errPreviewLimit` is success for preview, not an error).
- Empty / NUL / high-control-byte payloads must not be treated as text.
- Path quoting: remote paths with spaces, `'`, and Unicode must round-trip through `shellQuote` when the kind shells `unzip`/`tar`/`aapt`.
- Queue: preview pulls are not transfer ops; they must not block the FIFO upload/download queue, and a transfer must not cancel a preview.

Keep an isolated test only for those. Everything else is an E2E PNG.

## Classification

Match on the selected **file** (not directories, not links-to-dirs). First matching row wins.

| Order | Kind | How |
|---|---|---|
| 1 | Directory / link-to-dir | `e.IsDir()` |
| 2 | Raster | Extension in `.jpg .jpeg .png .gif .webp .bmp` (case-insensitive) |
| 3 | SVG | `.svg` |
| 4 | PDF | `.pdf` |
| 5 | HEIC | `.heic .heif` or `ftyp` brand `heic/heif/mif1/msf1` |
| 6 | APK family | `.apk .xapk .apks .apkm .apex` (WPF `APK_NAMES`) |
| 7 | Zip family | `.zip .jar .aar` (not already APK) |
| 8 | Tar family | `.tar .tgz .tpz .tbz .tbz2 .txz .tzst` or `.tar.*` |
| 9 | Audio | `.mp3 .aac .ogg .oga .opus .wav .flac .m4a .amr` |
| 10 | Video | `.mp4 .webm .3gp .mkv .mov .avi` |
| 11 | Text | `printable` after a bounded pull, **or** a known text extension (below) with no NUL in the pulled prefix |
| 12 | Hex | everything else that is a regular file |

A known photo/APK/archive extension is **never** shown as text, even if the bytes happen to be printable (WPF does this). A `.wav` with NULs is audio, not text.

### Known text extensions (syntax via `editor.LanguageFor`)

Also treat as text when the name is a listed filename (`Makefile`, `Dockerfile`, `.bashrc`, `build.prop`, …).

| Group | Extensions / names |
|---|---|
| Plain | `.txt .log .text .out` |
| Android / config | `.prop .rc .conf .cfg .ini .properties .editorconfig` ; `build.prop` |
| Data | `.json .jsonc .json5 .xml .csv .tsv .yml .yaml .toml` |
| Web | `.html .htm .xhtml .css .scss .less .js .mjs .cjs .ts .tsx` |
| Docs | `.md .markdown .mdx` |
| Shell / make | `.sh .bash .zsh .fish` ; `Makefile`, `Dockerfile` |
| Code | `.java .kt .kts .smali .py .go .rs .c .h .cpp .cc .cs .lua .php .rb .swift .proto .sql .diff .patch` |

Unknown extension: still text if `printable` (same heuristic as today: no NUL, control bytes except `\n\r\t` are rare, `bad*8 < len`). UTF-8 as-is; if a BOM is present, skip it. Do not invent a charset detector.

## Kind contracts

### 1. Raster (`jpg/jpeg/png/gif/webp/bmp`)

- Cap: 8 MiB. Larger: metadata + `"Image too large to preview (N MB)"`.
- Decode with `ui.DecodeBitmap` (PNG, JPEG+EXIF orientation, GIF **first frame**, WebP, BMP).
- Draw `ui.Image` fitted to pane width, keep aspect, `Clip`. No checkerboard.
- GIF: caption `"GIF · first frame"` under the image.
- Decode error: `"Could not decode image"` + hex of the first 256 bytes is **not** required; metadata + error is enough.
- Icon view keeps using `wantIcon`; details may reuse that bitmap if the file is small (`≤ 256 KiB`) so a thumbnail is already sharp. Otherwise pull once into `previewBitmaps`.

### 2. SVG

- Cap: 1 MiB.
- `ui.ParseSVG` then `ui.Icon` (theme text/accent) **or** `ui.Image` if the document has its own colors. Prefer `ui.Image` when the SVG sets fills other than `currentColor`.
- `ParseSVG` does not draw `<text>`, embedded images, filters. If parse fails, fall through to **text** (XML highlighting) on the same bytes.
- Caption when parse succeeds: `"SVG · paths only"` so missing text is not a surprise.

### 3. PDF

WPF renders every page through `Windows.Data.Pdf`. Go must show pages in the pane, not a “unsupported” stub.

- Cap: 8 MiB pull. Larger: metadata + size message.
- Rasterize pages to `*ui.Bitmap`, stacked in a `ui.Scroll`, pane-width wide.
- First paint: page 1 as soon as it is ready; following pages append (do not wait for the whole document).
- Cap rendered pages at 20 for the pane. If more, footer `"Showing 20 of N pages"`.
- Encrypted PDF: prompt for password in the pane (WPF `IsPdfPasswordPromptVisible`). Wrong password: `"Wrong password"` and keep the field.
- Implementation constraint: `CGO_ENABLED=0`. Acceptable engines, in order:
  1. Pure-Go rasterizer that can be a module dependency under `go/` (not `third/mygo`).
  2. Windows-only WinRT `PdfDocument` via `syscall` (same API WPF uses), with a clear `"PDF preview is Windows-only"` on other GOOS.
- If the engine is missing at runtime: `"PDF preview unavailable"` — do not dump the binary as text.

### 4. HEIC / HEIF

- No decode.
- Body: `"HEIC camera image — preview is not available"` plus size and modified. Do not pull more than 64 bytes (magic).

### 5. APK family

- Always show metadata + package icon if we can get one.
- Icon: existing `iconFor` (best `res/**icon*.png` by density). Pull the APK only when `size ≤ 8 MiB`. Larger: skip icon, still list contents via device `unzip -l`.
- Info block (plain text, 12px):
  - First 30 zip members (name + size), then `"… N more"` if truncated.
  - `META-INF/MANIFEST.MF` first 40 lines when present (`unzip -p` on device, quoted path).
- Do not run `apkInfo` into a hidden global; write into `detail` so the pane actually shows it.
- `.xapk/.apks/.apkm` are zip families: same TOC path; icon only if a nested `.apk` is not required (do not recursively pull).

### 6. Zip / tar TOC

Details on an archive **does not** enter it (that remains submit / `openArchive`).

- Zip / jar / aar: `unzip -l` on device, parse with the existing `archiveEntries` zip branch. Cap 200 rows in the pane.
- Tar family: `tar -tf` (and `-tzf` / `-tjf` / `-tJf` / `--zstd -tf` according to suffix). Cap 200 rows.
- Each row: name, size when known, folder mark when the name ends in `/`.
- Failure (`unzip`/`tar` missing, permission): `d.err`, no fake listing.
- Extend `isArchive` to the tar suffixes in the table so extract/submit stay consistent. `.tzst` extract uses `tar --zstd -xf` when the binary supports it; if the shell errors, surface the error.

### 7. Audio / video (metadata only)

- Pull at most 64 KiB (enough for ID3 / ftyp / RIFF).
- Body: `"Audio"` or `"Video"` + `"not playable in preview"`.
- Best-effort extra line, omit if unknown:
  - WAV: sample rate / channels from the `fmt ` chunk.
  - MP3: ID3v2 title if present in the prefix.
  - MP4/3GP: `ftyp` brand.
- Never call a decoder or external player.

### 8. Text

- Skip pull when `size` ≥ `MaxPreviewBytes` (setting, default **300 KiB**, WPF `MaxPreviewFileSize`). Message: `"File is too large to preview"`. Edit overlay keeps its own 512 KiB limit.
- Pull cap = `min(size, MaxPreviewBytes)`.
- NUL in the pulled bytes → treat as **hex**, not text (WPF).
- Render with `editor.View` read-only, `LanguageFor(name)` when MyGo knows the language, otherwise plain.
- Header row: language name or `"Plain text"`, plus a **Hex** toggle that swaps the body to a hex dump of the same bytes (WPF syntax selector). Toggle does not re-pull.
- Truncation: if `size > len(pulled)`, caption `"Showing first N KB"`.

### 9. Hex (binary fallback)

- Pull cap: 4 KiB (same as `hexDump` today).
- Body: the existing `hexDump` layout (`offset  hex  |ascii|`).
- Caption: `"Binary · first 4 KB"`.
- `.so .dex .odex .oat .bin .dat .pak .db` land here unless a kind above claimed them.

### 10. Directory

- Name, path, mode, modified. No children listing (the main list is the listing).
- Optional one-liner: `"N items"` from the current pane only if the directory **is** the open folder; do not `LIST` a different dir just for Details.

## Settings

Add to `settings` / Settings page (persist in `settings.json`):

| Field | Default | Meaning |
|---|---|---|
| `MaxPreviewKB` | `300` | Text/PDF/image skip-or-cap threshold in KiB. `0` means “use default 300”, not “unlimited”. Clamp 16…10240. |
| `PreviewPDF` | `true` | If false, PDF shows the unavailable message without pulling. |

Raster/APK 8 MiB and hex 4 KiB stay constants.

## UI details

- Kind label in the metadata line uses a specific word: `Folder`, `Image`, `SVG`, `PDF`, `HEIC`, `APK`, `Archive`, `Audio`, `Video`, `Text`, `Binary`, `Link` — not always `File`.
- Image/SVG/PDF bitmaps live in `detail` (`*ui.Bitmap` or `[]*ui.Bitmap`), not in the text field. `detail.preview` remains the text/hex/TOC/info string.
- `detailPanel` branches:

```
if d.err != "" → danger text
if d.bitmaps != nil → scroll of ui.Image
if d.svg != nil → ui.Icon/Image
if d.edit != nil → editor.View (read-only)
else if d.preview != "" → ui.Text 12px or hex monospaced
else if loading → spinner
```

- Hex uses a monospaced `FontSize(12)`.
- Do not put Edit/Save in this pane. A small `"Open in editor"` button is allowed when the kind is text or hex and `size ≤ 512 KiB`; it calls `openEditor`.

## Concurrency and cache

| Cache | Key | Value | Evict |
|---|---|---|---|
| `fileIcons` | remote path | `*ui.Bitmap` (icon-sized) | device change, 64 entries LRU |
| `previewBitmaps` | remote path + mtime/size | `*ui.Bitmap` or PDF page list | device change, Close, 8 entries LRU |

Preview pulls use their own goroutine, not `enqueue`. They must not call `load` / `begin` (browse slot).

On `serial` change: clear both caches, close the pane or keep it with `"No device"`.

## Testing

E2E through `ui.NewTester`. Each case writes a PNG ≥ 1000 bytes:

| Test | Fixture | Artifact | Must show |
|---|---|---|---|
| Raster | `photo.png` bytes in a fake entry + injected bitmap (or decode fixture) | `e2e-preview-image.png` | Image in the pane, not `"Not a text file"`; toolbar still above the list |
| Text + syntax | `build.prop` / `notes.json` content | `e2e-preview-text.png` | Body text, language or filename visible |
| Hex | bytes with NULs, `.bin` | `e2e-preview-hex.png` | Offset dump, not mojibake |
| APK | small zip with `res/mipmap-xxhdpi/ic_launcher.png` + MANIFEST | `e2e-preview-apk.png` | Icon and a member name |
| Zip TOC | `unzip -l` shaped listing injected as `detail.preview` **or** parsed `archiveEntries` fed into the pane | `e2e-preview-zip.png` | Member names, still in `/sdcard` not inside the zip |
| SVG | path-only SVG | `e2e-preview-svg.png` | Vector, not raw XML |
| PDF | skip if engine absent; otherwise one-page PDF | `e2e-preview-pdf.png` | At least page 1 |
| Too large | size `> MaxPreviewKB` | `e2e-preview-too-large.png` | Size message, no body |
| Layout | long file list + details open | `e2e-preview-layout.png` | Tab bottom ≤ Up top; preview column right of the list |

Isolated tests (only if E2E cannot see it): `archiveEntries` zip header skip (`Name`/`----`); `printable` / NUL; stale `gen`; `limitedWriter` cap.

## Implementation order

Ship in this order so each step is demoable:

1. **Classify + pane chrome** — kind labels, 360 width, loading/error, `Open in editor`. Text path still as today but using `MaxPreviewKB`.
2. **Raster in the pane** — `DecodeBitmap` + `ui.Image`, reuse `fileIcons` when possible.
3. **Text via `editor.View` + Hex toggle** — LanguageFor, NUL → hex.
4. **APK in the pane** — move `apkInfo` into `detail`, draw icon.
5. **Zip/tar TOC in the pane** — expand `isArchive` suffixes; do not change submit-to-browse.
6. **SVG**.
7. **Audio/video/HEIC labels** (64 KiB prefix, no playback).
8. **PDF pages** (engine + password field).
9. **Caches / LRU / settings**.

Do not stop after step 3. Steps 4–8 are part of this spec, not follow-ups.

## Mapping to files

| File | Change |
|---|---|
| `cmd/adbexplorer/state.go` | `detail` fields (`bitmaps`, `svg`, `edit`, `loading`, kind); `showDetail` classifier; caps; gen-safe publish |
| `cmd/adbexplorer/view.go` | `detailPanel` body; width 360; kind line; Hex toggle; Open in editor |
| `cmd/adbexplorer/icons.go` | Shared raster/APK decode; `previewBitmaps`; do not `wantIcon` from the list view |
| `cmd/adbexplorer/apk.go` | Feed `detail` instead of `apkText` |
| `cmd/adbexplorer/preview.go` | Hex helper stays; editor overlay unchanged except it can be opened from the pane |
| `cmd/adbexplorer/settings.go` / `settings_page.go` | `MaxPreviewKB`, `PreviewPDF` |
| `cmd/adbexplorer/view_test.go` | E2E cases above |
| New `cmd/adbexplorer/preview_pdf.go` | PDF engine wrapper (build-tagged `windows` if WinRT) |

## Acceptance

A device folder with a PNG, a `build.prop`, a small APK, a zip, an SVG, a PDF, and a `.so` can be clicked one by one with Details open. Each shows the kind-specific body above. The file list never covers the toolbar. Closing Details restores the list width. Switching selection cancels the previous pull.
)
