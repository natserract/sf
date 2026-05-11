const templateSelectEl = document.getElementById('templateSelect');
const templateMetaEl = document.getElementById('templateMeta');
const runButtonEl = document.getElementById('runButton');
const stopButtonEl = document.getElementById('stopButton');
const closePopupButtonEl = document.getElementById('closePopupButton');
const statusBoxEl = document.getElementById('statusBox');
const debugModeToggleEl = document.getElementById('debugModeToggle');
const domPickerCardEl = document.getElementById('domPickerCard');
const pickElementButtonEl = document.getElementById('pickElementButton');
const pickerPreviewEl = document.getElementById('pickerPreview');
const insertTextStepButtonEl = document.getElementById('insertTextStepButton');
const insertScreenshotStepButtonEl = document.getElementById('insertScreenshotStepButton');
const progressTaskTextEl = document.getElementById('progressTaskText');
const progressStepTextEl = document.getElementById('progressStepText');
const progressBarFillEl = document.getElementById('progressBarFill');
const progressStateTextEl = document.getElementById('progressStateText');

let pickedElementPayload = null;
const TBRG_DEBUG_MODE_STORAGE_KEY = 'tbrg.debugModeEnabled';
let latestProgress = {
  completedTasks: 0,
  totalTasks: 0,
  completedSteps: 0,
  totalSteps: 0
};
let ignoreProgressUpdates = false;

function setStatus(message, tone) {
  statusBoxEl.textContent = message;
  statusBoxEl.classList.remove('error', 'success');
  if (tone) {
    statusBoxEl.classList.add(tone);
  }
}

function tbrgSafeNumber(value) {
  const n = Number(value);
  return Number.isFinite(n) && n >= 0 ? n : 0;
}

function updateProgressUi(progress, stateText) {
  latestProgress = {
    completedTasks: tbrgSafeNumber(progress?.completedTasks),
    totalTasks: tbrgSafeNumber(progress?.totalTasks),
    completedSteps: tbrgSafeNumber(progress?.completedSteps),
    totalSteps: tbrgSafeNumber(progress?.totalSteps)
  };

  progressTaskTextEl.textContent = `Tasks: ${latestProgress.completedTasks}/${latestProgress.totalTasks}`;
  progressStepTextEl.textContent = `Steps: ${latestProgress.completedSteps}/${latestProgress.totalSteps}`;

  const denominator = latestProgress.totalSteps;
  const numerator = latestProgress.completedSteps;
  const percent = denominator > 0 ? Math.max(0, Math.min(100, Math.round((numerator / denominator) * 100))) : 0;
  progressBarFillEl.style.width = `${percent}%`;

  const isDone = latestProgress.totalTasks > 0 && latestProgress.completedTasks >= latestProgress.totalTasks;
  progressBarFillEl.classList.toggle('done', isDone);
  progressStateTextEl.textContent = stateText || (isDone ? 'Finished.' : 'Running...');
}

function setIdleProgressUi() {
  updateProgressUi({ completedTasks: 0, totalTasks: 0, completedSteps: 0, totalSteps: 0 }, 'Idle.');
  progressBarFillEl.classList.remove('done');
}

function sendMessage(message, timeoutMs = 90000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`Timed out after ${Math.round(timeoutMs / 1000)}s while waiting for extension response.`));
    }, timeoutMs);

    chrome.runtime.sendMessage(message, (response) => {
      clearTimeout(timer);

      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }

      if (!response) {
        reject(new Error('No response from extension background service.'));
        return;
      }

      resolve(response);
    });
  });
}

