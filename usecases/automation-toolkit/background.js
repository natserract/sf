importScripts('vendor/js-yaml.min.js', 'template_loader.js', 'reveal_renderer.js');

const TBRG_USER_INPUT_TIMEOUT_MS = 15 * 60 * 1000;
let tbrgRunGeneration = 0;
/** When true, debug DOM activity is not appended (avoids logging automated clicks/hovers as user exploration). */
let tbrgAutomationJobInProgress = false;

const TBRG_DEBUG_MODE_STORAGE_KEY = 'tbrg.debugModeEnabled';
const TBRG_DEBUG_DOM_LOG_KEY = 'tbrg.debugDomActivityLog';
const TBRG_DEBUG_EXPORT_RELATIVE_PATH = 'automation-toolkit-debug/dom-activity-log.json';
const TBRG_DEBUG_SCHEMA_VERSION = 1;
const TBRG_DEBUG_MAX_EVENTS = 2000;
const TBRG_DEBUG_RECORDER_SCRIPT_ID = 'tbrg_debug_dom_recorder';

function tbrgStoppedError() {
  const error = new Error('Automation stopped by user.');
  error.code = 'TBRG_STOPPED';
  return error;
}

function tbrgAssertRunActive(runGeneration) {
  if (runGeneration !== tbrgRunGeneration) {
    throw tbrgStoppedError();
  }
}

function tbrgGetActiveTab() {
  return new Promise((resolve, reject) => {
    chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }

      if (!tabs[0]?.id) {
        reject(new Error('No active tab found.'));
        return;
      }

      resolve(tabs[0]);
    });
  });
}

function tbrgUpdateTab(tabId, updateProperties) {
  return new Promise((resolve, reject) => {
    chrome.tabs.update(tabId, updateProperties, (tab) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      resolve(tab);
    });
  });
}

function tbrgWaitForTabComplete(tabId, timeoutMs) {
  return new Promise((resolve, reject) => {
    const timeoutId = setTimeout(() => {
      chrome.tabs.onUpdated.removeListener(handleUpdate);
      reject(new Error('Timed out waiting for navigation to complete.'));
    }, timeoutMs);

    function handleUpdate(updatedTabId, changeInfo, tab) {
      if (updatedTabId === tabId && changeInfo.status === 'complete') {
        clearTimeout(timeoutId);
        chrome.tabs.onUpdated.removeListener(handleUpdate);
        resolve(tab);
      }
    }

    chrome.tabs.onUpdated.addListener(handleUpdate);
    chrome.tabs.get(tabId, (tab) => {
      if (chrome.runtime.lastError) {
        return;
      }

      if (tab?.status === 'complete') {
        clearTimeout(timeoutId);
        chrome.tabs.onUpdated.removeListener(handleUpdate);
        resolve(tab);
      }
    });
  });
}

async function tbrgEnsureTargetPage(template) {
  const activeTab = await tbrgGetActiveTab();
  const targetUrl = (template.page || '').trim();

  if (!targetUrl) {
    return activeTab;
  }

  if (activeTab.url === targetUrl) {
    return activeTab;
  }

  const updatedTab = await tbrgUpdateTab(activeTab.id, { url: targetUrl, active: true });
  await tbrgWaitForTabComplete(updatedTab.id, 30000);
  return updatedTab;
}

function tbrgEffectivePageForTask(template, task) {
  if (!Object.prototype.hasOwnProperty.call(task, 'page')) {
    return (template.page || '').trim();
  }
  const trimmed = typeof task.page === 'string' ? task.page.trim() : '';
  return trimmed || (template.page || '').trim();
}

function tbrgFrameContextForTask(template, task) {
  const frameSelector = Object.prototype.hasOwnProperty.call(task, 'frameSelector')
    ? (typeof task.frameSelector === 'string' ? task.frameSelector.trim() : '')
    : String(template.frameSelector || '').trim();

  const frameUrlIncludes = Object.prototype.hasOwnProperty.call(task, 'frameUrlIncludes')
    ? (typeof task.frameUrlIncludes === 'string' ? task.frameUrlIncludes.trim() : '')
    : String(template.frameUrlIncludes || '').trim();

  let frameResolveTimeoutMs = Number(template.frameResolveTimeoutMs) > 0 ? Number(template.frameResolveTimeoutMs) : 0;
  if (Object.prototype.hasOwnProperty.call(task, 'frameResolveTimeoutMs')) {
    const v = Number(task.frameResolveTimeoutMs);
    frameResolveTimeoutMs = Number.isFinite(v) && v > 0 ? v : 0;
  }

  return {
    frameSelector,
    frameUrlIncludes,
    frameResolveTimeoutMs
  };
}

function tbrgPickStepsArray(template) {
  if (Array.isArray(template.tasks) && template.tasks.length > 0) {
    return null;
  }
  return Array.isArray(template.steps) ? template.steps : [];
}

