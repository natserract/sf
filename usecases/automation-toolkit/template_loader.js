const TBRG_CUSTOM_TEMPLATE_STORAGE_KEY = 'tbrg.customTemplateText';
const TBRG_SELECTED_TEMPLATE_STORAGE_KEY = 'tbrg.selectedTemplateId';
const TBRG_TEMPLATE_INDEX_FILE = 'templates/index.json';

const TBRG_ALLOWED_STEP_TYPES = new Set(['text', 'csv', 'waitFor', 'image', 'dom', 'network']);

async function tbrgFetchText(relativePath) {
  const response = await fetch(chrome.runtime.getURL(relativePath), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Failed to load template asset: ${relativePath}`);
  }
  return response.text();
}

async function tbrgLoadTemplateIndex() {
  let parsed;
  try {
    const raw = await tbrgFetchText(TBRG_TEMPLATE_INDEX_FILE);
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new Error(`Failed to load template index "${TBRG_TEMPLATE_INDEX_FILE}": ${error.message || String(error)}`);
  }

  const entries = Array.isArray(parsed)
    ? parsed
    : (Array.isArray(parsed?.templates) ? parsed.templates : null);
  if (!entries) {
    throw new Error(`Template index "${TBRG_TEMPLATE_INDEX_FILE}" must be an array or { templates: [] }.`);
  }

  const normalized = [];
  for (const entry of entries) {
    if (typeof entry !== 'string') {
      continue;
    }
    const p = entry.trim().replace(/^\.?\//, '');
    if (!p || !p.startsWith('templates/') || !p.endsWith('/config.json')) {
      continue;
    }
    normalized.push(p);
  }

  return [...new Set(normalized)];
}

function tbrgValidateStep(step, templateId, source, path) {
  if (!step || typeof step !== 'object') {
    throw new Error(`Template "${templateId}" has invalid step at ${path} in ${source}.`);
  }
  const type = step.type;
  if (typeof type !== 'string' || !type.trim()) {
    throw new Error(`Template "${templateId}" step at ${path} is missing a "type" string in ${source}.`);
  }
  if (!TBRG_ALLOWED_STEP_TYPES.has(type)) {
    throw new Error(`Template "${templateId}" has unsupported step type "${type}" at ${path} in ${source}.`);
  }

  const operator = typeof step.operator === 'string' ? step.operator.trim() : '';
  if (!operator) {
    throw new Error(`Template "${templateId}" step "${step.id || path}" must include an "operator" string in ${source}.`);
  }
  const stepValue = typeof step.value === 'string' ? step.value.trim() : '';

  if (type !== 'waitFor' && (typeof step.id !== 'string' || !step.id.trim())) {
    throw new Error(`Template "${templateId}" step type "${type}" at ${path} requires an "id" string in ${source}.`);
  }

  if (Object.prototype.hasOwnProperty.call(step, 'label') && typeof step.label !== 'string') {
    throw new Error(`Template "${templateId}" step "${step.id || path}" has non-string "label" at ${path} in ${source}.`);
  }
  if (Object.prototype.hasOwnProperty.call(step, 'helpText') && typeof step.helpText !== 'string') {
    throw new Error(`Template "${templateId}" step "${step.id || path}" has non-string "helpText" at ${path} in ${source}.`);
  }

  if (type === 'waitFor') {
    if (operator !== 'exists') {
      throw new Error(`Template "${templateId}" step "${step.id || path}" waitFor supports only operator "exists" in ${source}.`);
    }
    if (typeof step.selector !== 'string' || !step.selector.trim()) {
      throw new Error(`Template "${templateId}" waitFor.exists step "${step.id || path}" requires "selector" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'requireVisible') && typeof step.requireVisible !== 'boolean') {
      throw new Error(`Template "${templateId}" waitFor.exists step "${step.id || path}" has non-boolean "requireVisible" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'value')) {
      throw new Error(`Template "${templateId}" waitFor.exists step "${step.id || path}" must not define "value" in ${source}.`);
    }
  }

  if (type === 'text') {
    if (operator !== 'input') {
      throw new Error(`Template "${templateId}" step "${step.id}" text supports only operator "input" in ${source}.`);
    }
    if (!stepValue) {
      throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'multiline') && typeof step.multiline !== 'boolean') {
      throw new Error(`Template "${templateId}" step "${step.id}" has non-boolean "multiline" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'placeholder') && typeof step.placeholder !== 'string') {
      throw new Error(`Template "${templateId}" step "${step.id}" has non-string "placeholder" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'default') && typeof step.default !== 'string') {
      throw new Error(`Template "${templateId}" step "${step.id}" has non-string "default" in ${source}.`);
    }
  }

  if (type === 'csv') {
    if (operator !== 'input' && operator !== 'aggregate') {
      throw new Error(`Template "${templateId}" step "${step.id}" csv supports operators "input" and "aggregate" in ${source}.`);
    }

    if (operator === 'input') {
      if (!stepValue) {
        throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'delimiter') && typeof step.delimiter !== 'string') {
        throw new Error(`Template "${templateId}" step "${step.id}" has non-string "delimiter" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'hasHeader') && typeof step.hasHeader !== 'boolean') {
        throw new Error(`Template "${templateId}" step "${step.id}" has non-boolean "hasHeader" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'maxRows') && !(Number.isFinite(Number(step.maxRows)) && Number(step.maxRows) >= 1)) {
        throw new Error(`Template "${templateId}" step "${step.id}" has invalid "maxRows" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'maxBytes') && !(Number.isFinite(Number(step.maxBytes)) && Number(step.maxBytes) >= 1)) {
        throw new Error(`Template "${templateId}" step "${step.id}" has invalid "maxBytes" in ${source}.`);
      }
    }

    if (operator === 'aggregate') {
      if (!stepValue) {
        throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
      }
      if (typeof step.sourceValue !== 'string' || !step.sourceValue.trim()) {
        throw new Error(`Template "${templateId}" csv.aggregate step "${step.id}" requires "sourceValue" in ${source}.`);
      }
      if (typeof step.aggregateType !== 'string' || !step.aggregateType.trim()) {
        throw new Error(`Template "${templateId}" csv.aggregate step "${step.id}" requires "aggregateType" in ${source}.`);
      }
      if (step.aggregateType !== 'sum') {
        throw new Error(`Template "${templateId}" csv.aggregate step "${step.id}" currently supports aggregateType "sum" only in ${source}.`);
      }
      if (typeof step.column !== 'string' || !step.column.trim()) {
        throw new Error(`Template "${templateId}" csv.aggregate step "${step.id}" requires "column" in ${source}.`);
      }
    }
  }

  if (type === 'dom') {
    if (operator !== 'readText') {
      throw new Error(`Template "${templateId}" step "${step.id}" dom supports only operator "readText" in ${source}.`);
    }
    if (!stepValue) {
      throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
    }
    if (typeof step.selector !== 'string' || !step.selector.trim()) {
      throw new Error(`Template "${templateId}" dom.readText step "${step.id}" requires "selector" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'requireVisible') && typeof step.requireVisible !== 'boolean') {
      throw new Error(`Template "${templateId}" dom.readText step "${step.id}" has non-boolean "requireVisible" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'textMode')) {
      const tm = typeof step.textMode === 'string' ? step.textMode.trim() : '';
      if (tm !== 'innerText' && tm !== 'textContent') {
        throw new Error(`Template "${templateId}" dom.readText step "${step.id}" has invalid "textMode" (use "innerText" or "textContent") in ${source}.`);
      }
    }
    if (
      Object.prototype.hasOwnProperty.call(step, 'matchIndex') &&
      !(Number.isFinite(Number(step.matchIndex)) && Number(step.matchIndex) >= 0)
    ) {
      throw new Error(`Template "${templateId}" dom.readText step "${step.id}" has invalid "matchIndex" in ${source}.`);
    }
  }

  if (type === 'image') {
    if (operator !== 'capture' && operator !== 'input') {
      throw new Error(`Template "${templateId}" step "${step.id}" image supports operators "capture" and "input" in ${source}.`);
    }
    if (operator === 'capture') {
      if (!stepValue) {
        throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
      }
      if (typeof step.selector !== 'string' || !step.selector.trim()) {
        throw new Error(`Template "${templateId}" image.capture step "${step.id}" requires "selector" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'readySelector') && typeof step.readySelector !== 'string') {
        throw new Error(`Template "${templateId}" image.capture step "${step.id}" has non-string "readySelector" in ${source}.`);
      }
      if (
        Object.prototype.hasOwnProperty.call(step, 'readyTimeoutMs') &&
        !(Number.isFinite(Number(step.readyTimeoutMs)) && Number(step.readyTimeoutMs) > 0)
      ) {
        throw new Error(`Template "${templateId}" image.capture step "${step.id}" has invalid "readyTimeoutMs" in ${source}.`);
      }
      if (
        Object.prototype.hasOwnProperty.call(step, 'readyMatchIndex') &&
        !(Number.isFinite(Number(step.readyMatchIndex)) && Number(step.readyMatchIndex) >= 0)
      ) {
        throw new Error(`Template "${templateId}" image.capture step "${step.id}" has invalid "readyMatchIndex" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'readyRequireVisible') && typeof step.readyRequireVisible !== 'boolean') {
        throw new Error(`Template "${templateId}" image.capture step "${step.id}" has non-boolean "readyRequireVisible" in ${source}.`);
      }
    }
    if (operator === 'input') {
      if (!stepValue) {
        throw new Error(`Template "${templateId}" step "${step.id}" requires non-empty "value" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'accept') && typeof step.accept !== 'string') {
        throw new Error(`Template "${templateId}" step "${step.id}" has non-string "accept" in ${source}.`);
      }
      if (Object.prototype.hasOwnProperty.call(step, 'maxBytes') && !(Number.isFinite(Number(step.maxBytes)) && Number(step.maxBytes) >= 1)) {
        throw new Error(`Template "${templateId}" step "${step.id}" has invalid "maxBytes" in ${source}.`);
      }
    }
  }

  if (type === 'network') {
    if (operator !== 'saveUrl') {
      throw new Error(`Template "${templateId}" step "${step.id}" network supports only operator "saveUrl" in ${source}.`);
    }
    const urlRaw = typeof step.url === 'string' ? step.url.trim() : '';
    if (!urlRaw) {
      throw new Error(`Template "${templateId}" network.saveUrl step "${step.id}" requires "url" in ${source}.`);
    }
    let parsedUrl;
    try {
      parsedUrl = new URL(urlRaw);
    } catch (_error) {
      throw new Error(`Template "${templateId}" network.saveUrl step "${step.id}" has invalid "url" in ${source}.`);
    }
    if (parsedUrl.protocol !== 'https:') {
      throw new Error(`Template "${templateId}" network.saveUrl step "${step.id}" url must use https in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'downloadBasename') && typeof step.downloadBasename !== 'string') {
      throw new Error(`Template "${templateId}" network.saveUrl step "${step.id}" has non-string "downloadBasename" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'value') && typeof step.value !== 'string') {
      throw new Error(`Template "${templateId}" network.saveUrl step "${step.id}" has non-string "value" in ${source}.`);
    }
    if (Object.prototype.hasOwnProperty.call(step, 'downloadFileExtension')) {
      const rawExt = typeof step.downloadFileExtension === 'string' ? step.downloadFileExtension.trim() : '';
      const ext = rawExt.replace(/^\.+/, '');
      if (!/^[a-zA-Z0-9]{1,15}$/.test(ext)) {
        throw new Error(
          `Template "${templateId}" network.saveUrl step "${step.id}" has invalid "downloadFileExtension" (use letters/digits only, e.g. "json" or "csv") in ${source}.`
        );
      }
    }
  }

  if (Object.prototype.hasOwnProperty.call(step, 'timeoutMs')) {
    const timeoutMs = Number(step.timeoutMs);
    if (!(Number.isFinite(timeoutMs) && timeoutMs > 0)) {
      throw new Error(`Template "${templateId}" step at ${path} has invalid "timeoutMs" in ${source}.`);
    }
  }
}

function tbrgNormalizeDeckStyle(deckStyle, templateId, source) {
  if (!deckStyle) {
    return null;
  }
  if (typeof deckStyle !== 'object' || Array.isArray(deckStyle)) {
    throw new Error(`Template "${templateId}" has invalid "deckStyle" in ${source}.`);
  }
  const normalized = {};
  for (const [key, value] of Object.entries(deckStyle)) {
    if (value == null) {
      continue;
    }
    if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') {
      throw new Error(`Template "${templateId}" deckStyle key "${key}" must be primitive in ${source}.`);
    }
    normalized[key] = value;
  }
  return normalized;
}

function tbrgNormalizeEmbedUrl(templateId, embedUrlRaw, source) {
  const embedUrl = typeof embedUrlRaw === 'string' ? embedUrlRaw.trim() : '';
  if (!embedUrl) {
    throw new Error(`Template "${templateId}" requires non-empty "embedUrl" in ${source}.`);
  }
  if (!embedUrl.startsWith('https://')) {
    throw new Error(`Template "${templateId}" embedUrl must use https in ${source}.`);
  }
  let parsed;
  try {
    parsed = new URL(embedUrl);
  } catch (_error) {
    throw new Error(`Template "${templateId}" embedUrl is not a valid URL in ${source}.`);
  }
  if (parsed.protocol !== 'https:') {
    throw new Error(`Template "${templateId}" embedUrl must use https in ${source}.`);
  }
  return embedUrl;
}

function tbrgNormalizeTemplate(template, source) {
  if (!template || typeof template !== 'object') {
    throw new Error(`Invalid template object from ${source}`);
  }

  if (!template.id || typeof template.id !== 'string') {
    throw new Error(`Template in ${source} is missing an "id" string.`);
  }

  const hasSteps = Array.isArray(template.steps);
  const hasTasks = Array.isArray(template.tasks);
  const runMode = typeof template.runMode === 'string' ? template.runMode.trim() : '';

  if (runMode === 'embed') {
    if (hasSteps || hasTasks) {
      throw new Error(`Template "${template.id}" runMode "embed" must not define "steps" or "tasks" in ${source}.`);
    }
    const embedUrl = tbrgNormalizeEmbedUrl(template.id, template.embedUrl, source);
    return {
      id: template.id,
      name: template.name || template.id,
      runMode: 'embed',
      embedUrl,
      page: '',
      frameSelector: '',
      frameUrlIncludes: '',
      frameResolveTimeoutMs: 0,
      steps: [],
      tasks: null,
      slides: Array.isArray(template.slides) ? template.slides : [],
      themeId: '',
      deckStyle: null,
      deckCss: '',
      slideLayouts: null,
      slidesHtmlFileResolved: '',
      source
    };
  }

  if (!hasSteps && !hasTasks) {
    throw new Error(`Template "${template.id}" must include either "steps" or "tasks".`);
  }

  const normalizedTasks = hasTasks
    ? template.tasks.map((task, index) => {
      if (!task || typeof task !== 'object') {
        throw new Error(`Template "${template.id}" task at index ${index} is invalid.`);
      }
      if (!Array.isArray(task.steps)) {
        throw new Error(`Template "${template.id}" task "${task.id || index}" must include a "steps" array.`);
      }

      task.steps.forEach((step, stepIndex) => {
        tbrgValidateStep(step, template.id, source, `tasks[${index}].steps[${stepIndex}]`);
      });

      const normalizedTask = {
        id: typeof task.id === 'string' ? task.id : `task_${index + 1}`,
        name: task.name || task.id || `Task ${index + 1}`,
        steps: task.steps
      };

      if (Object.prototype.hasOwnProperty.call(task, 'page')) {
        normalizedTask.page = typeof task.page === 'string' ? task.page : '';
      }
      if (Object.prototype.hasOwnProperty.call(task, 'frameSelector')) {
        normalizedTask.frameSelector = typeof task.frameSelector === 'string' ? task.frameSelector : '';
      }
      if (Object.prototype.hasOwnProperty.call(task, 'frameUrlIncludes')) {
        normalizedTask.frameUrlIncludes = typeof task.frameUrlIncludes === 'string' ? task.frameUrlIncludes : '';
      }
      if (Object.prototype.hasOwnProperty.call(task, 'frameResolveTimeoutMs')) {
        const v = Number(task.frameResolveTimeoutMs);
        normalizedTask.frameResolveTimeoutMs = Number.isFinite(v) && v > 0 ? v : 0;
      }

      return normalizedTask;
    })
    : null;

  const normalizedSteps = hasSteps
    ? template.steps
    : normalizedTasks.flatMap((task) => task.steps);

  if (hasSteps) {
    template.steps.forEach((step, stepIndex) => {
      tbrgValidateStep(step, template.id, source, `steps[${stepIndex}]`);
    });
  }

  const frameResolveTimeoutMs = Number(template.frameResolveTimeoutMs);
  const normalizedFrameResolveTimeoutMs =
    Number.isFinite(frameResolveTimeoutMs) && frameResolveTimeoutMs > 0 ? frameResolveTimeoutMs : 0;
  const themeId = typeof template.themeId === 'string' ? template.themeId.trim() : '';
  const deckCss = typeof template.deckCss === 'string' ? template.deckCss : '';
  const slideLayouts = template.slideLayouts && typeof template.slideLayouts === 'object' && !Array.isArray(template.slideLayouts)
    ? template.slideLayouts
    : null;
  if (template.slideLayouts && !slideLayouts) {
    throw new Error(`Template "${template.id}" has invalid "slideLayouts" in ${source}.`);
  }
  const deckStyle = tbrgNormalizeDeckStyle(template.deckStyle, template.id, source);
  const slidesHtmlFileResolved = `templates/${template.id}/template.html`;

  const embedUrlAfter =
    runMode === 'embedAfter' ? tbrgNormalizeEmbedUrl(template.id, template.embedUrl, source) : '';

  return {
    id: template.id,
    name: template.name || template.id,
    runMode: runMode === 'embedAfter' ? 'embedAfter' : runMode || '',
    embedUrl: embedUrlAfter,
    page: template.page || '',
    frameSelector: typeof template.frameSelector === 'string' ? template.frameSelector : '',
    frameUrlIncludes: typeof template.frameUrlIncludes === 'string' ? template.frameUrlIncludes : '',
    frameResolveTimeoutMs: normalizedFrameResolveTimeoutMs,
    steps: normalizedSteps,
    tasks: normalizedTasks,
    slides: Array.isArray(template.slides) ? template.slides : [],
    themeId,
    deckStyle,
    deckCss,
    slideLayouts,
    slidesHtmlFileResolved,
    source
  };
}

function tbrgParseTemplateText(templateText, source) {
  const rawText = (templateText || '').trim();
  if (!rawText) {
    throw new Error('Template text is empty.');
  }

  let parsed;
  if (rawText.startsWith('{') || rawText.startsWith('[')) {
    parsed = JSON.parse(rawText);
  } else {
    if (!self.jsyaml || typeof self.jsyaml.load !== 'function') {
      throw new Error('YAML parser is not available.');
    }
    parsed = self.jsyaml.load(rawText);
  }

  return tbrgNormalizeTemplate(parsed, source);
}

async function tbrgLoadBundledTemplates() {
  const templateFiles = await tbrgLoadTemplateIndex();
  const templates = [];
  for (const relativePath of templateFiles) {
    const templateText = await tbrgFetchText(relativePath);
    templates.push(tbrgParseTemplateText(templateText, relativePath));
  }
  return templates;
}

function tbrgStorageGet(keys) {
  return new Promise((resolve) => {
    chrome.storage.local.get(keys, resolve);
  });
}

function tbrgStorageSet(values) {
  return new Promise((resolve) => {
    chrome.storage.local.set(values, resolve);
  });
}

async function tbrgGetCustomTemplateState() {
  const result = await tbrgStorageGet([
    TBRG_CUSTOM_TEMPLATE_STORAGE_KEY,
    TBRG_SELECTED_TEMPLATE_STORAGE_KEY
  ]);

  return {
    customTemplateText: result[TBRG_CUSTOM_TEMPLATE_STORAGE_KEY] || '',
    selectedTemplateId: result[TBRG_SELECTED_TEMPLATE_STORAGE_KEY] || ''
  };
}

async function tbrgSaveCustomTemplateText(templateText) {
  const trimmedText = (templateText || '').trim();

  if (trimmedText) {
    tbrgParseTemplateText(trimmedText, 'custom template');
  }

  await tbrgStorageSet({
    [TBRG_CUSTOM_TEMPLATE_STORAGE_KEY]: trimmedText
  });
}

async function tbrgSetSelectedTemplateId(templateId) {
  await tbrgStorageSet({
    [TBRG_SELECTED_TEMPLATE_STORAGE_KEY]: templateId || ''
  });
}

async function tbrgListTemplates() {
  const bundledTemplates = await tbrgLoadBundledTemplates();
  const { customTemplateText, selectedTemplateId } = await tbrgGetCustomTemplateState();
  const templates = [...bundledTemplates];

  if (customTemplateText) {
    templates.unshift(tbrgParseTemplateText(customTemplateText, 'custom template'));
  }

  let effectiveSelectedTemplateId = selectedTemplateId;
  if (!templates.some((template) => template.id === effectiveSelectedTemplateId)) {
    effectiveSelectedTemplateId = templates[0]?.id || '';
  }

  return {
    templates,
    selectedTemplateId: effectiveSelectedTemplateId,
    customTemplateText
  };
}

async function tbrgResolveTemplateById(templateId) {
  const { templates } = await tbrgListTemplates();
  const template = templates.find((item) => item.id === templateId);
  if (!template) {
    throw new Error(`Unknown template: ${templateId}`);
  }
  return template;
}
