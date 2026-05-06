if (self.__TBRG_MESSAGE_HANDLER__) {
  chrome.runtime.onMessage.removeListener(self.__TBRG_MESSAGE_HANDLER__);
}

  function tbrgSleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function tbrgNormalizeMatchIndex(matchIndex) {
    const n = Number(matchIndex);
    return Number.isFinite(n) && n >= 0 ? Math.floor(n) : 0;
  }

  function tbrgGetElement(selector, matchIndex = 0) {
    const idx = tbrgNormalizeMatchIndex(matchIndex);
    const elements = document.querySelectorAll(selector);
    const element = elements[idx];
    if (!element) {
      throw new Error(`Selector not found: ${selector} (matchIndex ${idx}, found ${elements.length})`);
    }
    return element;
  }

  function tbrgIsElementVisible(element) {
    if (!element || !(element instanceof Element)) {
      return false;
    }
    if (typeof element.checkVisibility === 'function') {
      try {
        return element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true });
      } catch (_e) {
        // Fall through to geometric checks.
      }
    }
    const rect = element.getBoundingClientRect();
    const style = window.getComputedStyle(element);
    return (
      rect.width > 0 &&
      rect.height > 0 &&
      style.visibility !== 'hidden' &&
      style.display !== 'none' &&
      parseFloat(style.opacity || '1') > 0
    );
  }

  async function tbrgWaitForSelector(selector, timeoutMs, matchIndex = 0) {
    const idx = tbrgNormalizeMatchIndex(matchIndex);
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      const elements = document.querySelectorAll(selector);
      if (elements.length > idx) {
        return elements[idx];
      }
      await tbrgSleep(250);
    }

    throw new Error(`Timed out waiting for selector: ${selector} (matchIndex ${idx})`);
  }

  async function tbrgWaitForHidden(selector, timeoutMs) {
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      const elements = document.querySelectorAll(selector);
      if (elements.length === 0) {
        return true;
      }
      const anyVisible = Array.from(elements).some((node) => tbrgIsElementVisible(node));
      if (!anyVisible) {
        return true;
      }
      await tbrgSleep(250);
    }

    throw new Error(`Timed out waiting for selector to be absent or hidden: ${selector}`);
  }

  function tbrgCssRectToImageRect(bounds, devicePixelRatio) {
    return {
      x: Math.max(0, Math.floor(bounds.x * devicePixelRatio)),
      y: Math.max(0, Math.floor(bounds.y * devicePixelRatio)),
      width: Math.max(1, Math.floor(bounds.width * devicePixelRatio)),
      height: Math.max(1, Math.floor(bounds.height * devicePixelRatio))
    };
  }

  function tbrgAccumulateFrameOffsets(localLeft, localTop) {
    let x = localLeft;
    let y = localTop;
    let currentWindow = window;

    try {
      while (currentWindow !== currentWindow.top) {
        const frameElement = currentWindow.frameElement;
        if (!frameElement) {
          break;
        }
        const frameRect = frameElement.getBoundingClientRect();
        x += frameRect.left;
        y += frameRect.top;
        currentWindow = currentWindow.parent;
      }
    } catch (_error) {
      throw new Error('Cannot resolve iframe-to-viewport offset for this frame (likely cross-origin restrictions).');
    }

    return { x, y };
  }

  function tbrgGetGlobalViewportBounds(element) {
    const rect = element.getBoundingClientRect();
    const origin = tbrgAccumulateFrameOffsets(rect.left, rect.top);

    return {
      x: origin.x,
      y: origin.y,
      width: rect.width,
      height: rect.height
    };
  }

  function tbrgFindHorizontalScrollHost(fromEl) {
    let node = fromEl;
    while (node && node !== document.documentElement && node !== document.body) {
      if (node instanceof Element && node.scrollWidth > node.clientWidth + 2) {
        return node;
      }
      node = node.parentElement;
    }

    if (document.documentElement.scrollWidth > window.innerWidth + 2) {
      return document.documentElement;
    }

    return null;
  }

  function tbrgLocalViewportRectToTabDeviceRect(localLeft, localTop, localWidth, localHeight, devicePixelRatio) {
    const origin = tbrgAccumulateFrameOffsets(localLeft, localTop);
    const dpr = devicePixelRatio || 1;
    return {
      sx: Math.max(0, Math.floor(origin.x * dpr)),
      sy: Math.max(0, Math.floor(origin.y * dpr)),
      sw: Math.max(1, Math.floor(localWidth * dpr)),
      sh: Math.max(1, Math.floor(localHeight * dpr))
    };
  }

  function tbrgClampSourceRect(image, sx, sy, sw, sh) {
    const sx0 = Math.max(0, Math.min(sx, Math.max(0, image.width - 1)));
    const sy0 = Math.max(0, Math.min(sy, Math.max(0, image.height - 1)));
    const sw0 = Math.max(1, Math.min(sw, image.width - sx0));
    const sh0 = Math.max(1, Math.min(sh, image.height - sy0));
    return { sx: sx0, sy: sy0, sw: sw0, sh: sh0 };
  }

  function tbrgScrollHostUsesWindow(scrollHost) {
    return scrollHost === document.documentElement || scrollHost === document.body;
  }

  async function tbrgCaptureWideChartStrips(boundsSource, devicePixelRatio, settleMs) {
    const dpr = devicePixelRatio || 1;
    const scrollHost = tbrgFindHorizontalScrollHost(boundsSource);
    if (!scrollHost) {
      return null;
    }

    const usesWin = tbrgScrollHostUsesWindow(scrollHost);
    const maxSl = usesWin
      ? Math.max(0, document.documentElement.scrollWidth - window.innerWidth)
      : Math.max(0, scrollHost.scrollWidth - scrollHost.clientWidth);

    if (maxSl <= 2) {
      return null;
    }

    const clientSpan = usesWin ? window.innerWidth : scrollHost.clientWidth;
    const step = Math.max(64, Math.floor(clientSpan * 0.55));
    const scrollPositions = [];
    for (let sl = 0; sl < maxSl; sl += step) {
      scrollPositions.push(sl);
    }
    if (scrollPositions.length === 0 || scrollPositions[scrollPositions.length - 1] < maxSl - 2) {
      scrollPositions.push(maxSl);
    }

    const savedHostLeft = scrollHost.scrollLeft;
    const savedHostTop = scrollHost.scrollTop;
    const savedWinX = window.scrollX;
    const savedWinY = window.scrollY;

    function applyHorizontalScroll(sl) {
      if (usesWin) {
        window.scrollTo(sl, savedWinY);
      } else {
        scrollHost.scrollLeft = sl;
      }
    }

    try {
      applyHorizontalScroll(scrollPositions[0]);
      await tbrgSleep(Math.max(40, Math.min(160, settleMs)));
      await new Promise((resolve) => requestAnimationFrame(resolve));

      const baseRect = boundsSource.getBoundingClientRect();
      const outW = Math.max(1, Math.ceil(baseRect.width * dpr));
      const outH = Math.max(1, Math.ceil(baseRect.height * dpr));

      let outputCanvas;
      if (typeof OffscreenCanvas !== 'undefined') {
        outputCanvas = new OffscreenCanvas(outW, outH);
      } else {
        outputCanvas = document.createElement('canvas');
        outputCanvas.width = outW;
        outputCanvas.height = outH;
      }
      const outputCtx = outputCanvas.getContext('2d');
      if (!outputCtx) {
        throw new Error('Cannot create 2D context for stitched screenshot.');
      }

      const vw = window.innerWidth;
      const vh = window.innerHeight;

      for (let i = 0; i < scrollPositions.length; i += 1) {
        applyHorizontalScroll(scrollPositions[i]);
        await tbrgSleep(Math.max(40, Math.min(160, settleMs)));
        await new Promise((resolve) => requestAnimationFrame(resolve));

        const canvasRect = boundsSource.getBoundingClientRect();
        const visLeft = Math.max(0, canvasRect.left);
        const visTop = Math.max(0, canvasRect.top);
        const visRight = Math.min(vw, canvasRect.right);
        const visBottom = Math.min(vh, canvasRect.bottom);
        const vwCss = visRight - visLeft;
        const vhCss = visBottom - visTop;

        if (vwCss <= 2 || vhCss <= 2) {
          continue;
        }

        const tabDev = tbrgLocalViewportRectToTabDeviceRect(visLeft, visTop, vwCss, vhCss, dpr);
        const response = await chrome.runtime.sendMessage({
          type: 'TBRG_CAPTURE_VISIBLE_TAB'
        });

        if (!response || !response.ok || !response.dataUrl) {
          throw new Error(response?.error || 'Failed to capture visible tab.');
        }

        const shot = await tbrgLoadImage(response.dataUrl);
        const src = tbrgClampSourceRect(shot, tabDev.sx, tabDev.sy, tabDev.sw, tabDev.sh);
        const dstX = Math.round((visLeft - canvasRect.left) * dpr);
        const dstY = Math.round((visTop - canvasRect.top) * dpr);

        outputCtx.drawImage(shot, src.sx, src.sy, src.sw, src.sh, dstX, dstY, src.sw, src.sh);
      }

      if (typeof outputCanvas.convertToBlob === 'function') {
        const blob = await outputCanvas.convertToBlob({ type: 'image/png' });
        return await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onloadend = () => resolve(reader.result);
          reader.onerror = () => reject(new Error('Failed to encode stitched screenshot.'));
          reader.readAsDataURL(blob);
        });
      }

      return outputCanvas.toDataURL('image/png');
    } finally {
      if (usesWin) {
        window.scrollTo(savedWinX, savedWinY);
      } else {
        scrollHost.scrollLeft = savedHostLeft;
        scrollHost.scrollTop = savedHostTop;
      }
    }
  }

  async function tbrgLoadImage(dataUrl) {
    return new Promise((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = () => reject(new Error('Failed to decode screenshot image.'));
      image.src = dataUrl;
    });
  }

  async function tbrgCropScreenshot(dataUrl, cssBounds, devicePixelRatio) {
    const image = await tbrgLoadImage(dataUrl);
    const pixelRect = tbrgCssRectToImageRect(cssBounds, devicePixelRatio);
    const cropWidth = Math.min(pixelRect.width, Math.max(0, image.width - pixelRect.x));
    const cropHeight = Math.min(pixelRect.height, Math.max(0, image.height - pixelRect.y));
    const truncated =
      cropWidth < pixelRect.width - 1 ||
      cropHeight < pixelRect.height - 1 ||
      pixelRect.x + pixelRect.width > image.width + 1 ||
      pixelRect.y + pixelRect.height > image.height + 1;

    if (cropWidth <= 0 || cropHeight <= 0) {
      throw new Error('Calculated crop area is outside the captured viewport.');
    }

    let encoded;

    if (typeof OffscreenCanvas !== 'undefined') {
      try {
        const canvas = new OffscreenCanvas(cropWidth, cropHeight);
        const context = canvas.getContext('2d');
        if (!context) {
          throw new Error('OffscreenCanvas 2D context is unavailable.');
        }
        context.drawImage(
          image,
          pixelRect.x,
          pixelRect.y,
          cropWidth,
          cropHeight,
          0,
          0,
          cropWidth,
          cropHeight
        );
        const blob = await canvas.convertToBlob({ type: 'image/png' });
        encoded = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onloadend = () => resolve(reader.result);
          reader.onerror = () => reject(new Error('Failed to convert cropped screenshot to data URL.'));
          reader.readAsDataURL(blob);
        });
      } catch (_error) {
        encoded = null;
      }
    }

    if (!encoded) {
      const canvas = document.createElement('canvas');
      canvas.width = cropWidth;
      canvas.height = cropHeight;
      const context = canvas.getContext('2d');
      if (!context) {
        throw new Error('Canvas 2D context is unavailable.');
      }
      context.drawImage(
        image,
        pixelRect.x,
        pixelRect.y,
        cropWidth,
        cropHeight,
        0,
        0,
        cropWidth,
        cropHeight
      );
      encoded = canvas.toDataURL('image/png');
    }

    return { dataUrl: encoded, truncated };
  }

  async function tbrgCaptureElement(selector, matchIndex, preCaptureDelayMs, boundsSelector, stitchOverflow) {
    let root = tbrgGetElement(selector, matchIndex);
    root.scrollIntoView({ behavior: 'auto', block: 'center', inline: 'start' });
    const settleMs = Number(preCaptureDelayMs) > 0 ? Number(preCaptureDelayMs) : 450;
    await tbrgSleep(settleMs);
    root = tbrgGetElement(selector, matchIndex);

    let boundsSource = root;
    if (boundsSelector) {
      const inner = root.querySelector(boundsSelector);
      if (inner) {
        boundsSource = inner;
      }
    } else {
      const chartCanvas = root.querySelector('canvas.chart');
      if (chartCanvas) {
        boundsSource = chartCanvas;
      }
    }

    const dpr = window.devicePixelRatio || 1;
    const allowStitch = stitchOverflow !== false;
    const scrollHost = tbrgFindHorizontalScrollHost(boundsSource);
    const chartRect = boundsSource.getBoundingClientRect();
    const hostSpan = scrollHost && !tbrgScrollHostUsesWindow(scrollHost) ? scrollHost.clientWidth : window.innerWidth;
    const shouldTryStitch =
      allowStitch &&
      scrollHost &&
      scrollHost.scrollWidth > scrollHost.clientWidth + 2 &&
      chartRect.width > hostSpan + 4;

    if (shouldTryStitch) {
      const stitched = await tbrgCaptureWideChartStrips(boundsSource, dpr, settleMs);
      if (stitched) {
        return stitched;
      }
    }

    const bounds = tbrgGetGlobalViewportBounds(boundsSource);
    const response = await chrome.runtime.sendMessage({
      type: 'TBRG_CAPTURE_VISIBLE_TAB'
    });

    if (!response || !response.ok || !response.dataUrl) {
      throw new Error(response?.error || 'Failed to capture visible tab.');
    }

    const { dataUrl, truncated } = await tbrgCropScreenshot(response.dataUrl, bounds, dpr);

    if (
      truncated &&
      allowStitch &&
      scrollHost &&
      scrollHost.scrollWidth > scrollHost.clientWidth + 2
    ) {
      const stitched = await tbrgCaptureWideChartStrips(boundsSource, dpr, settleMs);
      if (stitched) {
        return stitched;
      }
    }

    return dataUrl;
  }

  async function tbrgRunStep(step) {
    const timeoutMs = Number(step.timeoutMs) > 0 ? Number(step.timeoutMs) : 10000;
    const matchIndex = step.matchIndex;

    if (step.type === 'delay') {
      const durationMs = Number(step.durationMs) > 0 ? Number(step.durationMs) : 0;
      if (durationMs <= 0) {
        throw new Error('delay step requires durationMs > 0');
      }
      await tbrgSleep(durationMs);
      return { ok: true, value: true };
    }

    if (step.type === 'waitFor') {
      await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      return { ok: true, value: true };
    }

    if (step.type === 'waitForHidden') {
      await tbrgWaitForHidden(step.selector, timeoutMs);
      return { ok: true, value: true };
    }

    if (step.type === 'click') {
      const element = await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      element.click();
      return { ok: true, value: true };
    }

    if (step.type === 'text') {
      const element = await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      return { ok: true, value: element.textContent.trim() };
    }

    if (step.type === 'attribute') {
      const element = await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      return { ok: true, value: element.getAttribute(step.attribute) ?? '' };
    }

    if (step.type === 'screenshot') {
      await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      const imageDataUrl = await tbrgCaptureElement(
        step.selector,
        matchIndex,
        step.preCaptureDelayMs,
        typeof step.boundsSelector === 'string' ? step.boundsSelector : '',
        step.stitchOverflow
      );
      return { ok: true, value: imageDataUrl };
    }

    throw new Error(`Unsupported step type: ${step.type}`);
  }

  async function tbrgExecuteTemplate(template) {
    const results = {};
    const stepResults = [];
    const taskResults = [];
    const stopOnFailure = template?.stopOnFailure !== false;
    const stepsToRun = Array.isArray(template.tasks)
      ? template.tasks.flatMap((task) =>
        (task.steps || []).map((step) => ({ ...step, __taskId: task.id || 'task' }))
      )
      : template.steps;

    for (const step of stepsToRun) {
      try {
        const stepResult = await tbrgRunStep(step);
        if (step.id) {
          results[step.id] = stepResult.value;
        }
        stepResults.push({
          id: step.id,
          taskId: step.__taskId || null,
          ...stepResult
        });
      } catch (error) {
        stepResults.push({
          id: step.id,
          taskId: step.__taskId || null,
          ok: false,
          error: error.message || String(error)
        });
        if (stopOnFailure) {
          break;
        }
      }
    }

    if (Array.isArray(template.tasks)) {
      for (const task of template.tasks) {
        const relatedSteps = stepResults.filter((stepResult) => stepResult.taskId === (task.id || 'task'));
        taskResults.push({
          id: task.id || 'task',
          ok: relatedSteps.every((stepResult) => stepResult.ok),
          steps: relatedSteps
        });
      }
    }

    return {
      ok: true,
      url: location.href,
      title: document.title,
      results,
      stepResults,
      taskResults
    };
  }

  self.__TBRG_MESSAGE_HANDLER__ = (message, _sender, sendResponse) => {
    if (message?.type !== 'TBRG_EXECUTE_TEMPLATE') {
      return false;
    }

    tbrgExecuteTemplate(message.template)
      .then((result) => sendResponse(result))
      .catch((error) => {
        sendResponse({
          ok: false,
          error: error.message || String(error)
        });
      });

    return true;
  };

  chrome.runtime.onMessage.addListener(self.__TBRG_MESSAGE_HANDLER__);