async function tbrgExecuteTasksSequentially(template, onProgress, runGeneration, progressTabId) {
  const merged = {
    ok: true,
    results: {},
    stepResults: [],
    taskResults: [],
    url: '',
    title: ''
  };

  const totalTasks = template.tasks.length;
  let completedTasks = 0;
  let completedSteps = 0;
  const totalStepsAll = template.tasks.reduce(
    (acc, t) => acc + (Array.isArray(t.steps) ? t.steps.length : 0),
    0
  );

  for (const task of template.tasks) {
    const stepsOffsetBeforeTask = completedSteps;
    const tasksFinishedBefore = completedTasks;
    tbrgAssertRunActive(runGeneration);
    const pageForNav = tbrgEffectivePageForTask(template, task);
    const targetTab = await tbrgEnsureTargetPage({ ...template, page: pageForNav });
    tbrgAssertRunActive(runGeneration);
    await tbrgExecuteScript(targetTab.id, ['content_runner.js'], true);

    const frameCtx = tbrgFrameContextForTask(template, task);
    const frameTemplate = { ...template, ...frameCtx };

    const targetFrameId = await tbrgResolveTargetFrameId(targetTab.id, frameTemplate);

    await tbrgExecuteScriptInFrame(targetTab.id, ['content_runner.js'], targetFrameId);
    tbrgAssertRunActive(runGeneration);
    const partial = await tbrgSendTabMessageWithFrameBootstrap(targetTab.id, targetFrameId, {
      type: 'TBRG_EXECUTE_TEMPLATE',
      template: { tasks: [task], slides: [] },
      progressReporting: {
        templateId: template.id,
        progressTabId,
        completedStepsOffset: stepsOffsetBeforeTask,
        totalStepsOverall: totalStepsAll,
        completedTasks: tasksFinishedBefore,
        totalTasks,
        currentTaskId: task.id || null,
        currentTaskName: task.name || null
      }
    }, TBRG_USER_INPUT_TIMEOUT_MS);
    tbrgAssertRunActive(runGeneration);

    if (!partial?.ok) {
      throw new Error(partial?.error || 'Content execution failed.');
    }

    Object.assign(merged.results, partial.results || {});
    merged.stepResults.push(...(partial.stepResults || []));
    merged.taskResults.push(...(partial.taskResults || []));
    merged.url = partial.url || merged.url;
    merged.title = partial.title || merged.title;

    const taskStepTotal = Array.isArray(task.steps) ? task.steps.length : 0;
    const taskStepCompleted = (partial.stepResults || []).filter((step) => step.ok).length;
    completedTasks += 1;
    completedSteps += taskStepCompleted;
    if (typeof onProgress === 'function') {
      await onProgress({
        stage: 'running',
        completedTasks,
        totalTasks,
        completedSteps,
        currentTaskId: task.id || null,
        currentTaskName: task.name || null,
        lastCompletedStepId: (partial.stepResults || []).filter((step) => step.ok).slice(-1)[0]?.id || null,
        taskStepTotal,
        taskStepCompleted
      });
    }

    const failedSteps = (partial.stepResults || []).filter((step) => !step.ok);
    if (failedSteps.length > 0) {
      const failedSummary = failedSteps
        .slice(0, 3)
        .map((step) => `${step.id || 'unknown'}: ${step.error || 'step failed'}`)
        .join('; ');
      throw new Error(
        `Template execution failed (${failedSteps.length} step failure${failedSteps.length === 1 ? '' : 's'}): ${failedSummary}`
      );
    }
  }

  return merged;
}

function tbrgDownloadDataUrlAsPng(dataUrl, basename) {
  return new Promise((resolve, reject) => {
    const safeBase = (basename || 'screenshot').replace(/[^\w\-]+/g, '_');
    const filename = `${safeBase}-${Date.now()}.png`;

    chrome.downloads.download(
      {
        url: dataUrl,
        filename,
        saveAs: false,
        conflictAction: 'uniquify'
      },
      (downloadId) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve({ downloadId, filename });
      }
    );
  });
}

async function tbrgDownloadScreenshotExports(template, results) {
  if (!results || typeof results !== 'object') {
    return [];
  }

  const stepsLists = [];
  if (Array.isArray(template.tasks) && template.tasks.length > 0) {
    for (const task of template.tasks) {
      if (Array.isArray(task.steps)) {
        stepsLists.push(task.steps);
      }
    }
  } else if (Array.isArray(template.steps)) {
    stepsLists.push(template.steps);
  }

  const downloads = [];
  for (const steps of stepsLists) {
    for (const step of steps) {
      if (!step || step.type !== 'image' || step.operator !== 'capture' || !step.id) {
        continue;
      }
      const basename = typeof step.downloadBasename === 'string' ? step.downloadBasename.trim() : '';
      if (!basename) {
        continue;
      }
      const valueKey = typeof step.value === 'string' ? step.value.trim() : '';
      const dataUrl = valueKey ? results[valueKey] : undefined;
      if (typeof dataUrl !== 'string' || !dataUrl.startsWith('data:image/')) {
        continue;
      }

      downloads.push(await tbrgDownloadDataUrlAsPng(dataUrl, basename));
    }
  }

  return downloads;
}

function tbrgExecuteScript(tabId, files, allFrames = false) {
  return new Promise((resolve, reject) => {
    chrome.scripting.executeScript(
      {
        target: { tabId, allFrames },
        files
      },
      (injectionResults) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve(injectionResults);
      }
    );
  });
}

function tbrgExecuteScriptInFrame(tabId, files, frameId) {
  return new Promise((resolve, reject) => {
    chrome.scripting.executeScript(
      {
        target: { tabId, frameIds: [frameId] },
        files
      },
      (injectionResults) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve(injectionResults || []);
      }
    );
  });
}

function tbrgExecuteScriptFunction(tabId, func, allFrames = false, args = []) {
  return new Promise((resolve, reject) => {
    chrome.scripting.executeScript(
      {
        target: { tabId, allFrames },
        func,
        args
      },
      (injectionResults) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve(injectionResults || []);
      }
    );
  });
}

function tbrgSendTabMessage(tabId, message, frameId = undefined, timeoutMs = 90000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`Timed out after ${Math.round(timeoutMs / 1000)}s waiting for frame response.`));
    }, timeoutMs);

    const options = typeof frameId === 'number' ? { frameId } : undefined;
    chrome.tabs.sendMessage(tabId, message, options, (response) => {
      clearTimeout(timer);
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      resolve(response);
    });
  });
}

async function tbrgSendTabMessageWithFrameBootstrap(tabId, frameId, message, timeoutMs = 90000) {
  try {
    return await tbrgSendTabMessage(tabId, message, frameId, timeoutMs);
  } catch (error) {
    const msg = error?.message || String(error);
    if (!msg.includes('Receiving end does not exist')) {
      throw error;
    }

    await tbrgExecuteScriptInFrame(tabId, ['content_runner.js'], frameId);
    return tbrgSendTabMessage(tabId, message, frameId, timeoutMs);
  }
}

function tbrgSleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function tbrgIsFrameResolveRetryableError(error) {
  const msg = error?.message || String(error);
  return msg.includes('No frame matched') || msg.includes('No iframe matched');
}