function renderTemplateOptions(templates, selectedTemplateId) {
  templateSelectEl.innerHTML = '';

  const sorted = [...templates].sort((a, b) => {
    const la = String(a.name || a.id || '').toLowerCase();
    const lb = String(b.name || b.id || '').toLowerCase();
    return la.localeCompare(lb, undefined, { sensitivity: 'base' });
  });

  if (sorted.length === 0) {
    templateSelectEl.disabled = true;
    runButtonEl.disabled = true;
    templateMetaEl.textContent = 'No templates found (config.json).';
    return;
  }

  templateSelectEl.disabled = false;
  runButtonEl.disabled = false;

  sorted.forEach((template) => {
    const option = document.createElement('option');
    option.value = template.id;
    option.textContent = `${template.name || template.id}`;
    if (template.id === selectedTemplateId) {
      option.selected = true;
    }
    templateSelectEl.appendChild(option);
  });

  updateTemplateMeta(templates);
}

function updateTemplateMeta(templates) {
  const currentTemplate = templates.find((template) => template.id === templateSelectEl.value);
  if (!currentTemplate) {
    templateMetaEl.textContent = 'No template selected.';
    return;
  }

  if (currentTemplate.runMode === 'embed') {
    templateMetaEl.textContent = 'Embedded tool | Opens as overlay on the current tab';
    return;
  }

  if (currentTemplate.runMode === 'embedAfter') {
    let stepCount = 0;
    if (Array.isArray(currentTemplate.tasks) && currentTemplate.tasks.length > 0) {
      stepCount = currentTemplate.tasks.reduce(
        (acc, task) => acc + (Array.isArray(task.steps) ? task.steps.length : 0),
        0
      );
    } else if (Array.isArray(currentTemplate.steps)) {
      stepCount = currentTemplate.steps.length;
    }
    templateMetaEl.textContent = `Automation then embedded tool | ${currentTemplate.page || 'Per-task page'} | ${stepCount} step${stepCount === 1 ? '' : 's'}`;
    return;
  }

  const stepCount = Array.isArray(currentTemplate.steps) ? currentTemplate.steps.length : 0;
  templateMetaEl.textContent = `${currentTemplate.page || 'Current page'} | ${stepCount} step${stepCount === 1 ? '' : 's'}`;
}

function slugifyStepId(value) {
  const normalized = String(value || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
  return normalized || `picked_${Date.now()}`;
}

function setPickedElement(payload) {
  pickedElementPayload = payload || null;
  const hasPicked = Boolean(pickedElementPayload && pickedElementPayload.selector);
  insertTextStepButtonEl.disabled = !hasPicked;
  insertScreenshotStepButtonEl.disabled = !hasPicked;

  if (!hasPicked) {
    pickerPreviewEl.textContent = 'No element picked yet.';
    return;
  }

  const lines = [
    `Selector: ${pickedElementPayload.selector}`,
    `Match index: ${pickedElementPayload.matchIndex || 0}`,
    `Tag: ${pickedElementPayload.tagName || 'unknown'}`
  ];

  if (pickedElementPayload.textSample) {
    lines.push(`Text sample: ${pickedElementPayload.textSample}`);
  }
  if (pickedElementPayload.frameUrl) {
    lines.push(`Frame URL: ${pickedElementPayload.frameUrl}`);
  }

  pickerPreviewEl.textContent = lines.join('\n');
}

async function tbrgStorageGet(keys) {
  return await new Promise((resolve) => chrome.storage.local.get(keys, resolve));
}

function buildStepSnippet(stepType) {
  if (!pickedElementPayload?.selector) {
    throw new Error('No picked element to insert.');
  }

  const baseId = slugifyStepId(pickedElementPayload.textSample || pickedElementPayload.tagName || 'picked');
  let step;
  if (stepType === 'image') {
    step = {
      id: `${baseId}_image`,
      type: 'image',
      operator: 'capture',
      selector: pickedElementPayload.selector,
      timeoutMs: 60000
    };
  } else if (stepType === 'waitFor') {
    step = {
      type: 'waitFor',
      operator: 'exists',
      selector: pickedElementPayload.selector,
      timeoutMs: 60000
    };
  } else {
    throw new Error(`Unsupported snippet type: ${stepType}`);
  }

  if (Number(pickedElementPayload.matchIndex) > 0) {
    step.matchIndex = Number(pickedElementPayload.matchIndex);
  }

  return `${JSON.stringify(step, null, 2)},\n`;
}

async function copyToClipboard(text) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  // Fallback: hidden textarea + execCommand.
  const el = document.createElement('textarea');
  el.value = text;
  el.style.position = 'fixed';
  el.style.left = '-9999px';
  el.style.top = '0';
  document.body.appendChild(el);
  el.focus();
  el.select();
  document.execCommand('copy');
  el.remove();
}

