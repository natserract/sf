importScripts('vendor/js-yaml.min.js', 'template_loader.js', 'reveal_renderer.js');

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

async function tbrgExecuteTasksSequentially(template) {
  const merged = {
    ok: true,
    results: {},
    stepResults: [],
    taskResults: [],
    url: '',
    title: ''
  };

  for (const task of template.tasks) {
    const pageForNav = tbrgEffectivePageForTask(template, task);
    const targetTab = await tbrgEnsureTargetPage({ ...template, page: pageForNav });
    await tbrgExecuteScript(targetTab.id, ['content_runner.js'], true);

    const frameCtx = tbrgFrameContextForTask(template, task);
    const frameTemplate = { ...template, ...frameCtx };

    const targetFrameId = await tbrgResolveTargetFrameId(targetTab.id, frameTemplate);

    await tbrgExecuteScriptInFrame(targetTab.id, ['content_runner.js'], targetFrameId);
    const partial = await tbrgSendTabMessageWithFrameBootstrap(targetTab.id, targetFrameId, {
      type: 'TBRG_EXECUTE_TEMPLATE',
      template: { tasks: [task], slides: [] }
    }, 60000);

    if (!partial?.ok) {
      throw new Error(partial?.error || 'Content execution failed.');
    }

    Object.assign(merged.results, partial.results || {});
    merged.stepResults.push(...(partial.stepResults || []));
    merged.taskResults.push(...(partial.taskResults || []));
    merged.url = partial.url || merged.url;
    merged.title = partial.title || merged.title;

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
      if (!step || step.type !== 'screenshot' || !step.id) {
        continue;
      }
      const basename = typeof step.downloadBasename === 'string' ? step.downloadBasename.trim() : '';
      if (!basename) {
        continue;
      }
      const dataUrl = results[step.id];
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

const tbrgPendingPickerByTabId = new Map();

function tbrgDownloadJsonDebugFile(payload, basename = 'picked-element-debug') {
  return new Promise((resolve, reject) => {
    const safeBase = (basename || 'picked-element-debug').replace(/[^\w\-]+/g, '_');
    const filename = `${safeBase}-${Date.now()}.json`;
    const jsonText = JSON.stringify(payload, null, 2);
    const dataUrl = `data:application/json;charset=utf-8,${encodeURIComponent(jsonText)}`;

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

async function tbrgRunJob(templateId) {
  const template = await tbrgResolveTemplateById(templateId);
  await tbrgSetSelectedTemplateId(template.id);

  let executionResult;

  if (Array.isArray(template.tasks) && template.tasks.length > 0) {
    executionResult = await tbrgExecuteTasksSequentially(template);
  } else {
    const stepsOnly = tbrgPickStepsArray(template);
    if (!stepsOnly || stepsOnly.length === 0) {
      throw new Error(`Template "${template.id}" has no executable steps.`);
    }

    const targetTab = await tbrgEnsureTargetPage(template);
    await tbrgExecuteScript(targetTab.id, ['content_runner.js'], true);
    const targetFrameId = await tbrgResolveTargetFrameId(targetTab.id, template);

    await tbrgExecuteScriptInFrame(targetTab.id, ['content_runner.js'], targetFrameId);
    executionResult = await tbrgSendTabMessageWithFrameBootstrap(targetTab.id, targetFrameId, {
      type: 'TBRG_EXECUTE_TEMPLATE',
      template
    }, 60000);

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
  }

  const screenshotDownloads = await tbrgDownloadScreenshotExports(template, executionResult.results);

  const html = tbrgBuildRevealDeckHtml(template, executionResult.results);
  const download = await tbrgDownloadDeck(html, template.id);

  return {
    templateId: template.id,
    executionResult,
    download,
    screenshotDownloads
  };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
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

  if (message?.type === 'TBRG_START_JOB') {
    tbrgSetRunningBadge()
      .then(() => tbrgRunJob(message.templateId))
      .then((result) => {
        tbrgClearBadge();
        sendResponse({
          ok: true,
          templateId: result.templateId,
          resultKeys: Object.keys(result.executionResult.results),
          downloadFilename: result.download.filename,
          screenshotFilenames: (result.screenshotDownloads || []).map((item) => item.filename)
        });
      })
      .catch((error) => {
        tbrgSetErrorBadge();
        sendResponse({ ok: false, error: error.message || String(error) });
      });
    return true;
  }

  if (message?.type === 'TBRG_PICK_ELEMENT') {
    tbrgPickDomElementFromActiveTab()
      .then(async ({ pick, pageUrl }) => {
        const debugPayload = {
          capturedAt: new Date().toISOString(),
          pageUrl,
          pick
        };
        const debugDownload = await tbrgDownloadJsonDebugFile(debugPayload);
        sendResponse({ ok: true, pick, debugJsonFilename: debugDownload.filename });
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
