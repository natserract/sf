# Automation Toolkit

This use case is a Chrome Extension MVP for running automation scripts

## What it does

- Loads a bundled report template from `templates/`
- Reuses the current browser session and optionally navigates to a target URL
- Executes steps using a strict `type + operator` schema:
  - `waitFor.exists` for DOM readiness checks
  - `dom.readText` for reading visible text from a selector into results (for slides and downstream steps)
  - `text.input` for in-page user text input
  - `csv.input` for in-page CSV upload and parsing
  - `csv.aggregate` for derived CSV values (for example column sum)
  - `image.capture` for DOM screenshot capture
  - `image.input` for in-page image/video upload
- Captures element screenshots with `chrome.tabs.captureVisibleTab`
- Crops the captured viewport image to the target element bounds
- Generates and downloads a Reveal.js HTML deck

## Files

- `manifest.json`: MV3 extension manifest
- `popup.html` / `popup.js`: extension UI
- `background.js`: orchestration, navigation, screenshot capture, deck export
- `content_runner.js`: page-side DOM execution runtime
- `dom_picker.js`: in-page interactive DOM picker for selector capture
- `template_loader.js`: bundled/custom template loading and YAML/JSON parsing
- `reveal_renderer.js`: Reveal.js deck generation
- `templates/default-kit/default-sales.json`: starter template
- `vendor/js-yaml.min.js`: vendored YAML parser

## Template schema

Template fields:

- `id`: unique template id
- `name`: human-friendly name
- `page`: optional URL to navigate to before running
- `frameSelector`: optional iframe selector in the top document (e.g. `.mc-app-iframe`) to target execution inside that iframe
- `frameUrlIncludes`: optional iframe URL substring to target execution inside a specific frame
- `steps`: ordered extraction steps (legacy flat format)
- `tasks`: ordered task groups; each task has `id`, optional `name`, and `steps`
- `slides`: optional Reveal.js slide definitions with `{{stepId}}` placeholders
- `themeId`: optional built-in deck theme key (for style presets)
- `deckStyle`: optional deck style tokens (`deckBg`, `cardBg`, `cardRadius`, `textPrimary`, `accentPrimary`, `fontFamily`)
- `deckCss`: optional custom CSS overrides (scoped to this template deck)
- `slideLayouts`: optional layout metadata for slide authoring conventions
- external slides HTML: auto-resolved by template id as `templates/<template-id>/slides.html`

Supported step routes:

### `waitFor.exists`

Waits until `document.querySelectorAll(selector)[matchIndex]` exists. Set `"requireVisible": true` to keep polling until that node passes a visibility check (useful when a shell appears before content, e.g. wait for `.parent .ps-container`).

```json
{
  "type": "waitFor",
  "operator": "exists",
  "selector": "body",
  "timeoutMs": 10000
}
```

### `dom.readText`

Waits for the node (same polling rules as `waitFor` when `requireVisible` is set), then stores trimmed `innerText` or `textContent` under `value` for `{{placeholder}}` use in slides.

```json
{
  "id": "metricPrimary",
  "type": "dom",
  "operator": "readText",
  "value": "intelMetric0Text",
  "selector": ".primary-value span",
  "requireVisible": true,
  "timeoutMs": 60000,
  "matchIndex": 0,
  "textMode": "innerText"
}
```

Optional `textMode`: `"innerText"` (default) or `"textContent"`.

### `text.input`

```json
{
  "id": "reportNotes",
  "type": "text",
  "operator": "input",
  "value": "reportNotes",
  "label": "Notes for this run",
  "multiline": true,
  "placeholder": "Type notes..."
}
```

### `csv.input`

```json
{
  "id": "salesCsv",
  "type": "csv",
  "operator": "input",
  "value": "salesCsv",
  "label": "Upload CSV",
  "delimiter": ",",
  "hasHeader": true,
  "maxRows": 5000
}
```

### `csv.aggregate`

```json
{
  "id": "orderRateSum",
  "type": "csv",
  "operator": "aggregate",
  "value": "orderRateSum",
  "sourceValue": "salesCsv",
  "aggregateType": "sum",
  "column": "Order Rate"
}
```

