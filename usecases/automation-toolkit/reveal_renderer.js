function tbrgEscapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function tbrgSanitizeClassNamePart(value, fallback = 'default') {
  const raw = String(value || '').trim().toLowerCase();
  const safe = raw.replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '');
  return safe || fallback;
}

function tbrgToInterpolatedText(value) {
  if (value == null) {
    return '';
  }
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  try {
    const json = JSON.stringify(value, null, 2);
    if (typeof json === 'string') {
      const maxLen = 5000;
      return json.length > maxLen ? `${json.slice(0, maxLen)}\n... (truncated)` : json;
    }
  } catch (_e) {
    // ignore
  }
  return String(value);
}

function tbrgInterpolate(templateText, resultMap) {
  return String(templateText).replace(/\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}/g, (_match, key) => {
    const value = resultMap[key];
    if (typeof value === 'string' && value.startsWith('data:image/')) {
      return `<img src="${value}" alt="${tbrgEscapeHtml(key)}" style="max-width: 100%; max-height: 480px;" />`;
    }
    return tbrgEscapeHtml(tbrgToInterpolatedText(value));
  });
}

function tbrgBuildDefaultSlides(resultMap) {
  const entries = Object.entries(resultMap);
  const slides = [
    {
      title: 'Report Summary',
      body: entries.length
        ? `<ul>${entries.map(([key, value]) => `<li><strong>${tbrgEscapeHtml(key)}:</strong> ${typeof value === 'string' && value.startsWith('data:image/') ? '[image]' : tbrgEscapeHtml(tbrgToInterpolatedText(value))}</li>`).join('')}</ul>`
        : '<p>No results were captured.</p>'
    }
  ];

  for (const [key, value] of entries) {
    if (typeof value === 'string' && value.startsWith('data:image/')) {
      slides.push({
        title: key,
        body: `<img src="${value}" alt="${tbrgEscapeHtml(key)}" style="max-width: 100%; max-height: 560px;" />`
      });
      continue;
    }

    slides.push({
      title: key,
      body: `<pre style="font-size: 1.1rem; white-space: pre-wrap; text-align: left;">${tbrgEscapeHtml(tbrgToInterpolatedText(value))}</pre>`
    });
  }

  return slides;
}

function tbrgRenderSlides(template, resultMap) {
  const slideDefs = Array.isArray(template.slides) && template.slides.length
    ? template.slides
    : tbrgBuildDefaultSlides(resultMap);

  return slideDefs.map((slide) => {
    const title = slide.title ? `<h2>${tbrgInterpolate(slide.title, resultMap)}</h2>` : '';
    const body = slide.body ? tbrgInterpolate(slide.body, resultMap) : '';
    return `<section>${title}${body}</section>`;
  }).join('\n');
}

function tbrgRenderExternalSlidesHtml(externalSlidesHtml, resultMap) {
  const raw = String(externalSlidesHtml || '').trim();
  if (!raw) {
    return '';
  }
  return tbrgInterpolate(raw, resultMap);
}

