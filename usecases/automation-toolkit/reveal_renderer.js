function tbrgEscapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function tbrgInterpolate(templateText, resultMap) {
  return String(templateText).replace(/\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}/g, (_match, key) => {
    const value = resultMap[key];
    if (typeof value === 'string' && value.startsWith('data:image/')) {
      return `<img src="${value}" alt="${tbrgEscapeHtml(key)}" style="max-width: 100%; max-height: 480px;" />`;
    }
    return tbrgEscapeHtml(value ?? '');
  });
}

function tbrgBuildDefaultSlides(resultMap) {
  const entries = Object.entries(resultMap);
  const slides = [
    {
      title: 'Report Summary',
      body: entries.length
        ? `<ul>${entries.map(([key, value]) => `<li><strong>${tbrgEscapeHtml(key)}:</strong> ${typeof value === 'string' && value.startsWith('data:image/') ? '[image]' : tbrgEscapeHtml(value)}</li>`).join('')}</ul>`
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
      body: `<p style="font-size: 2rem;">${tbrgEscapeHtml(value)}</p>`
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

function tbrgBuildRevealDeckHtml(template, resultMap) {
  const title = template.name || template.id;
  const slideMarkup = tbrgRenderSlides(template, resultMap);

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
    }
  </style>
</head>
<body>
  <div class="reveal">
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