async function tbrgResolveTargetFrameIdAttempt(tabId, template) {
  const frameSelector = (template.frameSelector || '').trim();
  const frameNeedle = (template.frameUrlIncludes || '').trim();

  if (frameSelector) {
    const framesByUrl = await tbrgExecuteScriptFunction(
      tabId,
      () => location.href,
      true
    );

    const selectorMatches = await tbrgExecuteScriptFunction(
      tabId,
      (selector) => {
        try {
          if (!window.frameElement) {
            return { isMatch: false, href: location.href };
          }
          return {
            isMatch: window.frameElement.matches(selector),
            href: location.href
          };
        } catch (_error) {
          return { isMatch: false, href: location.href };
        }
      },
      true,
      [frameSelector]
    );

    const selectorFrameMatch = selectorMatches.find((frame) => frame.result?.isMatch === true);
    if (selectorFrameMatch) {
      return selectorFrameMatch.frameId;
    }

    const selectorSrcMatches = await tbrgExecuteScriptFunction(
      tabId,
      (selector) => {
        const iframe = document.querySelector(selector);
        if (!iframe) {
          return null;
        }
        return iframe.getAttribute('src') || iframe.src || null;
      },
      false,
      [frameSelector]
    );

    const iframeSrc = selectorSrcMatches[0]?.result || '';
    if (iframeSrc) {
      const srcWithoutHash = iframeSrc.split('#')[0];
      const urlFrameMatch = framesByUrl.find((frame) =>
        typeof frame.result === 'string' &&
        srcWithoutHash &&
        (frame.result.includes(srcWithoutHash) || srcWithoutHash.includes(frame.result))
      );
      if (urlFrameMatch) {
        return urlFrameMatch.frameId;
      }
    }

    const available = framesByUrl
      .map((frame) => frame.result)
      .filter((url) => typeof url === 'string')
      .slice(0, 8)
      .join(', ');

    throw new Error(
      `No frame matched frameSelector="${frameSelector}". ` +
      `Iframe src="${iframeSrc || 'none'}". ` +
      `Available frames: ${available || 'none'}`
    );
  }

  if (!frameNeedle) {
    return 0;
  }

  const frames = await tbrgExecuteScriptFunction(
    tabId,
    () => location.href,
    true
  );

  const match = frames.find((frame) => typeof frame.result === 'string' && frame.result.includes(frameNeedle));
  if (!match) {
    const available = frames
      .map((frame) => frame.result)
      .filter((url) => typeof url === 'string')
      .slice(0, 5)
      .join(', ');
    throw new Error(`No iframe matched frameUrlIncludes="${frameNeedle}". Available frames: ${available || 'none'}`);
  }

  return match.frameId;
}

async function tbrgResolveTargetFrameId(tabId, template) {
  const timeoutMs = Number(template.frameResolveTimeoutMs) > 0 ? Number(template.frameResolveTimeoutMs) : 30000;
  const pollMs = 250;
  const deadline = Date.now() + timeoutMs;
  let lastError;

  while (Date.now() < deadline) {
    try {
      return await tbrgResolveTargetFrameIdAttempt(tabId, template);
    } catch (error) {
      lastError = error;
      if (!tbrgIsFrameResolveRetryableError(error)) {
        throw error;
      }
      const remaining = deadline - Date.now();
      if (remaining <= 0) {
        break;
      }
      await tbrgSleep(Math.min(pollMs, remaining));
    }
  }

  const waitedNote = ` (waited ${timeoutMs}ms for iframe / embedded app to load)`;
  if (lastError?.message) {
    throw new Error(`${lastError.message}${waitedNote}`);
  }
  throw new Error(`Timed out resolving target iframe${waitedNote}`);
}

function tbrgMimeTypeForAssetFilename(filename) {
  const lower = String(filename || '').toLowerCase();
  if (lower.endsWith('.png')) return 'image/png';
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg';
  if (lower.endsWith('.gif')) return 'image/gif';
  if (lower.endsWith('.webp')) return 'image/webp';
  if (lower.endsWith('.svg')) return 'image/svg+xml';
  return 'application/octet-stream';
}

function tbrgUint8ArrayToBase64(bytes) {
  let binary = '';
  const len = bytes.length;
  const chunk = 0x8000;
  for (let i = 0; i < len; i += chunk) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, Math.min(i + chunk, len)));
  }
  return btoa(binary);
}