function tbrgBuildScopedDeckCss(scopeClass, template) {
  const builtInThemeMap = {
    'eraspace-kpi': `
      .kpi-hero { background: #eef3ff; border-radius: 24px; padding: 24px; color: #1e2447; }
      .kpi-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
      .kpi-subtitle { margin: 0; font-size: 1rem; opacity: 0.85; }
      .kpi-period { margin: 8px 0 0; font-size: 1.1rem; font-weight: 700; color: #384178; text-transform: uppercase; letter-spacing: 0.03em; }
      .kpi-badge { background: #2055c7; color: #fff; border-radius: 999px; padding: 10px 14px; font-size: 0.85rem; font-weight: 600; white-space: nowrap; }
      .kpi-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 14px; }
      .kpi-card { background: #dce7ff; border-radius: 18px; padding: 16px; box-sizing: border-box; }
      .kpi-card.primary { background: #1654b8; color: #fff; }
      .kpi-card h4 { margin: 0 0 10px; font-size: 1rem; font-weight: 700; }
      .kpi-value { margin: 0; font-size: 2.2rem; line-height: 1.1; font-weight: 800; }
      .kpi-value.small { font-size: 1.8rem; color: #1654b8; }
      .kpi-footnote { margin: 8px 0 0; font-size: 0.82rem; opacity: 0.9; }
      .kpi-notes { margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 0.9rem; line-height: 1.35; background: rgba(255, 255, 255, 0.7); border-radius: 10px; padding: 10px; max-height: 220px; overflow: auto; }
      .kpi-card.media img { max-height: 230px !important; width: 100%; object-fit: contain; background: #fff; border-radius: 10px; padding: 6px; }
      .summary-layout { display: grid; grid-template-columns: 1.1fr 0.9fr; gap: 16px; align-items: start; }
      .summary-col { background: #eef3ff; border-radius: 16px; padding: 14px; }
      .summary-col h4 { margin: 0 0 10px; color: #2d3975; }
      .summary-col img { max-height: 250px !important; width: 100%; object-fit: contain; background: #fff; border-radius: 10px; margin-bottom: 10px; padding: 6px; }
    `
  };

  const themeKey = tbrgSanitizeClassNamePart(template.themeId || '', '');
  const builtInCss = themeKey ? (builtInThemeMap[themeKey] || '') : '';
  const deckStyle = template.deckStyle && typeof template.deckStyle === 'object' ? template.deckStyle : {};
  const tokenCss = `
    .${scopeClass} {
      --deck-bg: ${tbrgEscapeHtml(deckStyle.deckBg || '#ffffff')};
      --card-bg: ${tbrgEscapeHtml(deckStyle.cardBg || '#eef3ff')};
      --card-radius: ${tbrgEscapeHtml(deckStyle.cardRadius || '16px')};
      --text-primary: ${tbrgEscapeHtml(deckStyle.textPrimary || '#1f2937')};
      --accent-primary: ${tbrgEscapeHtml(deckStyle.accentPrimary || '#1654b8')};
      --font-family: ${tbrgEscapeHtml(deckStyle.fontFamily || 'Inter, system-ui, Arial, sans-serif')};
    }
    .${scopeClass} { font-family: var(--font-family); background: var(--deck-bg); color: var(--text-primary); }
    .${scopeClass} .template-card { background: var(--card-bg); border-radius: var(--card-radius); }
    .${scopeClass} .template-accent { color: var(--accent-primary); }
  `;
  const customCss = typeof template.deckCss === 'string' ? template.deckCss : '';

  const scopedBuiltInCss = builtInCss
    .replace(/(^|\})\s*([^{@}][^{]*?)\s*\{/g, (_m, brace, selector) => {
      const parts = selector.split(',').map((s) => s.trim()).filter(Boolean);
      const prefixed = parts.map((s) => `.${scopeClass} ${s}`).join(', ');
      return `${brace}\n${prefixed} {`;
    });

  const scopedCustomCss = customCss
    ? customCss.replace(/(^|\})\s*([^{@}][^{]*?)\s*\{/g, (_m, brace, selector) => {
      const parts = selector.split(',').map((s) => s.trim()).filter(Boolean);
      const prefixed = parts.map((s) => `.${scopeClass} ${s}`).join(', ');
      return `${brace}\n${prefixed} {`;
    })
    : '';

  const combined = [scopedBuiltInCss, tokenCss, scopedCustomCss].filter(Boolean).join('\n');
  if (!combined.trim()) {
    return '';
  }
  return combined;
}

function tbrgBuildRevealDeckHtml(template, resultMap, externalSlidesHtml = '') {
  const title = template.name || template.id;
  const externalMarkup = tbrgRenderExternalSlidesHtml(externalSlidesHtml, resultMap);
  const slideMarkup = externalMarkup || tbrgRenderSlides(template, resultMap);
  const templateScopeClass = `theme-${tbrgSanitizeClassNamePart(template.id, 'template')}`;
  const scopedCss = tbrgBuildScopedDeckCss(templateScopeClass, template);

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${tbrgEscapeHtml(title)}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/reveal.js@5/dist/reveal.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/reveal.js@5/dist/theme/white.css">
  <style>
    .reveal section img {
      border: 0;
      box-shadow: none;
      background: transparent;
      max-width: 100%;
      height: auto;
    }
    .reveal section {
      text-align: left;
    }
    ${scopedCss}
  </style>
</head>
<body>
  <div class="reveal ${templateScopeClass}">
    <div class="slides">
      ${slideMarkup}
    </div>
  </div>
  <script src="https://cdn.jsdelivr.net/npm/reveal.js@5/dist/reveal.js"></script>
  <script>
    Reveal.initialize({
      hash: true,
      slideNumber: true
    });
  </script>
</body>
</html>`;
}
