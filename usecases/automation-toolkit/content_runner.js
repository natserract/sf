if (self.__TBRG_MESSAGE_HANDLER__) {
  chrome.runtime.onMessage.removeListener(self.__TBRG_MESSAGE_HANDLER__);
}

(() => {

  function tbrgSleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function tbrgNow() {
    return Date.now();
  }

  const TBRG_AUTOMATION_HILITE_PANEL_ID = 'tbrg-automation-selector-panel';
  const TBRG_AUTOMATION_HILITE_RING_ID = 'tbrg-automation-selector-ring';

  let tbrgAutomationStepContext = null;
  let tbrgAutomationLastAnnounceKey = '';
  let tbrgAutomationLastAnnounceAt = 0;

  function tbrgAutomationSetStepContext(ctx) {
    tbrgAutomationStepContext = ctx && typeof ctx === 'object' ? ctx : null;
  }

  function tbrgAutomationRemoveDomHighlight() {
    document.getElementById(TBRG_AUTOMATION_HILITE_PANEL_ID)?.remove();
    document.getElementById(TBRG_AUTOMATION_HILITE_RING_ID)?.remove();
  }

  function tbrgEnsureAutomationHighlighterStyles() {
    const id = 'tbrg-automation-dom-highlight-styles';
    if (document.getElementById(id)) {
      return;
    }
    const style = document.createElement('style');
    style.id = id;
    style.textContent = `
      #${TBRG_AUTOMATION_HILITE_PANEL_ID} {
        position: fixed;
        left: 12px;
        bottom: 12px;
        max-width: min(560px, calc(100vw - 24px));
        z-index: 2147483640;
        background: rgba(15, 23, 42, 0.94);
        color: #e2e8f0;
        border: 1px solid rgba(148, 163, 184, 0.35);
        border-radius: 10px;
        padding: 10px 12px;
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        font-size: 11px;
        line-height: 1.45;
        box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
        pointer-events: none;
      }
      #${TBRG_AUTOMATION_HILITE_PANEL_ID} .tbrg-automation-dom-step {
        color: #94a3b8;
        margin-bottom: 6px;
        font-size: 10px;
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }
      #${TBRG_AUTOMATION_HILITE_PANEL_ID} .tbrg-automation-dom-sel {
        color: #7dd3fc;
        word-break: break-all;
        margin-bottom: 6px;
      }
      #${TBRG_AUTOMATION_HILITE_PANEL_ID} .tbrg-automation-dom-chain-label {
        color: #94a3b8;
        margin-bottom: 2px;
      }
      #${TBRG_AUTOMATION_HILITE_PANEL_ID} .tbrg-automation-dom-chain {
        margin: 0;
        white-space: pre-wrap;
        word-break: break-all;
        color: #bbf7d0;
      }
      #${TBRG_AUTOMATION_HILITE_RING_ID} {
        position: fixed;
        z-index: 2147483639;
        border: 2px solid #22c55e;
        border-radius: 4px;
        box-sizing: border-box;
        pointer-events: none;
        box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.2) inset;
      }
    `;
    document.documentElement.appendChild(style);
  }

  function tbrgCssEscapeForChain(value) {
    if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') {
      return CSS.escape(value);
    }
    return String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  }

  function tbrgBuildSelectorChainFromElement(element) {
    const chain = [];
    let node = element;

    while (node && node.nodeType === Node.ELEMENT_NODE && chain.length < 8) {
      const tag = node.tagName.toLowerCase();
      const idAttr = node.getAttribute('id');
      if (idAttr) {
        chain.unshift(`#${tbrgCssEscapeForChain(idAttr)}`);
        break;
      }

      const dataTestId = node.getAttribute('data-testid') || node.getAttribute('data-test-id');
      if (dataTestId) {
        chain.unshift(`[data-testid="${dataTestId}"]`);
        break;
      }

      if (node.getAttribute('role')) {
        chain.unshift(`${tag}[role="${node.getAttribute('role')}"]`);
      } else if (node.classList.length > 0) {
        chain.unshift(`${tag}.${tbrgCssEscapeForChain(node.classList[0])}`);
      } else {
        chain.unshift(tag);
      }

      node = node.parentElement;
    }

    return chain.join(' > ');
  }

  function tbrgAutomationShowDomAccess(element, { selector, matchIndex } = {}) {
    if (!(element instanceof Element)) {
      return;
    }
    const idx = tbrgNormalizeMatchIndex(matchIndex);
    const sel = typeof selector === 'string' ? selector : '';
    const dedupeKey = `${sel}|${idx}|${tbrgAutomationStepContext?.type || ''}|${tbrgAutomationStepContext?.operator || ''}`;
    const now = Date.now();
    if (dedupeKey === tbrgAutomationLastAnnounceKey && now - tbrgAutomationLastAnnounceAt < 150) {
      return;
    }
    tbrgAutomationLastAnnounceKey = dedupeKey;
    tbrgAutomationLastAnnounceAt = now;

    tbrgEnsureAutomationHighlighterStyles();
    tbrgAutomationRemoveDomHighlight();

    const chain = tbrgBuildSelectorChainFromElement(element) || element.tagName?.toLowerCase() || '(unknown)';
    const ctx = tbrgAutomationStepContext;
    const stepLine = ctx ? [ctx.type, ctx.operator, ctx.id].filter(Boolean).join(' · ') : 'Automation';

    const panel = tbrgCreateEl('div', { id: TBRG_AUTOMATION_HILITE_PANEL_ID }, []);
    panel.appendChild(tbrgCreateEl('div', { class: 'tbrg-automation-dom-step', text: `DOM — ${stepLine}` }));
    panel.appendChild(
      tbrgCreateEl('div', {
        class: 'tbrg-automation-dom-sel',
        text: `Selector: ${sel || '(n/a)'}  ·  matchIndex: ${idx}`
      })
    );
    panel.appendChild(tbrgCreateEl('div', { class: 'tbrg-automation-dom-chain-label', text: 'Selector chain (ancestor → target):' }));
    panel.appendChild(tbrgCreateEl('pre', { class: 'tbrg-automation-dom-chain', text: chain }));

    document.documentElement.appendChild(panel);

    try {
      const rect = element.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        const ring = tbrgCreateEl('div', { id: TBRG_AUTOMATION_HILITE_RING_ID }, []);
        Object.assign(ring.style, {
          left: `${rect.left}px`,
          top: `${rect.top}px`,
          width: `${rect.width}px`,
          height: `${rect.height}px`
        });
        document.documentElement.appendChild(ring);
      }
    } catch (_e) {
      // Ring is optional.
    }
  }

  function tbrgToDisplayString(value) {
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
      return typeof json === 'string' ? json : String(value);
    } catch (_e) {
      return String(value);
    }
  }

  function tbrgCreateEl(tag, attrs = {}, children = []) {
    const el = document.createElement(tag);
    Object.entries(attrs || {}).forEach(([k, v]) => {
      if (v == null) {
        return;
      }
      if (k === 'class') {
        el.className = String(v);
        return;
      }
      if (k === 'text') {
        el.textContent = String(v);
        return;
      }
      if (k === 'html') {
        el.innerHTML = String(v);
        return;
      }
      if (k.startsWith('on') && typeof v === 'function') {
        el.addEventListener(k.slice(2).toLowerCase(), v);
        return;
      }
      el.setAttribute(k, String(v));
    });
    for (const child of children || []) {
      if (child == null) {
        continue;
      }
      el.appendChild(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return el;
  }

  function tbrgEnsureDialogStyles() {
    const existing = document.getElementById('tbrg-input-dialog-styles');
    if (existing) {
      return;
    }
    const style = document.createElement('style');
    style.id = 'tbrg-input-dialog-styles';
    style.textContent = `
      .tbrg-overlay {
        position: fixed;
        inset: 0;
        z-index: 2147483647;
        background: rgba(0, 0, 0, 0.55);
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 16px;
      }
      .tbrg-modal {
        width: min(720px, 100%);
        background: #0b1220;
        color: #e5e7eb;
        border: 1px solid rgba(255, 255, 255, 0.12);
        border-radius: 12px;
        box-shadow: 0 20px 60px rgba(0, 0, 0, 0.55);
        font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
      }
      .tbrg-modal-header {
        padding: 14px 16px 10px;
        border-bottom: 1px solid rgba(255, 255, 255, 0.10);
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
      }
      .tbrg-title {
        font-size: 14px;
        font-weight: 600;
        margin: 0;
      }
      .tbrg-badge {
        font-size: 12px;
        opacity: 0.9;
      }
      .tbrg-modal-body {
        padding: 14px 16px 6px;
      }
      .tbrg-label {
        display: block;
        font-size: 13px;
        font-weight: 600;
        margin: 0 0 6px;
      }
      .tbrg-help {
        margin: 0 0 10px;
        font-size: 12px;
        opacity: 0.85;
        line-height: 1.35;
      }
      .tbrg-input, .tbrg-textarea {
        width: 100%;
        box-sizing: border-box;
        padding: 10px 10px;
        border-radius: 10px;
        border: 1px solid rgba(255, 255, 255, 0.14);
        background: rgba(255, 255, 255, 0.06);
        color: #e5e7eb;
        outline: none;
        font-size: 13px;
      }
      .tbrg-textarea {
        min-height: 120px;
        resize: vertical;
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      }
      .tbrg-error {
        margin-top: 10px;
        color: #fecaca;
        background: rgba(220, 38, 38, 0.12);
        border: 1px solid rgba(220, 38, 38, 0.22);
        padding: 10px;
        border-radius: 10px;
        font-size: 12px;
        white-space: pre-wrap;
      }
      .tbrg-modal-footer {
        padding: 12px 16px 14px;
        border-top: 1px solid rgba(255, 255, 255, 0.10);
        display: flex;
        justify-content: flex-end;
        gap: 10px;
      }
      .tbrg-btn {
        padding: 9px 12px;
        border-radius: 10px;
        border: 1px solid rgba(255, 255, 255, 0.14);
        background: rgba(255, 255, 255, 0.08);
        color: #e5e7eb;
        font-size: 13px;
        cursor: pointer;
      }
      .tbrg-btn-primary {
        background: #2563eb;
        border-color: rgba(37, 99, 235, 0.65);
      }
      .tbrg-btn:disabled {
        opacity: 0.55;
        cursor: not-allowed;
      }
    `;
    document.documentElement.appendChild(style);
  }

  function tbrgUserCancelledError(reason) {
    const error = new Error(reason || 'User cancelled input.');
    error.name = 'TBRG_USER_CANCELLED';
    return error;
  }

  function tbrgShowBlockingDialog({ title, badge, label, helpText, bodyEl, primaryText, timeoutMs }) {
    tbrgEnsureDialogStyles();

    const existing = document.getElementById('tbrg-input-dialog-overlay');
    if (existing) {
      existing.remove();
    }

    const errorBox = tbrgCreateEl('div', { class: 'tbrg-error', style: 'display:none' }, []);

    const overlay = tbrgCreateEl('div', { class: 'tbrg-overlay', id: 'tbrg-input-dialog-overlay' }, []);
    const modal = tbrgCreateEl('div', { class: 'tbrg-modal', role: 'dialog', 'aria-modal': 'true' }, []);
    const header = tbrgCreateEl('div', { class: 'tbrg-modal-header' }, [
      tbrgCreateEl('h3', { class: 'tbrg-title', text: title || 'Automation Input' }),
      tbrgCreateEl('div', { class: 'tbrg-badge', text: badge || '' })
    ]);
    const body = tbrgCreateEl('div', { class: 'tbrg-modal-body' }, [
      label ? tbrgCreateEl('div', { class: 'tbrg-label', text: label }) : null,
      helpText ? tbrgCreateEl('p', { class: 'tbrg-help', text: helpText }) : null,
      bodyEl,
      errorBox
    ].filter(Boolean));

    const cancelBtn = tbrgCreateEl('button', { class: 'tbrg-btn', type: 'button', text: 'Cancel' });
    const okBtn = tbrgCreateEl('button', { class: 'tbrg-btn tbrg-btn-primary', type: 'button', text: primaryText || 'Submit' });
    const footer = tbrgCreateEl('div', { class: 'tbrg-modal-footer' }, [cancelBtn, okBtn]);

    modal.appendChild(header);
    modal.appendChild(body);
    modal.appendChild(footer);
    overlay.appendChild(modal);
    document.documentElement.appendChild(overlay);

    function setError(msg) {
      if (!msg) {
        errorBox.style.display = 'none';
        errorBox.textContent = '';
        return;
      }
      errorBox.style.display = 'block';
      errorBox.textContent = String(msg);
    }

    let finished = false;
    let timeoutId = null;

    function cleanup() {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      overlay.remove();
      document.removeEventListener('keydown', onKeydown, true);
    }

    function onKeydown(e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        cancel();
      }
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        e.stopPropagation();
        confirm();
      }
    }

    function cancel() {
      if (finished) return;
      finished = true;
      cleanup();
      reject(tbrgUserCancelledError('User cancelled input.'));
    }

    async function confirm() {
      if (finished) return;
      setError('');
      try {
        okBtn.disabled = true;
        cancelBtn.disabled = true;
        const value = await onConfirm();
        finished = true;
        cleanup();
        resolve(value);
      } catch (e) {
        okBtn.disabled = false;
        cancelBtn.disabled = false;
        setError(e?.message || String(e));
      }
    }

    let resolve;
    let reject;
    let onConfirm = async () => true;

    const promise = new Promise((res, rej) => {
      resolve = res;
      reject = rej;
    });

    cancelBtn.addEventListener('click', cancel);
    okBtn.addEventListener('click', confirm);
    overlay.addEventListener('mousedown', (e) => {
      if (e.target === overlay) {
        cancel();
      }
    });
    document.addEventListener('keydown', onKeydown, true);

    if (Number(timeoutMs) > 0) {
      timeoutId = setTimeout(() => {
        if (finished) return;
        finished = true;
        cleanup();
        reject(new Error(`Timed out waiting for user input (${Math.round(Number(timeoutMs) / 1000)}s).`));
      }, Number(timeoutMs));
    }

    requestAnimationFrame(() => {
      try {
        const focusable = modal.querySelector('input, textarea, button, select, [tabindex]:not([tabindex="-1"])');
        if (focusable) {
          focusable.focus();
        } else {
          okBtn.focus();
        }
      } catch (_e) {
        // Ignore focus errors.
      }
    });

    return {
      setConfirmHandler(handler) {
        onConfirm = handler;
      },
      promise
    };
  }

  async function tbrgReadFileAsText(file, maxBytes) {
    if (!file) {
      throw new Error('No file selected.');
    }
    if (Number(maxBytes) > 0 && file.size > Number(maxBytes)) {
      throw new Error(`File is too large (${file.size} bytes). Max is ${Number(maxBytes)} bytes.`);
    }
    return await new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(new Error('Failed to read file.'));
      reader.readAsText(file);
    });
  }

  async function tbrgReadFileAsDataUrl(file, maxBytes) {
    if (!file) {
      throw new Error('No file selected.');
    }
    if (Number(maxBytes) > 0 && file.size > Number(maxBytes)) {
      throw new Error(`File is too large (${file.size} bytes). Max is ${Number(maxBytes)} bytes.`);
    }
    return await new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(new Error('Failed to read file.'));
      reader.readAsDataURL(file);
    });
  }

  function tbrgParseCsvLine(line, delimiter) {
    const out = [];
    let cur = '';
    let inQuotes = false;
    for (let i = 0; i < line.length; i += 1) {
      const ch = line[i];
      if (inQuotes) {
        if (ch === '"') {
          const next = line[i + 1];
          if (next === '"') {
            cur += '"';
            i += 1;
          } else {
            inQuotes = false;
          }
        } else {
          cur += ch;
        }
      } else {
        if (ch === '"') {
          inQuotes = true;
        } else if (ch === delimiter) {
          out.push(cur);
          cur = '';
        } else {
          cur += ch;
        }
      }
    }
    out.push(cur);
    return out;
  }

  function tbrgParseCsv(text, { delimiter = ',', hasHeader = true, maxRows = 5000 } = {}) {
    const del = typeof delimiter === 'string' && delimiter.length ? delimiter : ',';
    const safeMax = Number(maxRows) > 0 ? Math.floor(Number(maxRows)) : 5000;
    const raw = String(text || '');
    const normalized = raw.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    const lines = normalized.split('\n').filter((l) => l.trim().length > 0);
    if (lines.length === 0) {
      return { headers: [], rows: [] };
    }

    const rows = [];
    const first = tbrgParseCsvLine(lines[0], del).map((v) => v.trim());
    let headers = [];
    let startIndex = 0;

    if (hasHeader !== false) {
      headers = first.map((h, idx) => (h ? h : `col_${idx + 1}`));
      startIndex = 1;
    } else {
      headers = first.map((_h, idx) => `col_${idx + 1}`);
      rows.push(Object.fromEntries(headers.map((h, i) => [h, first[i] ?? ''])));
      startIndex = 1;
    }

    for (let i = startIndex; i < lines.length && rows.length < safeMax; i += 1) {
      const cols = tbrgParseCsvLine(lines[i], del);
      const row = {};
      for (let c = 0; c < headers.length; c += 1) {
        row[headers[c]] = (cols[c] ?? '').trim();
      }
      rows.push(row);
    }
    return { headers, rows };
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
    tbrgAutomationShowDomAccess(element, { selector, matchIndex: idx });
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

  /**
   * Some SPAs (Angular, etc.) ignore HTMLElement.click(); dispatch pointer/mouse events at element center.
   */
  function tbrgDispatchSyntheticPointerClick(element) {
    if (!(element instanceof Element)) {
      return;
    }
    const rect = element.getBoundingClientRect();
    const cx = Math.min(Math.max(rect.left + rect.width / 2, rect.left + 1), rect.right - 1);
    const cy = Math.min(Math.max(rect.top + rect.height / 2, rect.top + 1), rect.bottom - 1);
    const common = {
      bubbles: true,
      cancelable: true,
      composed: true,
      view: window,
      clientX: cx,
      clientY: cy,
      button: 0,
      buttons: 1
    };

    try {
      if (typeof element.focus === 'function') {
        element.focus({ preventScroll: true });
      }
    } catch (_e) {
      // ignore
    }

    const fire = (Ctor, type, extra = {}) => {
      try {
        element.dispatchEvent(new Ctor(type, { ...common, ...extra }));
      } catch (_e) {
        // ignore
      }
    };

    if (typeof PointerEvent === 'function') {
      fire(PointerEvent, 'pointerover', { pointerId: 1, pointerType: 'mouse', isPrimary: true });
      fire(PointerEvent, 'pointerenter', { pointerId: 1, pointerType: 'mouse', isPrimary: true });
    }
    fire(MouseEvent, 'mouseover', {});
    fire(MouseEvent, 'mousedown', {});
    if (typeof PointerEvent === 'function') {
      fire(PointerEvent, 'pointerdown', { pointerId: 1, pointerType: 'mouse', isPrimary: true });
    }
    fire(MouseEvent, 'mouseup', { buttons: 0 });
    if (typeof PointerEvent === 'function') {
      fire(PointerEvent, 'pointerup', { pointerId: 1, pointerType: 'mouse', isPrimary: true });
    }
    fire(MouseEvent, 'click', {});
  }

  async function tbrgWaitForSelector(selector, timeoutMs, matchIndex = 0, waitOptions = {}) {
    const requireVisible = waitOptions.requireVisible === true;
    const idx = tbrgNormalizeMatchIndex(matchIndex);
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      const elements = document.querySelectorAll(selector);
      if (elements.length > idx) {
        const candidate = elements[idx];
        if (!requireVisible || tbrgIsElementVisible(candidate)) {
          tbrgAutomationShowDomAccess(candidate, { selector, matchIndex: idx });
          return candidate;
        }
      }
      await tbrgSleep(250);
    }

    const visHint = requireVisible ? ' (visible)' : '';
    throw new Error(`Timed out waiting for selector: ${selector} (matchIndex ${idx})${visHint}`);
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

  async function tbrgDismissSelectors(selectors, attempts = 1, postDismissDelayMs = 0) {
    const list = Array.isArray(selectors) ? selectors.filter((s) => typeof s === 'string' && s.trim()) : [];
    if (list.length === 0) {
      return false;
    }
    const maxAttempts = Number(attempts) > 0 ? Math.min(8, Math.floor(Number(attempts))) : 1;
    let clickedAny = false;
    for (let i = 0; i < maxAttempts; i += 1) {
      let clickedThisRound = false;
      for (const selector of list) {
        const nodes = Array.from(document.querySelectorAll(selector));
        for (const node of nodes) {
          if (!(node instanceof Element)) {
            continue;
          }
          if (!tbrgIsElementVisible(node)) {
            continue;
          }
          node.scrollIntoView({ behavior: 'auto', block: 'center', inline: 'center' });
          await tbrgSleep(40);
          node.click();
          clickedThisRound = true;
          clickedAny = true;
        }
      }
      if (!clickedThisRound) {
        break;
      }
      const settle = Number(postDismissDelayMs) > 0 ? Number(postDismissDelayMs) : 140;
      await tbrgSleep(settle);
    }
    return clickedAny;
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

  async function tbrgCaptureElement(selector, matchIndex, preCaptureDelayMs, boundsSelector, stitchOverflow, captureOptions = {}) {
    await tbrgDismissSelectors(
      captureOptions.dismissSelectors,
      captureOptions.dismissAttempts,
      captureOptions.postDismissDelayMs
    );
    const readySel = typeof captureOptions.readySelector === 'string' ? captureOptions.readySelector.trim() : '';
    if (readySel) {
      const rTimeout =
        Number(captureOptions.readyTimeoutMs) > 0 ? Number(captureOptions.readyTimeoutMs) : 60000;
      const rIdx = tbrgNormalizeMatchIndex(captureOptions.readyMatchIndex);
      const rRequireVisible = captureOptions.readyRequireVisible !== false;
      await tbrgWaitForSelector(readySel, rTimeout, rIdx, { requireVisible: rRequireVisible });
    }
    let root = tbrgGetElement(selector, matchIndex);
    const scrollBlock = typeof captureOptions.captureScrollBlock === 'string' && captureOptions.captureScrollBlock.trim()
      ? captureOptions.captureScrollBlock.trim()
      : 'center';
    root.scrollIntoView({ behavior: 'auto', block: scrollBlock, inline: 'start' });
    const settleMs = Number(captureOptions.captureSettleMs) > 0
      ? Number(captureOptions.captureSettleMs)
      : (Number(preCaptureDelayMs) > 0 ? Number(preCaptureDelayMs) : 450);
    await tbrgSleep(settleMs);
    root = tbrgGetElement(selector, matchIndex);
    if (captureOptions.captureEnsureVisible !== false && !tbrgIsElementVisible(root)) {
      throw new Error(`Capture target "${selector}" is not visible after scrolling.`);
    }

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

  function tbrgToNumberForAggregation(value) {
    if (typeof value === 'number') {
      return Number.isFinite(value) ? value : null;
    }
    const text = String(value == null ? '' : value).trim();
    if (!text) {
      return null;
    }
    const normalized = text.endsWith('%') ? text.slice(0, -1).trim() : text;
    const parsed = Number(normalized);
    return Number.isFinite(parsed) ? parsed : null;
  }

  async function tbrgRunStep(step, resultsContext) {
    const timeoutMs = Number(step.timeoutMs) > 0 ? Number(step.timeoutMs) : 10000;
    const matchIndex = step.matchIndex;
    const type = String(step.type || '').trim();
    const operator = String(step.operator || '').trim();

    tbrgAutomationSetStepContext({
      id: typeof step.id === 'string' ? step.id.trim() : '',
      type,
      operator
    });

    if (type === 'text' && operator === 'input') {
      const stepId = String(step.id || '').trim();
      if (!stepId) {
        throw new Error('text.input step requires id');
      }
      const label = typeof step.label === 'string' ? step.label : stepId;
      const helpText = typeof step.helpText === 'string' ? step.helpText : '';
      const isMultiline = step.multiline === true;
      const placeholder = typeof step.placeholder === 'string' ? step.placeholder : '';
      const def = typeof step.default === 'string' ? step.default : '';

      const inputEl = isMultiline
        ? tbrgCreateEl('textarea', { class: 'tbrg-textarea', placeholder }, [])
        : tbrgCreateEl('input', { class: 'tbrg-input', type: 'text', placeholder }, []);
      inputEl.value = def;

      const dialog = tbrgShowBlockingDialog({
        title: 'Automation Input',
        badge: stepId,
        label,
        helpText,
        bodyEl: inputEl,
        primaryText: 'Submit',
        timeoutMs
      });

      dialog.setConfirmHandler(async () => {
        return String(inputEl.value || '');
      });

      const value = await dialog.promise;
      return { ok: true, value };
    }

    if (type === 'csv' && operator === 'input') {
      const stepId = String(step.id || '').trim();
      if (!stepId) {
        throw new Error('csv.input step requires id');
      }
      const label = typeof step.label === 'string' ? step.label : stepId;
      const helpText = typeof step.helpText === 'string' ? step.helpText : '';
      const delimiter = typeof step.delimiter === 'string' ? step.delimiter : ',';
      const hasHeader = step.hasHeader !== false;
      const maxRows = Number(step.maxRows) > 0 ? Number(step.maxRows) : 5000;
      const maxBytes = Number(step.maxBytes) > 0 ? Number(step.maxBytes) : 0;

      const fileInput = tbrgCreateEl('input', { class: 'tbrg-input', type: 'file', accept: '.csv,text/csv' }, []);

      const dialog = tbrgShowBlockingDialog({
        title: 'Automation Input',
        badge: stepId,
        label,
        helpText,
        bodyEl: fileInput,
        primaryText: 'Use file',
        timeoutMs
      });

      dialog.setConfirmHandler(async () => {
        const file = fileInput.files && fileInput.files[0] ? fileInput.files[0] : null;
        const text = await tbrgReadFileAsText(file, maxBytes);
        const parsed = tbrgParseCsv(text, { delimiter, hasHeader, maxRows });
        return {
          name: file ? file.name : '',
          size: file ? file.size : 0,
          headers: parsed.headers,
          rows: parsed.rows
        };
      });

      const value = await dialog.promise;
      return { ok: true, value };
    }

    if (type === 'csv' && operator === 'aggregate') {
      const stepId = String(step.id || '').trim();
      if (!stepId) {
        throw new Error('csv.aggregate step requires id');
      }
      const sourceValue = String(step.sourceValue || '').trim();
      const aggregateType = String(step.aggregateType || '').trim();
      const column = String(step.column || '').trim();
      if (!sourceValue || !aggregateType || !column) {
        throw new Error('csv.aggregate requires sourceValue, aggregateType, and column.');
      }
      if (aggregateType !== 'sum') {
        throw new Error(`Unsupported csv.aggregate aggregateType "${aggregateType}".`);
      }

      const source = resultsContext && typeof resultsContext === 'object' ? resultsContext[sourceValue] : null;
      const rows = Array.isArray(source?.rows) ? source.rows : null;
      if (!rows) {
        throw new Error(`csv.aggregate source "${sourceValue}" does not contain parsed CSV rows.`);
      }

      let sum = 0;
      let includedCount = 0;
      for (const row of rows) {
        const raw = row && typeof row === 'object' ? row[column] : undefined;
        const parsed = tbrgToNumberForAggregation(raw);
        if (parsed == null) {
          continue;
        }
        sum += parsed;
        includedCount += 1;
      }
      if (includedCount === 0) {
        throw new Error(`csv.aggregate found no numeric values in column "${column}" from "${sourceValue}".`);
      }
      return { ok: true, value: sum };
    }

    if (type === 'image' && operator === 'input') {
      const stepId = String(step.id || '').trim();
      if (!stepId) {
        throw new Error('image.input step requires id');
      }
      const label = typeof step.label === 'string' ? step.label : stepId;
      const helpText = typeof step.helpText === 'string' ? step.helpText : '';
      const accept = typeof step.accept === 'string' ? step.accept : 'image/*,video/*';
      const maxBytes = Number(step.maxBytes) > 0 ? Number(step.maxBytes) : 0;

      const fileInput = tbrgCreateEl('input', { class: 'tbrg-input', type: 'file', accept }, []);

      const dialog = tbrgShowBlockingDialog({
        title: 'Automation Input',
        badge: stepId,
        label,
        helpText,
        bodyEl: fileInput,
        primaryText: 'Use file',
        timeoutMs
      });

      dialog.setConfirmHandler(async () => {
        const file = fileInput.files && fileInput.files[0] ? fileInput.files[0] : null;
        const dataUrl = await tbrgReadFileAsDataUrl(file, maxBytes);
        return {
          name: file ? file.name : '',
          type: file ? file.type : '',
          size: file ? file.size : 0,
          dataUrl
        };
      });

      const value = await dialog.promise;
      return { ok: true, value };
    }

    if (type === 'waitFor' && operator === 'exists') {
      const requireVisible = step.requireVisible === true;
      await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex, { requireVisible });
      return { ok: true, value: true };
    }

    if (type === 'dom' && operator === 'hover') {
      const requireVisible = step.requireVisible === true;
      const idx = Number.isFinite(Number(matchIndex)) ? Number(matchIndex) : 0;
      await tbrgWaitForSelector(step.selector, timeoutMs, idx, { requireVisible });
      const el = tbrgGetElement(step.selector, idx);
      try {
        el.dispatchEvent(
          new MouseEvent('mouseenter', { bubbles: true, cancelable: true, view: window })
        );
        el.dispatchEvent(
          new MouseEvent('mouseover', { bubbles: true, cancelable: true, view: window })
        );
      } catch (_error) {
        throw new Error('dom.hover could not dispatch pointer events on the target element.');
      }
      const hoverSettleMs = Number(step.hoverSettleMs) > 0 ? Number(step.hoverSettleMs) : 250;
      await new Promise((resolve) => setTimeout(resolve, hoverSettleMs));
      return { ok: true, value: true };
    }

    if (type === 'dom' && operator === 'click') {
      const requireVisible = step.requireVisible === true;
      const idx = Number.isFinite(Number(matchIndex)) ? Number(matchIndex) : 0;
      await tbrgWaitForSelector(step.selector, timeoutMs, idx, { requireVisible });
      const el = tbrgGetElement(step.selector, idx);
      try {
        el.scrollIntoView({ block: 'center', inline: 'nearest' });
      } catch (_error) {
        // Best effort before click.
      }
      const preClickDelayMs = Number(step.preClickDelayMs) > 0 ? Number(step.preClickDelayMs) : 0;
      if (preClickDelayMs > 0) {
        await new Promise((resolve) => setTimeout(resolve, preClickDelayMs));
      }
      const clickDispatch =
        typeof step.clickDispatch === 'string' ? step.clickDispatch.trim().toLowerCase() : 'native';
      if (clickDispatch === 'synthetic') {
        tbrgDispatchSyntheticPointerClick(el);
      } else if (clickDispatch === 'both') {
        tbrgDispatchSyntheticPointerClick(el);
        if (typeof el.click === 'function') {
          el.click();
        }
      } else {
        if (typeof el.click !== 'function') {
          throw new Error('dom.click target has no click() method.');
        }
        el.click();
      }
      const postClickDelayMs = Number(step.postClickDelayMs) > 0 ? Number(step.postClickDelayMs) : 500;
      await new Promise((resolve) => setTimeout(resolve, postClickDelayMs));
      return { ok: true, value: true };
    }

    if (type === 'dom' && operator === 'readText') {
      const requireVisible = step.requireVisible === true;
      await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex, { requireVisible });
      const el = tbrgGetElement(step.selector, matchIndex);
      const mode =
        typeof step.textMode === 'string' && step.textMode.trim() === 'textContent'
          ? 'textContent'
          : 'innerText';
      const raw = mode === 'textContent' ? el.textContent : el.innerText;
      const text = String(raw == null ? '' : raw).trim();
      return { ok: true, value: text };
    }

    if (type === 'image' && operator === 'capture') {
      const readySel = typeof step.readySelector === 'string' ? step.readySelector.trim() : '';
      const readyTimeout =
        Number(step.readyTimeoutMs) > 0 ? Number(step.readyTimeoutMs) : timeoutMs;
      const readyMatch = Object.prototype.hasOwnProperty.call(step, 'readyMatchIndex')
        ? step.readyMatchIndex
        : matchIndex;
      const readyRequireVisible = step.readyRequireVisible !== false;
      if (readySel) {
        await tbrgWaitForSelector(readySel, readyTimeout, readyMatch, {
          requireVisible: readyRequireVisible
        });
      } else {
        await tbrgWaitForSelector(step.selector, timeoutMs, matchIndex);
      }
      const imageDataUrl = await tbrgCaptureElement(
        step.selector,
        matchIndex,
        step.preCaptureDelayMs,
        typeof step.boundsSelector === 'string' ? step.boundsSelector : '',
        step.stitchOverflow,
        {
          dismissSelectors: step.dismissSelectors,
          dismissAttempts: step.dismissAttempts,
          postDismissDelayMs: step.postDismissDelayMs,
          captureScrollBlock: step.captureScrollBlock,
          captureEnsureVisible: step.captureEnsureVisible,
          captureSettleMs: step.captureSettleMs,
          readySelector: readySel,
          readyTimeoutMs: readyTimeout,
          readyMatchIndex: readyMatch,
          readyRequireVisible
        }
      );
      return { ok: true, value: imageDataUrl };
    }

    if (type === 'network' && operator === 'saveUrl') {
      const url = typeof step.url === 'string' ? step.url.trim() : '';
      const downloadBasename = typeof step.downloadBasename === 'string' ? step.downloadBasename.trim() : '';
      const downloadFileExtension =
        typeof step.downloadFileExtension === 'string' ? step.downloadFileExtension.trim() : '';
      const response = await new Promise((resolve) => {
        chrome.runtime.sendMessage(
          {
            type: 'TBRG_DOWNLOAD_URL',
            url,
            downloadBasename,
            downloadFileExtension
          },
          resolve
        );
      });
      if (!response || response.ok !== true) {
        throw new Error(response?.error || 'URL download failed.');
      }
      return {
        ok: true,
        value: {
          filename: response.filename,
          url
        }
      };
    }

    throw new Error(`Unsupported step route: ${type}.${operator}`);
  }

  function tbrgMaybeReportStepProgress(progressReporting, stepResults) {
    if (!progressReporting || typeof progressReporting !== 'object') {
      return;
    }
    const offset = Number(progressReporting.completedStepsOffset) || 0;
    const totalOverall = Number(progressReporting.totalStepsOverall) || 0;
    const okCount = stepResults.filter((entry) => entry.ok).length;
    const completedSteps = offset + okCount;
    const lastOk = stepResults.filter((entry) => entry.ok).slice(-1)[0];
    try {
      chrome.runtime.sendMessage({
        type: 'TBRG_JOB_STEP_PROGRESS',
        stage: 'running',
        templateId: progressReporting.templateId,
        progressTabId: progressReporting.progressTabId,
        completedTasks: Number(progressReporting.completedTasks) || 0,
        totalTasks: Number(progressReporting.totalTasks) || 0,
        completedSteps,
        totalSteps: totalOverall,
        lastCompletedStepId: lastOk?.id || null,
        currentTaskId: progressReporting.currentTaskId || null,
        currentTaskName: progressReporting.currentTaskName || null
      });
    } catch (_error) {
      // Background may be unavailable; progress is best-effort.
    }
  }

  async function tbrgExecuteTemplate(template, progressReporting) {
    const results = {};
    const stepResults = [];
    const taskResults = [];
    const stopOnFailure = template?.stopOnFailure !== false;
    const stepsToRun = Array.isArray(template.tasks)
      ? template.tasks.flatMap((task) =>
        (task.steps || []).map((step) => ({ ...step, __taskId: task.id || 'task' }))
      )
      : template.steps;

    try {
      for (const step of stepsToRun) {
        const startedAt = tbrgNow();
        try {
          const stepResult = await tbrgRunStep(step, results);
          const valueKey = typeof step.value === 'string' ? step.value.trim() : '';
          if (valueKey) {
            results[valueKey] = stepResult.value;
          }
          stepResults.push({
            id: step.id,
            value: valueKey || null,
            taskId: step.__taskId || null,
            durationMs: tbrgNow() - startedAt,
            ...stepResult
          });
        } catch (error) {
          stepResults.push({
            id: step.id,
            value: typeof step.value === 'string' ? step.value.trim() || null : null,
            taskId: step.__taskId || null,
            durationMs: tbrgNow() - startedAt,
            ok: false,
            error: error.message || String(error)
          });
          if (stopOnFailure) {
            tbrgMaybeReportStepProgress(progressReporting, stepResults);
            break;
          }
        }
        tbrgMaybeReportStepProgress(progressReporting, stepResults);
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
    } finally {
      tbrgAutomationRemoveDomHighlight();
      tbrgAutomationSetStepContext(null);
    }
  }

  self.__TBRG_MESSAGE_HANDLER__ = (message, _sender, sendResponse) => {
    if (message?.type !== 'TBRG_EXECUTE_TEMPLATE') {
      return false;
    }

    tbrgExecuteTemplate(message.template, message.progressReporting)
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

})();
