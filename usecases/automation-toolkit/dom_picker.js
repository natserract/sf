(() => {
  if (self.__TBRG_DOM_PICKER_HANDLER__) {
    chrome.runtime.onMessage.removeListener(self.__TBRG_DOM_PICKER_HANDLER__);
  }

  const PICKER_ATTR = 'data-tbrg-picker-overlay';

  function tbrgCssEscape(value) {
    if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') {
      return CSS.escape(value);
    }
    return String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  }

  function tbrgBuildSelectorChain(element) {
    const chain = [];
    let node = element;

    while (node && node.nodeType === Node.ELEMENT_NODE && chain.length < 5) {
      const tag = node.tagName.toLowerCase();
      const id = node.getAttribute('id');
      if (id) {
        chain.unshift(`#${tbrgCssEscape(id)}`);
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
        chain.unshift(`${tag}.${tbrgCssEscape(node.classList[0])}`);
      } else {
        chain.unshift(tag);
      }

      node = node.parentElement;
    }

    return chain.join(' > ');
  }

  function tbrgBestSelector(element) {
    const id = element.getAttribute('id');
    if (id) {
      return `#${tbrgCssEscape(id)}`;
    }

    const dataAttrs = ['data-testid', 'data-test-id', 'data-qa', 'data-id', 'aria-label'];
    for (const attr of dataAttrs) {
      const value = element.getAttribute(attr);
      if (value) {
        return `[${attr}="${value}"]`;
      }
    }

    const tag = element.tagName.toLowerCase();
    const classNames = Array.from(element.classList).filter((name) => !/^ng-tns-|^ng-star/.test(name));
    if (classNames.length > 0) {
      const selector = `${tag}.${tbrgCssEscape(classNames[0])}`;
      if (document.querySelectorAll(selector).length <= 3) {
        return selector;
      }
    }

    return tbrgBuildSelectorChain(element) || tag;
  }

  function tbrgMatchIndexForSelector(selector, element) {
    try {
      const list = Array.from(document.querySelectorAll(selector));
      return Math.max(0, list.indexOf(element));
    } catch (_e) {
      return 0;
    }
  }

  function tbrgTrimmedText(element) {
    const text = (element.textContent || '').replace(/\s+/g, ' ').trim();
    return text.slice(0, 140);
  }

  function tbrgCreateOverlay() {
    const overlay = document.createElement('div');
    overlay.setAttribute(PICKER_ATTR, '1');
    Object.assign(overlay.style, {
      position: 'fixed',
      pointerEvents: 'none',
      border: '2px solid #2563eb',
      background: 'rgba(37, 99, 235, 0.12)',
      zIndex: '2147483647',
      borderRadius: '4px',
      boxSizing: 'border-box'
    });
    document.documentElement.appendChild(overlay);
    return overlay;
  }

  function tbrgStartPicker() {
    if (self.__TBRG_DOM_PICKER_STATE__) {
      return { ok: true };
    }

    const overlay = tbrgCreateOverlay();
    const state = {
      overlay,
      active: true,
      cleanup: null
    };
    self.__TBRG_DOM_PICKER_STATE__ = state;

    function cleanup() {
      if (!state.active) {
        return;
      }
      state.active = false;
      document.removeEventListener('mousemove', onMouseMove, true);
      document.removeEventListener('click', onClick, true);
      document.removeEventListener('keydown', onKeyDown, true);
      overlay.remove();
      self.__TBRG_DOM_PICKER_STATE__ = null;
    }
    state.cleanup = cleanup;

    function onMouseMove(event) {
      const target = event.target;
      if (!(target instanceof Element) || target === overlay || target.hasAttribute(PICKER_ATTR)) {
        return;
      }
      const rect = target.getBoundingClientRect();
      Object.assign(overlay.style, {
        left: `${rect.left}px`,
        top: `${rect.top}px`,
        width: `${rect.width}px`,
        height: `${rect.height}px`,
        display: 'block'
      });
    }

    function onClick(event) {
      const target = event.target;
      if (!(target instanceof Element) || target.hasAttribute(PICKER_ATTR)) {
        return;
      }

      event.preventDefault();
      event.stopPropagation();

      const selector = tbrgBestSelector(target);
      const payload = {
        selector,
        fallbackSelectors: [tbrgBuildSelectorChain(target)].filter(Boolean),
        matchIndex: tbrgMatchIndexForSelector(selector, target),
        tagName: target.tagName.toLowerCase(),
        textSample: tbrgTrimmedText(target),
        frameUrl: location.href
      };

      cleanup();
      chrome.runtime.sendMessage({
        type: 'TBRG_DOM_PICKER_RESULT',
        pick: payload
      });
    }

    function onKeyDown(event) {
      if (event.key !== 'Escape') {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      cleanup();
      chrome.runtime.sendMessage({
        type: 'TBRG_DOM_PICKER_CANCELLED',
        reason: 'Picker cancelled by user.'
      });
    }

    document.addEventListener('mousemove', onMouseMove, true);
    document.addEventListener('click', onClick, true);
    document.addEventListener('keydown', onKeyDown, true);

    return { ok: true };
  }

  function tbrgStopPicker() {
    if (typeof self.__TBRG_DOM_PICKER_STATE__?.cleanup === 'function') {
      self.__TBRG_DOM_PICKER_STATE__.cleanup();
    }
  }

  self.__TBRG_DOM_PICKER_START__ = tbrgStartPicker;
  self.__TBRG_DOM_PICKER_STOP__ = tbrgStopPicker;

  self.__TBRG_DOM_PICKER_HANDLER__ = (message, _sender, sendResponse) => {
    if (message?.type === 'TBRG_DOM_PICKER_START') {
      try {
        sendResponse(tbrgStartPicker());
      } catch (error) {
        sendResponse({ ok: false, error: error.message || String(error) });
      }
      return true;
    }

    if (message?.type === 'TBRG_DOM_PICKER_STOP') {
      tbrgStopPicker();
      sendResponse({ ok: true });
      return true;
    }

    return false;
  };

  chrome.runtime.onMessage.addListener(self.__TBRG_DOM_PICKER_HANDLER__);
})();