function setDebugModeEnabled(enabled) {
  const on = Boolean(enabled);
  debugModeToggleEl.checked = on;
  domPickerCardEl.style.display = on ? 'block' : 'none';
  if (!on) {
    setPickedElement(null);
  }
  chrome.storage.local.set({ [TBRG_DEBUG_MODE_STORAGE_KEY]: on });
}

async function loadTemplates() {
  setStatus('Loading templates...');
  const response = await sendMessage({ type: 'TBRG_LIST_TEMPLATES' });

  if (!response.ok) {
    throw new Error(response.error || 'Failed to load templates.');
  }

  renderTemplateOptions(response.templates, response.selectedTemplateId);

  const stored = await tbrgStorageGet([TBRG_DEBUG_MODE_STORAGE_KEY]);
  setDebugModeEnabled(Boolean(stored[TBRG_DEBUG_MODE_STORAGE_KEY]));
  setStatus('Ready.');
}

async function pickElementFromPage() {
  pickElementButtonEl.disabled = true;
  setStatus('Pick mode active: move cursor to the page and click element. Press Escape to cancel.');

  try {
    const response = await sendMessage({ type: 'TBRG_PICK_ELEMENT' }, 120000);
    if (!response.ok) {
      throw new Error(response.error || 'Failed to pick element.');
    }

    setPickedElement(response.pick);
    const debugFileLine = response.debugJsonFilename
      ? `\nDebug JSON downloaded: ${response.debugJsonFilename}`
      : '';
    setStatus(`Element captured. Use insert button to add JSON snippet.${debugFileLine}`, 'success');
  } catch (error) {
    setStatus(error.message || 'Element pick failed.', 'error');
  } finally {
    pickElementButtonEl.disabled = false;
  }
}

function insertStepFromPick(stepType) {
  const snippet = buildStepSnippet(stepType);
  copyToClipboard(snippet)
    .then(() => setStatus(`Copied ${stepType} step JSON to clipboard.`, 'success'))
    .catch((error) => setStatus(error.message || 'Failed to copy snippet.', 'error'));
}

async function runReport() {
  const selectedTemplateId = templateSelectEl.value;
  if (!selectedTemplateId) {
    throw new Error('No template selected.');
  }
  setStatus('Starting report run...');
  updateProgressUi({ completedTasks: 0, totalTasks: 0, completedSteps: 0, totalSteps: 0 }, 'Starting...');
  runButtonEl.disabled = true;

  try {
    const response = await sendMessage({
      type: 'TBRG_START_JOB',
      templateId: selectedTemplateId
    }, 15 * 60 * 1000);

    if (!response.ok) {
      if (response.stopped) {
        ignoreProgressUpdates = true;
        setStatus(response.error || 'Automation stopped by user.');
        setIdleProgressUi();
        return;
      }
      throw new Error(response.error || 'Report run failed.');
    }

    if (response.embedOpened) {
      setStatus(
        [
          `Opened embedded view for: ${response.templateId}`,
          'Use Close at the top of the page to dismiss the overlay.'
        ].join('\n'),
        'success'
      );
    } else {
      const screenshotFiles = Array.isArray(response.screenshotFilenames)
        ? response.screenshotFilenames.join(', ')
        : '';

      const lines = [
        `Finished template: ${response.templateId}`,
        `Extracted keys: ${response.resultKeys.join(', ') || 'none'}`,
        `Deck downloaded as: ${response.downloadFilename}`,
        screenshotFiles ? `Screenshot downloads: ${screenshotFiles}` : '',
        response.warning ? `Warning: ${response.warning}` : ''
      ].filter(Boolean);

      setStatus(lines.join('\n'), 'success');
    }
    updateProgressUi({
      completedTasks: latestProgress.completedTasks,
      totalTasks: latestProgress.totalTasks,
      completedSteps: latestProgress.completedSteps,
      totalSteps: latestProgress.totalSteps
    }, 'Finished.');
    progressBarFillEl.classList.add('done');
  } catch (error) {
    setStatus(error.message || 'Report run failed.', 'error');
    progressStateTextEl.textContent = 'Failed.';
  } finally {
    runButtonEl.disabled = false;
  }
}

