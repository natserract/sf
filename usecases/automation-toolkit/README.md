# Automation Toolkit

This use case is a Chrome Extension MVP for running automation scripts

## What it does

- Loads a bundled report template from `templates/`
- Reuses the current browser session and optionally navigates to a target URL
- Executes selector-based extraction steps:
  - `waitFor`
  - `text`
  - `attribute`
  - `screenshot`
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
- `templates/default-sales.json`: starter template
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

Supported step types:

### `waitFor`

```json
{
  "id": "dashboardLoaded",
  "type": "waitFor",
  "selector": "body",
  "timeoutMs": 10000
}
```

### `text`

```json
{
  "id": "revenue",
  "type": "text",
  "selector": ".revenue-value"
}
```

### `attribute`

```json
{
  "id": "reportDate",
  "type": "attribute",
  "selector": "[data-report-date]",
  "attribute": "data-report-date"
}
```

### `screenshot`

```json
{
  "id": "pipelineChart",
  "type": "screenshot",
  "selector": "#pipeline-chart"
}
```

## YAML support

The popup accepts a custom YAML or JSON template. Custom templates are stored in `chrome.storage.local` and take precedence over bundled templates when they use the same `id`.

## Pick Element from page

The popup includes a `Pick Element From Page` action to avoid hand-writing long CSS selectors.

1. Open the popup and click `Pick Element From Page`.
2. Move your cursor over the target page (or iframe) and click the element you want.
3. Back in the popup, review selector preview and click:
   - `Insert Text Step`, or
   - `Insert Screenshot Step`
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
2. Open a page where the selectors in `templates/default-sales.json` exist, or replace them with selectors from your real dashboard.
3. Run the bundled template and confirm:
   - text extraction populates `revenue`
   - attribute extraction populates `reportDate`
   - screenshot extraction returns an image for `pipelineChart`
4. Open the downloaded HTML file and verify:
   - Reveal.js slides render correctly
   - placeholder values are bound into slides
   - the screenshot slide shows the cropped element image

## Known MVP limitations

- `page` matching is exact URL matching
- iframe execution supports target-frame matching via `frameSelector` or `frameUrlIncludes`, but cross-origin restrictions can still block accurate crop offset resolution in some pages
- if a selector resolves outside the visible viewport, the screenshot crop may fail
- slide export is HTML-only; PDF export is not implemented yet