### `image.capture`

Optional `readySelector` (and `readyTimeoutMs`, `readyMatchIndex`, `readyRequireVisible`): wait for a descendant or compound selector before capture, and **again after** any `dismissSelectors` clicks so the chart can remount once a modal closes.

```json
{
  "id": "pipelineChart",
  "type": "image",
  "operator": "capture",
  "value": "pipelineChart",
  "selector": "#pipeline-chart",
  "readySelector": "#pipeline-chart .ps-container",
  "readyRequireVisible": true
}
```

### `image.input`

```json
{
  "id": "uploadedAsset",
  "type": "image",
  "operator": "input",
  "value": "uploadedAsset",
  "label": "Upload image or video",
  "accept": "image/*,video/*",
  "maxBytes": 8000000
}
```

Notes:
- `operator` is required for every step.
- `value` is required for output-producing steps (`text.input`, `csv.input`, `csv.aggregate`, `image.capture`, `image.input`).
- `waitFor.exists` must not define `value` because it does not produce exported output.
- Old legacy types (`attribute`, `screenshot`, `inputText`, `inputCsv`, `inputMedia`, etc.) are not supported.
- Presentation styling can be controlled per template via `themeId`, `deckStyle`, and `deckCss`.
- Slide markup is loaded from `templates/<template-id>/slides.html` and placeholders are replaced from the runtime result map keyed by `value`.

### Template-specific styling example

```json
{
  "id": "default-kit",
  "themeId": "eraspace-kpi",
  "deckStyle": {
    "deckBg": "#f6f8ff",
    "cardBg": "#e6eeff",
    "cardRadius": "18px",
    "textPrimary": "#1e2447",
    "accentPrimary": "#1654b8"
  },
  "deckCss": ".kpi-hero { border: 1px solid rgba(22,84,184,0.1); }"
}
```

## YAML support

The popup accepts a custom YAML or JSON template. Custom templates are stored in `chrome.storage.local` and take precedence over bundled templates when they use the same `id`.

## Pick Element from page

The popup includes a `Pick Element From Page` action to avoid hand-writing long CSS selectors.

1. Open the popup and click `Pick Element From Page`.
2. Move your cursor over the target page (or iframe) and click the element you want.
3. Back in the popup, review selector preview and click:
   - `Insert Text Step` (inserts `waitFor.exists`), or
   - `Insert Screenshot Step` (inserts `image.capture`)
4. Save the custom template.

The picker inserts a JSON step snippet into the custom template editor using the captured selector (and `matchIndex` when needed).

## How to load the extension

1. Open `chrome://extensions`
2. Enable Developer mode
3. Click `Load unpacked`
4. Select `usecases/automation-toolkit`

## How to use it

1. Open the target dashboard page in Chrome and log in normally.
2. Open the extension popup.
3. Select the bundled template or paste a custom YAML/JSON template.
4. Click `Run Report`.
5. Approve the save dialog for the generated Reveal.js deck.

## Manual MVP test checklist

1. Load the unpacked extension successfully without manifest errors.
2. Open a page where the selectors in `templates/default-kit/default-sales.json` exist, or replace them with selectors from your real dashboard.
3. Run the bundled template and confirm:
   - `text.input` collects `reportNotes`
   - `csv.input` parses uploaded CSV into `salesCsv`
   - `csv.aggregate(sum)` returns `orderRateSum` from column `Order Rate`
   - `image.capture` returns an image for `stoChartScreenshot`
4. Open the downloaded HTML file and verify:
   - Reveal.js slides render correctly
   - placeholder values are bound into slides
   - the screenshot slide shows the cropped element image

## Known MVP limitations

- `page` matching is exact URL matching
- iframe execution supports target-frame matching via `frameSelector` or `frameUrlIncludes`, but cross-origin restrictions can still block accurate crop offset resolution in some pages
- if a selector resolves outside the visible viewport, the screenshot crop may fail
- slide export is HTML-only; PDF export is not implemented yet