async function runReportWithProgress() {
  setStatus('Starting report run...\nTracking progress...');
  await runReport();
}

async function stopAutomation() {
  stopButtonEl.disabled = true;
  ignoreProgressUpdates = true;
  setIdleProgressUi();
  try {
    const response = await sendMessage({ type: 'TBRG_STOP_JOB' });
    if (!response.ok) {
      throw new Error(response.error || 'Failed to stop automation.');
    }
    setStatus('Automation stop requested.');
    setIdleProgressUi();
  } catch (error) {
    setStatus(error.message || 'Failed to stop automation.', 'error');
  } finally {
    stopButtonEl.disabled = false;
  }
}

chrome.runtime.onMessage.addListener((message) => {
  if (message?.type !== 'TBRG_JOB_PROGRESS') {
    return;
  }
  if (message.stage === 'started') {
    ignoreProgressUpdates = false;
  } else if (ignoreProgressUpdates) {
    return;
  }
  const taskSummary = `Tasks: ${tbrgSafeNumber(message.completedTasks)}/${tbrgSafeNumber(message.totalTasks)}`;
  const stepSummary = `Steps: ${tbrgSafeNumber(message.completedSteps)}/${tbrgSafeNumber(message.totalSteps)}`;
  const detail = message.lastCompletedStepId ? `Last step: ${message.lastCompletedStepId}` : '';

  if (message.stage === 'started') {
    updateProgressUi(message, 'Started...');
    setStatus(`Report started.\n${taskSummary}\n${stepSummary}`);
    return;
  }

  if (message.stage === 'running') {
    const taskLine = message.currentTaskName
      ? `Current task: ${message.currentTaskName}`
      : (message.currentTaskId ? `Current task: ${message.currentTaskId}` : '');
    updateProgressUi(message, 'Running...');
    setStatus([taskSummary, stepSummary, taskLine, detail].filter(Boolean).join('\n'));
    return;
  }

  if (message.stage === 'finished') {
    updateProgressUi(message, 'Finished.');
    progressBarFillEl.classList.add('done');
    return;
  }

  if (message.stage === 'error') {
    progressStateTextEl.textContent = 'Failed.';
    setStatus(message.error || 'Report run failed.', 'error');
  }
});

templateSelectEl.addEventListener('change', async () => {
  try {
    const response = await sendMessage({ type: 'TBRG_LIST_TEMPLATES' });
    if (response.ok) {
      updateTemplateMeta(response.templates);
    }
  } catch (error) {
    setStatus(error.message || 'Failed to refresh template details.', 'error');
  }
});

debugModeToggleEl.addEventListener('change', () => setDebugModeEnabled(debugModeToggleEl.checked));

pickElementButtonEl.addEventListener('click', pickElementFromPage);
insertTextStepButtonEl.addEventListener('click', () => insertStepFromPick('waitFor'));
insertScreenshotStepButtonEl.addEventListener('click', () => insertStepFromPick('image'));
runButtonEl.addEventListener('click', runReportWithProgress);
stopButtonEl.addEventListener('click', () => {
  stopAutomation();
  window.close();
});
closePopupButtonEl.addEventListener('click', () => window.close());

setPickedElement(null);
setIdleProgressUi();

loadTemplates().catch((error) => {
  setStatus(error.message || 'Failed to initialize popup.', 'error');
});
