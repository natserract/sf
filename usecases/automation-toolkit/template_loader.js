const TBRG_CUSTOM_TEMPLATE_STORAGE_KEY = 'tbrg.customTemplateText';
const TBRG_SELECTED_TEMPLATE_STORAGE_KEY = 'tbrg.selectedTemplateId';
const TBRG_BUNDLED_TEMPLATE_FILES = [
  'templates/default-sales.json'
];

async function tbrgFetchText(relativePath) {
  const response = await fetch(chrome.runtime.getURL(relativePath), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Failed to load template asset: ${relativePath}`);
  }
  return response.text();
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

  const frameResolveTimeoutMs = Number(template.frameResolveTimeoutMs);
  const normalizedFrameResolveTimeoutMs =
    Number.isFinite(frameResolveTimeoutMs) && frameResolveTimeoutMs > 0 ? frameResolveTimeoutMs : 0;

  return {
    id: template.id,
    name: template.name || template.id,
    page: template.page || '',
    frameSelector: typeof template.frameSelector === 'string' ? template.frameSelector : '',
    frameUrlIncludes: typeof template.frameUrlIncludes === 'string' ? template.frameUrlIncludes : '',
    frameResolveTimeoutMs: normalizedFrameResolveTimeoutMs,
    steps: normalizedSteps,
    tasks: normalizedTasks,
    slides: Array.isArray(template.slides) ? template.slides : [],
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
  const templates = [];
  for (const relativePath of TBRG_BUNDLED_TEMPLATE_FILES) {
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