async function tbrgFetchExtensionAssetBytes(relativePath) {
  const response = await fetch(chrome.runtime.getURL(relativePath), { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Failed to load extension asset: ${relativePath}`);
  }
  return new Uint8Array(await response.arrayBuffer());
}

/**
 * Replace src="assets/..." (and href=) with data URLs so downloaded single-file HTML decks still show images.
 */
async function tbrgInlineDeckRelativeAssets(html, templateId) {
  const id = typeof templateId === 'string' && templateId.trim() ? templateId.trim() : '';
  if (!id || typeof html !== 'string') {
    return html;
  }
  const re = /(?:src|href)=(["'])assets\/([^"'#?]+)\1/gi;
  const files = new Set();
  let match;
  while ((match = re.exec(html)) !== null) {
    files.add(match[2]);
  }
  let out = html;
  for (const file of files) {
    const extPath = `templates/${id}/assets/${file}`;
    try {
      const bytes = await tbrgFetchExtensionAssetBytes(extPath);
      const mime = tbrgMimeTypeForAssetFilename(file);
      const dataUrl = `data:${mime};base64,${tbrgUint8ArrayToBase64(bytes)}`;
      out = out.split(`src="assets/${file}"`).join(`src="${dataUrl}"`);
      out = out.split(`src='assets/${file}'`).join(`src='${dataUrl}'`);
      out = out.split(`href="assets/${file}"`).join(`href="${dataUrl}"`);
      out = out.split(`href='assets/${file}'`).join(`href='${dataUrl}'`);
    } catch (_error) {
      // Missing asset: keep original relative path.
    }
  }
  return out;
}

function tbrgDownloadDeck(html, templateId) {
  return new Promise((resolve, reject) => {
    const filename = `monthly-report-${templateId}-${Date.now()}.html`;
    const base64Html = btoa(unescape(encodeURIComponent(html)));
    const dataUrl = `data:text/html;charset=utf-8;base64,${base64Html}`;

    chrome.downloads.download(
      {
        url: dataUrl,
        filename,
        saveAs: false,
        conflictAction: 'uniquify'
      },
      (downloadId) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }

        resolve({ downloadId, filename });
      }
    );
  });
}

async function tbrgSetActionBadge(text, color) {
  try {
    if (color) {
      await chrome.action.setBadgeBackgroundColor({ color });
    }
    await chrome.action.setBadgeText({ text: text || '' });
  } catch (_error) {
    // Ignore badge errors (not critical to job).
  }
}

async function tbrgSetRunningBadge() {
  await tbrgSetActionBadge('RUN', '#2563eb');
}

async function tbrgSetErrorBadge() {
  await tbrgSetActionBadge('ERR', '#dc2626');
  setTimeout(() => {
    tbrgSetActionBadge('', null);
  }, 5000);
}

async function tbrgClearBadge() {
  await tbrgSetActionBadge('', null);
}

function tbrgCountTaskAndStepTotals(template) {
  const tasks = Array.isArray(template.tasks) ? template.tasks : [];
  if (tasks.length > 0) {
    return {
      totalTasks: tasks.length,
      totalSteps: tasks.reduce((acc, task) => acc + (Array.isArray(task.steps) ? task.steps.length : 0), 0)
    };
  }
  const steps = Array.isArray(template.steps) ? template.steps : [];
  return {
    totalTasks: steps.length > 0 ? 1 : 0,
    totalSteps: steps.length
  };
}

async function tbrgNotifyJobProgress(payload) {
  try {
    await chrome.runtime.sendMessage({
      type: 'TBRG_JOB_PROGRESS',
      ...payload
    });
  } catch (_error) {
    // Popup may be closed while a job is running.
  }
}

function tbrgComputeStepPercent(payload) {
  const done = Number(payload?.completedSteps) || 0;
  const total = Number(payload?.totalSteps) || 0;
  if (total <= 0) {
    return 0;
  }
  return Math.max(0, Math.min(100, Math.round((done / total) * 100)));
}

async function tbrgRenderPageProgress(tabId, payload) {
  if (!Number.isInteger(tabId)) {
    return;
  }
  const percent = tbrgComputeStepPercent(payload);
  const stage = String(payload?.stage || 'running');
  const done = Number(payload?.completedSteps) || 0;
  const total = Number(payload?.totalSteps) || 0;
  const title =
    stage === 'finished'
      ? 'Automation finished'
      : stage === 'error'
        ? 'Automation failed'
        : stage === 'cancelled'
          ? 'Automation stopped'
        : 'Automation running';
  const detail =
    stage === 'error'
      ? String(payload?.error || 'Unknown error')
      : stage === 'cancelled'
        ? String(payload?.error || 'Stopped by user.')
      : `Steps ${done}/${total} (${percent}%)`;

  await tbrgExecuteScriptFunction(
    tabId,
    (progress) => {
      const OVERLAY_ID = '__tbrg_progress_overlay__';
      const STYLE_ID = '__tbrg_progress_overlay_style__';
      if (progress.stage === 'cancelled') {
        const existing = document.getElementById(OVERLAY_ID);
        if (existing) {
          existing.remove();
        }
        const styleEl = document.getElementById(STYLE_ID);
        if (styleEl) {
          styleEl.remove();
        }
        return;
      }

      if (!document.getElementById(STYLE_ID)) {
        const style = document.createElement('style');
        style.id = STYLE_ID;
        style.textContent = `
          #${OVERLAY_ID} {
            position: fixed;
            top: 12px;
            right: 12px;
            z-index: 2147483647;
            width: 280px;
            border-radius: 12px;
            border: 1px solid rgba(37,99,235,0.24);
            background: rgba(15,23,42,0.94);
            color: #e2e8f0;
            box-shadow: 0 12px 30px rgba(2,6,23,0.35);
            font: 12px/1.35 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
            padding: 10px;
          }
          #${OVERLAY_ID} .title { font-weight: 700; margin-bottom: 6px; }
          #${OVERLAY_ID} .detail { opacity: 0.92; margin-bottom: 8px; word-break: break-word; }
          #${OVERLAY_ID} .track {
            width: 100%;
            height: 8px;
            border-radius: 999px;
            background: rgba(148,163,184,0.3);
            overflow: hidden;
          }
          #${OVERLAY_ID} .fill {
            height: 100%;
            width: 0%;
            background: #2563eb;
            transition: width 0.2s ease;
          }
          #${OVERLAY_ID}.done .fill { background: #16a34a; }
          #${OVERLAY_ID}.error .fill { background: #dc2626; }
        `;
        document.documentElement.appendChild(style);
      }

      let el = document.getElementById(OVERLAY_ID);
      if (!el) {
        el = document.createElement('div');
        el.id = OVERLAY_ID;
        el.innerHTML = `
          <div class="title"></div>
          <div class="detail"></div>
          <div class="track"><div class="fill"></div></div>
        `;
        document.documentElement.appendChild(el);
      }

      el.classList.toggle('done', progress.stage === 'finished');
      el.classList.toggle('error', progress.stage === 'error');
      el.querySelector('.title').textContent = progress.title;
      el.querySelector('.detail').textContent = progress.detail;
      el.querySelector('.fill').style.width = `${progress.percent}%`;

      if (progress.stage === 'finished' || progress.stage === 'error') {
        const hideDelayMs = progress.stage === 'finished' ? 7000 : 10000;
        setTimeout(() => {
          const target = document.getElementById(OVERLAY_ID);
          if (target) {
            target.remove();
          }
        }, hideDelayMs);
      }
    },
    false,
    [{ stage, title, detail, percent }]
  );
}

const tbrgPendingPickerByTabId = new Map();

let tbrgDebugTabListenerAttached = false;

function tbrgDebugOnTabUpdated(tabId, changeInfo, tab) {
  if (typeof changeInfo.url !== 'string' || !/^https?:\/\//i.test(changeInfo.url)) {
    return;
  }
  tbrgDebugAppendEvent({
    type: 'navigation',
    tabId,
    url: changeInfo.url,
    title: typeof tab?.title === 'string' ? tab.title : ''
  }).catch(() => null);
}

function tbrgDebugAttachTabListener() {
  if (tbrgDebugTabListenerAttached) {
    return;
  }
  chrome.tabs.onUpdated.addListener(tbrgDebugOnTabUpdated);
  tbrgDebugTabListenerAttached = true;
}

function tbrgDebugDetachTabListener() {
  if (!tbrgDebugTabListenerAttached) {
    return;
  }
  chrome.tabs.onUpdated.removeListener(tbrgDebugOnTabUpdated);
  tbrgDebugTabListenerAttached = false;
}

async function tbrgDebugRegisterRecorder() {
  const existing = await new Promise((resolve) => {
    chrome.scripting.getRegisteredContentScripts({ ids: [TBRG_DEBUG_RECORDER_SCRIPT_ID] }, (scripts) => {
      if (chrome.runtime.lastError) {
        resolve([]);
        return;
      }
      resolve(scripts || []);
    });
  });
  if (Array.isArray(existing) && existing.length > 0) {
    return;
  }
  await new Promise((resolve, reject) => {
    chrome.scripting.registerContentScripts(
      [
        {
          id: TBRG_DEBUG_RECORDER_SCRIPT_ID,
          js: ['debug_dom_recorder.js'],
          matches: ['<all_urls>'],
          runAt: 'document_idle',
          allFrames: true
        }
      ],
      () => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve();
      }
    );
  });
}

async function tbrgDebugUnregisterRecorder() {
  await new Promise((resolve) => {
    chrome.scripting.unregisterContentScripts({ ids: [TBRG_DEBUG_RECORDER_SCRIPT_ID] }, () => resolve());
  });
}

async function tbrgDebugInjectRecorderIntoOpenTabs() {
  let tabs = [];
  try {
    tabs = await chrome.tabs.query({ url: ['https://*/*', 'http://*/*'] });
  } catch (_error) {
    return;
  }
  for (const tab of tabs) {
    if (!Number.isInteger(tab.id)) {
      continue;
    }
    try {
      await tbrgExecuteScript(tab.id, ['debug_dom_recorder.js'], true);
    } catch (_error) {
      // Restricted tab, no host access, etc.
    }
  }
}

async function tbrgDebugEnsureRecorderActive() {
  await tbrgDebugRegisterRecorder();
  await tbrgDebugInjectRecorderIntoOpenTabs();
  tbrgDebugAttachTabListener();
}

async function tbrgDebugOnUserEnabledDebug() {
  const fresh = {
    schemaVersion: TBRG_DEBUG_SCHEMA_VERSION,
    sessionStartedAt: new Date().toISOString(),
    events: []
  };
  await new Promise((resolve) => {
    chrome.storage.local.set({ [TBRG_DEBUG_DOM_LOG_KEY]: fresh }, resolve);
  });
  await tbrgDebugEnsureRecorderActive();
}

async function tbrgDebugOnUserDisabledDebug() {
  tbrgDebugDetachTabListener();
  try {
    await tbrgDebugUnregisterRecorder();
  } catch (_error) {
    // Best effort.
  }
  const stored = await new Promise((resolve) => {
    chrome.storage.local.get([TBRG_DEBUG_DOM_LOG_KEY], resolve);
  });
  const doc = stored[TBRG_DEBUG_DOM_LOG_KEY];
  if (doc?.events?.length) {
    try {
      await tbrgExportDebugActivityLog(doc);
    } catch (_error) {
      // Best effort export on disable.
    }
  }
}

async function tbrgDebugAppendEvent(event) {
  const stored = await new Promise((resolve) => {
    chrome.storage.local.get([TBRG_DEBUG_MODE_STORAGE_KEY, TBRG_DEBUG_DOM_LOG_KEY], resolve);
  });
  if (!stored[TBRG_DEBUG_MODE_STORAGE_KEY]) {
    return;
  }
  if (!tbrgDebugTabListenerAttached) {
    tbrgDebugAttachTabListener();
  }
  let doc = stored[TBRG_DEBUG_DOM_LOG_KEY];
  if (!doc || !Array.isArray(doc.events) || typeof doc.sessionStartedAt !== 'string') {
    doc = {
      schemaVersion: TBRG_DEBUG_SCHEMA_VERSION,
      sessionStartedAt: new Date().toISOString(),
      events: []
    };
  }
  const ts = typeof event.ts === 'string' ? event.ts : new Date().toISOString();
  const nextEvent = { ...event, ts };
  const next = {
    ...doc,
    events: [...doc.events, nextEvent]
  };
  while (next.events.length > TBRG_DEBUG_MAX_EVENTS) {
    next.events.shift();
  }
  await new Promise((resolve) => {
    chrome.storage.local.set({ [TBRG_DEBUG_DOM_LOG_KEY]: next }, resolve);
  });
}

function tbrgExportDebugActivityLog(doc) {
  return new Promise((resolve, reject) => {
    if (!doc || !Array.isArray(doc.events)) {
      reject(new Error('No debug log to export.'));
      return;
    }
    const exportDoc = {
      ...doc,
      exportedAt: new Date().toISOString()
    };
    const jsonText = JSON.stringify(exportDoc, null, 2);
    const dataUrl = `data:application/json;charset=utf-8,${encodeURIComponent(jsonText)}`;

    chrome.downloads.download(
      {
        url: dataUrl,
        filename: TBRG_DEBUG_EXPORT_RELATIVE_PATH,
        saveAs: false,
        conflictAction: 'overwrite'
      },
      (downloadId) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve({ downloadId, filename: TBRG_DEBUG_EXPORT_RELATIVE_PATH });
      }
    );
  });
}

async function tbrgDebugClearLogKeepSession() {
  const fresh = {
    schemaVersion: TBRG_DEBUG_SCHEMA_VERSION,
    sessionStartedAt: new Date().toISOString(),
    events: []
  };
  await new Promise((resolve) => {
    chrome.storage.local.set({ [TBRG_DEBUG_DOM_LOG_KEY]: fresh }, resolve);
  });
}

chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName !== 'local' || !changes[TBRG_DEBUG_MODE_STORAGE_KEY]) {
    return;
  }
  const was = Boolean(changes[TBRG_DEBUG_MODE_STORAGE_KEY].oldValue);
  const now = Boolean(changes[TBRG_DEBUG_MODE_STORAGE_KEY].newValue);
  if (now && !was) {
    tbrgDebugOnUserEnabledDebug().catch(() => null);
    return;
  }
  if (!now && was) {
    tbrgDebugOnUserDisabledDebug().catch(() => null);
    return;
  }
  if (now) {
    tbrgDebugEnsureRecorderActive().catch(() => null);
  }
});

chrome.runtime.onInstalled.addListener(() => {
  chrome.storage.local.get([TBRG_DEBUG_MODE_STORAGE_KEY], (data) => {
    if (data[TBRG_DEBUG_MODE_STORAGE_KEY]) {
      tbrgDebugEnsureRecorderActive().catch(() => null);
    }
  });
});

chrome.runtime.onStartup.addListener(() => {
  chrome.storage.local.get([TBRG_DEBUG_MODE_STORAGE_KEY], (data) => {
    if (data[TBRG_DEBUG_MODE_STORAGE_KEY]) {
      tbrgDebugEnsureRecorderActive().catch(() => null);
    }
  });
});

async function tbrgStopDomPicker(tabId) {
  try {
    await tbrgExecuteScriptFunction(
      tabId,
      () => {
        if (typeof self.__TBRG_DOM_PICKER_STOP__ === 'function') {
          self.__TBRG_DOM_PICKER_STOP__();
        }
        return true;
      },
      true
    );
  } catch (_error) {
    // Best effort cleanup.
  }
}

async function tbrgPickDomElementFromActiveTab() {
  const tab = await tbrgGetActiveTab();
  const tabId = tab.id;
  const pageUrl = tab.url || '';

  if (tbrgPendingPickerByTabId.has(tabId)) {
    throw new Error('DOM picker is already active for this tab.');
  }

  await tbrgExecuteScript(tabId, ['dom_picker.js'], true);
  await tbrgExecuteScriptFunction(
    tabId,
    () => {
      if (typeof self.__TBRG_DOM_PICKER_START__ === 'function') {
        return self.__TBRG_DOM_PICKER_START__();
      }
      throw new Error('DOM picker script did not initialize.');
    },
    true
  );

  return await new Promise((resolve, reject) => {
    const timeoutId = setTimeout(async () => {
      tbrgPendingPickerByTabId.delete(tabId);
      await tbrgStopDomPicker(tabId);
      reject(new Error('Timed out waiting for DOM pick (120s).'));
    }, 120000);

    tbrgPendingPickerByTabId.set(tabId, {
      resolve: async (pickPayload) => {
        clearTimeout(timeoutId);
        tbrgPendingPickerByTabId.delete(tabId);
        await tbrgStopDomPicker(tabId);
        resolve({ pick: pickPayload, pageUrl });
      },
      reject: async (error) => {
        clearTimeout(timeoutId);
        tbrgPendingPickerByTabId.delete(tabId);
        await tbrgStopDomPicker(tabId);
        reject(error);
      }
    });
  });
}

async function tbrgShowEmbedOnTab(tabId, template) {
  if (!Number.isInteger(tabId)) {
    throw new Error('Invalid tab for embed overlay.');
  }
  await tbrgExecuteScript(tabId, ['embed_overlay.js'], false);
  await tbrgExecuteScriptFunction(
    tabId,
    (payload) => {
      if (typeof self.__TBRG_EMBED_SHOW__ === 'function') {
        self.__TBRG_EMBED_SHOW__(payload);
      }
    },
    false,
    [{ url: template.embedUrl, title: template.name || template.id }]
  );
}

async function tbrgRunEmbedJob(template, runGeneration) {
  tbrgAssertRunActive(runGeneration);
  const activeTab = await tbrgGetActiveTab();
  if (!activeTab?.id) {
    throw new Error('No active tab found.');
  }

  const totalTasks = 1;
  const totalSteps = 1;

  const startedPayload = {
    stage: 'started',
    templateId: template.id,
    completedTasks: 0,
    totalTasks,
    completedSteps: 0,
    totalSteps
  };
  await tbrgNotifyJobProgress(startedPayload);

  tbrgAssertRunActive(runGeneration);
  await tbrgShowEmbedOnTab(activeTab.id, template);
  tbrgAssertRunActive(runGeneration);

  const finishedPayload = {
    stage: 'finished',
    templateId: template.id,
    completedTasks: totalTasks,
    totalTasks,
    completedSteps: totalSteps,
    totalSteps,
    downloadFilename: ''
  };
  await tbrgNotifyJobProgress(finishedPayload);

  const executionResult = {
    ok: true,
    results: {},
    stepResults: [],
    url: activeTab.url || '',
    title: ''
  };

  return {
    templateId: template.id,
    executionResult,
    download: null,
    screenshotDownloads: [],
    embedOpened: true
  };
}

async function tbrgRunJob(templateId, runGeneration) {
  tbrgAutomationJobInProgress = true;
  try {
    tbrgAssertRunActive(runGeneration);
    const template = await tbrgResolveTemplateById(templateId);
    tbrgAssertRunActive(runGeneration);
    await tbrgSetSelectedTemplateId(template.id);
    tbrgAssertRunActive(runGeneration);

    if (template.runMode === 'embed') {
      return await tbrgRunEmbedJob(template, runGeneration);
    }

    const { totalTasks, totalSteps } = tbrgCountTaskAndStepTotals(template);
    const activeTab = await tbrgGetActiveTab();
    const progressTabId = activeTab?.id;

    const startedPayload = {
      stage: 'started',
      templateId: template.id,
      completedTasks: 0,
      totalTasks,
      completedSteps: 0,
      totalSteps
    };
    await tbrgNotifyJobProgress(startedPayload);
    await tbrgRenderPageProgress(progressTabId, startedPayload);
    tbrgAssertRunActive(runGeneration);

    let executionResult;

    if (Array.isArray(template.tasks) && template.tasks.length > 0) {
      executionResult = await tbrgExecuteTasksSequentially(template, async (progress) => {
        const payload = {
          templateId: template.id,
          totalTasks,
          totalSteps,
          ...progress
        };
        await tbrgNotifyJobProgress(payload);
        await tbrgRenderPageProgress(progressTabId, payload);
      }, runGeneration, progressTabId);
    } else {
      const stepsOnly = tbrgPickStepsArray(template);
      if (!stepsOnly || stepsOnly.length === 0) {
        throw new Error(`Template "${template.id}" has no executable steps.`);
      }

      const targetTab = await tbrgEnsureTargetPage(template);
      tbrgAssertRunActive(runGeneration);
      await tbrgExecuteScript(targetTab.id, ['content_runner.js'], true);
      const targetFrameId = await tbrgResolveTargetFrameId(targetTab.id, template);
      tbrgAssertRunActive(runGeneration);

      await tbrgExecuteScriptInFrame(targetTab.id, ['content_runner.js'], targetFrameId);
      executionResult = await tbrgSendTabMessageWithFrameBootstrap(targetTab.id, targetFrameId, {
        type: 'TBRG_EXECUTE_TEMPLATE',
        template,
        progressReporting: {
          templateId: template.id,
          progressTabId,
          completedStepsOffset: 0,
          totalStepsOverall: totalSteps,
          completedTasks: 0,
          totalTasks
        }
      }, TBRG_USER_INPUT_TIMEOUT_MS);
      tbrgAssertRunActive(runGeneration);

      if (!executionResult?.ok) {
        throw new Error(executionResult?.error || 'Content execution failed.');
      }

      const failedSteps = (executionResult.stepResults || []).filter((step) => !step.ok);
      if (failedSteps.length > 0) {
        const failedSummary = failedSteps
          .slice(0, 3)
          .map((step) => `${step.id || 'unknown'}: ${step.error || 'step failed'}`)
          .join('; ');
        throw new Error(
          `Template execution failed (${failedSteps.length} step failure${failedSteps.length === 1 ? '' : 's'}): ${failedSummary}`
        );
      }

      const runningPayload = {
        stage: 'running',
        templateId: template.id,
        completedTasks: totalTasks,
        totalTasks,
        completedSteps: (executionResult.stepResults || []).filter((step) => step.ok).length,
        totalSteps,
        lastCompletedStepId: (executionResult.stepResults || []).filter((step) => step.ok).slice(-1)[0]?.id || null
      };
      await tbrgNotifyJobProgress(runningPayload);
      await tbrgRenderPageProgress(progressTabId, runningPayload);
    }

    tbrgAssertRunActive(runGeneration);
    const screenshotDownloads = await tbrgDownloadScreenshotExports(template, executionResult.results);

    if (template.runMode === 'embedAfter') {
      tbrgAssertRunActive(runGeneration);
      if (!Number.isInteger(progressTabId)) {
        throw new Error('No active tab for embed overlay.');
      }
      await tbrgShowEmbedOnTab(progressTabId, template);
      tbrgAssertRunActive(runGeneration);

      const finishedEmbedPayload = {
        stage: 'finished',
        templateId: template.id,
        completedTasks: totalTasks,
        totalTasks,
        completedSteps: (executionResult.stepResults || []).filter((step) => step.ok).length,
        totalSteps,
        downloadFilename: ''
      };
      await tbrgNotifyJobProgress(finishedEmbedPayload);
      await tbrgRenderPageProgress(progressTabId, finishedEmbedPayload);

      return {
        templateId: template.id,
        executionResult,
        download: null,
        screenshotDownloads,
        embedOpened: true
      };
    }

    let externalSlidesHtml = '';
    const slidesCandidatePaths = [];
    if (typeof template.slidesHtmlFileResolved === 'string' && template.slidesHtmlFileResolved.trim()) {
      slidesCandidatePaths.push(template.slidesHtmlFileResolved.trim());
    }
    if (typeof template.id === 'string' && template.id.trim()) {
      slidesCandidatePaths.push(`templates/${template.id.trim()}/template.html`);
    }
    for (const candidate of slidesCandidatePaths) {
      tbrgAssertRunActive(runGeneration);
      try {
        externalSlidesHtml = await tbrgFetchText(candidate);
        if (externalSlidesHtml) {
          break;
        }
      } catch (_error) {
        // Try next candidate.
      }
    }

    tbrgAssertRunActive(runGeneration);
    const htmlRaw = tbrgBuildRevealDeckHtml(template, executionResult.results, externalSlidesHtml);
    const html = await tbrgInlineDeckRelativeAssets(htmlRaw, template.id);
    const download = await tbrgDownloadDeck(html, template.id);
    tbrgAssertRunActive(runGeneration);

    const finishedPayload = {
      stage: 'finished',
      templateId: template.id,
      completedTasks: totalTasks,
      totalTasks,
      completedSteps: (executionResult.stepResults || []).filter((step) => step.ok).length,
      totalSteps,
      downloadFilename: download.filename
    };
    await tbrgNotifyJobProgress(finishedPayload);
    await tbrgRenderPageProgress(progressTabId, finishedPayload);

    return {
      templateId: template.id,
      executionResult,
      download,
      screenshotDownloads,
      embedOpened: false
    };
  } finally {
    tbrgAutomationJobInProgress = false;
  }
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message?.type === 'TBRG_JOB_STEP_PROGRESS') {
    const tabId = Number(message.progressTabId);
    const payload = {
      stage: 'running',
      templateId: message.templateId,
      completedTasks: Number(message.completedTasks) || 0,
      totalTasks: Number(message.totalTasks) || 0,
      completedSteps: Number(message.completedSteps) || 0,
      totalSteps: Number(message.totalSteps) || 0,
      lastCompletedStepId: message.lastCompletedStepId || null,
      currentTaskId: message.currentTaskId || null,
      currentTaskName: message.currentTaskName || null
    };
    tbrgNotifyJobProgress(payload).catch(() => null);
    if (Number.isInteger(tabId)) {
      tbrgRenderPageProgress(tabId, payload).catch(() => null);
    }
    sendResponse({ ok: true });
    return true;
  }

  if (message?.type === 'TBRG_LIST_TEMPLATES') {
    tbrgListTemplates()
      .then((result) => sendResponse({ ok: true, ...result }))
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_SAVE_CUSTOM_TEMPLATE') {
    tbrgSaveCustomTemplateText(message.templateText || '')
      .then(() => sendResponse({ ok: true }))
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_DOWNLOAD_URL') {
    const url = typeof message.url === 'string' ? message.url.trim() : '';
    const downloadBasename = typeof message.downloadBasename === 'string' ? message.downloadBasename.trim() : '';
    const extRaw = typeof message.downloadFileExtension === 'string' ? message.downloadFileExtension.trim() : '';
    const ext = extRaw.replace(/^\.+/, '');
    if (!url.startsWith('https://')) {
      sendResponse({ ok: false, error: 'URL must use https.' });
      return false;
    }
    try {
      // Validate URL shape.
      new URL(url);
    } catch (_error) {
      sendResponse({ ok: false, error: 'Invalid URL.' });
      return false;
    }
    if (ext && !/^[a-zA-Z0-9]{1,15}$/.test(ext)) {
      sendResponse({ ok: false, error: 'Invalid downloadFileExtension.' });
      return false;
    }
    const safeBase = (downloadBasename || 'download').replace(/[^\w\-]+/g, '_');
    const filename = ext ? `${safeBase}-${Date.now()}.${ext}` : `${safeBase}-${Date.now()}`;
    chrome.downloads.download(
      {
        url,
        filename,
        saveAs: false,
        conflictAction: 'uniquify'
      },
      () => {
        if (chrome.runtime.lastError) {
          sendResponse({ ok: false, error: chrome.runtime.lastError.message });
          return;
        }
        sendResponse({ ok: true, filename });
      }
    );
    return true;
  }

  if (message?.type === 'TBRG_START_JOB') {
    const runGeneration = ++tbrgRunGeneration;
    tbrgSetRunningBadge()
      .then(() => tbrgRunJob(message.templateId, runGeneration))
      .then((result) => {
        if (runGeneration !== tbrgRunGeneration) {
          sendResponse({ ok: false, stopped: true, error: 'Automation stopped by user.' });
          return;
        }
        tbrgClearBadge();
        if (result.embedOpened) {
          sendResponse({
            ok: true,
            templateId: result.templateId,
            embedOpened: true,
            resultKeys: [],
            downloadFilename: '',
            screenshotFilenames: []
          });
          return;
        }
        sendResponse({
          ok: true,
          templateId: result.templateId,
          resultKeys: Object.keys(result.executionResult.results),
          downloadFilename: result.download.filename,
          screenshotFilenames: (result.screenshotDownloads || []).map((item) => item.filename)
        });
      })
      .catch((error) => {
        if (error?.code === 'TBRG_STOPPED') {
          tbrgClearBadge();
          sendResponse({ ok: false, stopped: true, error: error.message || String(error) });
          return;
        }
        tbrgSetErrorBadge();
        const errorPayload = {
          stage: 'error',
          error: error.message || String(error)
        };
        tbrgNotifyJobProgress(errorPayload);
        tbrgGetActiveTab()
          .then((tab) => tbrgRenderPageProgress(tab?.id, errorPayload))
          .catch(() => null);
        sendResponse({ ok: false, error: error.message || String(error) });
      });
    return true;
  }

  if (message?.type === 'TBRG_STOP_JOB') {
    tbrgRunGeneration += 1;
    const cancelledPayload = {
      stage: 'cancelled',
      error: 'Automation stopped by user.',
      completedTasks: 0,
      totalTasks: 0,
      completedSteps: 0,
      totalSteps: 0
    };
    tbrgNotifyJobProgress(cancelledPayload);
    tbrgGetActiveTab()
      .then(async (tab) => {
        const tabId = tab?.id;
        await tbrgRenderPageProgress(tabId, cancelledPayload);
        if (!Number.isInteger(tabId)) {
          return;
        }
        try {
          await tbrgExecuteScript(tabId, ['embed_overlay.js'], false);
          await tbrgExecuteScriptFunction(
            tabId,
            () => {
              if (typeof self.__TBRG_EMBED_HIDE__ === 'function') {
                self.__TBRG_EMBED_HIDE__();
              }
            },
            false,
            []
          );
        } catch (_error) {
          // Best effort embed cleanup.
        }
      })
      .catch(() => null);
    tbrgClearBadge()
      .then(() => sendResponse({ ok: true, stopped: true }))
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_DEBUG_ACTIVITY') {
    const ev = message.event;
    if (!ev || typeof ev !== 'object' || typeof ev.type !== 'string') {
      sendResponse({ ok: false, error: 'Invalid debug event.' });
      return false;
    }
    if (tbrgAutomationJobInProgress) {
      sendResponse({ ok: true, skippedDuringJob: true });
      return true;
    }
    tbrgDebugAppendEvent(ev)
      .then(() => sendResponse({ ok: true }))
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_DEBUG_EXPORT_LOG') {
    chrome.storage.local.get([TBRG_DEBUG_DOM_LOG_KEY], (stored) => {
      let doc = stored[TBRG_DEBUG_DOM_LOG_KEY];
      if (!doc || !Array.isArray(doc.events)) {
        doc = {
          schemaVersion: TBRG_DEBUG_SCHEMA_VERSION,
          sessionStartedAt: new Date().toISOString(),
          events: []
        };
      }
      tbrgExportDebugActivityLog(doc)
        .then((out) => sendResponse({ ok: true, filename: out.filename }))
        .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    });
    return true;
  }

  if (message?.type === 'TBRG_DEBUG_CLEAR_LOG') {
    tbrgDebugClearLogKeepSession()
      .then(() => sendResponse({ ok: true }))
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_PICK_ELEMENT') {
    tbrgPickDomElementFromActiveTab()
      .then(async ({ pick, pageUrl }) => {
        await tbrgDebugAppendEvent({
          type: 'domPick',
          pageUrl,
          pick
        });
        const flag = await new Promise((resolve) => {
          chrome.storage.local.get([TBRG_DEBUG_MODE_STORAGE_KEY], (data) => {
            resolve(Boolean(data[TBRG_DEBUG_MODE_STORAGE_KEY]));
          });
        });
        sendResponse({
          ok: true,
          pick,
          ...(flag ? { debugExportPathHint: TBRG_DEBUG_EXPORT_RELATIVE_PATH } : {})
        });
      })
      .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message?.type === 'TBRG_DOM_PICKER_RESULT' || message?.type === 'TBRG_DOM_PICKER_CANCELLED') {
    const tabId = sender.tab?.id;
    const pending = typeof tabId === 'number' ? tbrgPendingPickerByTabId.get(tabId) : null;
    if (pending) {
      if (message.type === 'TBRG_DOM_PICKER_RESULT' && message.pick?.selector) {
        pending.resolve(message.pick);
      } else {
        pending.reject(new Error(message.reason || 'DOM picker cancelled.'));
      }
    }
    sendResponse({ ok: true });
    return true;
  }

  if (message?.type === 'TBRG_CAPTURE_VISIBLE_TAB') {
    const tab = sender.tab;
    chrome.tabs.captureVisibleTab(tab?.windowId, { format: 'png' }, (dataUrl) => {
      if (chrome.runtime.lastError) {
        sendResponse({ ok: false, error: chrome.runtime.lastError.message });
        return;
      }

      sendResponse({ ok: true, dataUrl });
    });
    return true;
  }

  return false;
});
