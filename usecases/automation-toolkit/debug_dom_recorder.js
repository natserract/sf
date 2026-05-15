(() => {
  if (self.__TBRG_DEBUG_DOM_RECORDER__) {
    return;
  }
  self.__TBRG_DEBUG_DOM_RECORDER__ = true;

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

  function tbrgPickerActive() {
    return Boolean(self.__TBRG_DOM_PICKER_STATE__);
  }

  function tbrgIsSensitiveField(el) {
    if (el instanceof HTMLInputElement) {
      const t = (el.type || '').toLowerCase();
      if (t === 'password' || t === 'hidden') {
        return true;
      }
      const ac = (el.getAttribute('autocomplete') || '').toLowerCase();
      if (ac.includes('cc-') || ac === 'one-time-code') {
        return true;
      }
    }
    return false;
  }

  function tbrgSendActivity(event) {
    try {
      chrome.runtime.sendMessage({
        type: 'TBRG_DEBUG_ACTIVITY',
        event
      });
    } catch (_e) {
      // Extension context invalidated, etc.
    }
  }

  function tbrgBuildClickEvent(target) {
    const selector = tbrgBestSelector(target);
    return {
      type: 'click',
      ts: new Date().toISOString(),
      pageUrl: location.href,
      frameUrl: location.href,
      selector,
      fallbackSelectors: [tbrgBuildSelectorChain(target)].filter(Boolean),
      matchIndex: tbrgMatchIndexForSelector(selector, target),
      tagName: target.tagName.toLowerCase(),
      textSample: tbrgTrimmedText(target)
    };
  }

  function onClickCapture(event) {
    if (tbrgPickerActive()) {
      return;
    }
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    if (target.hasAttribute(PICKER_ATTR) || target.closest(`[${PICKER_ATTR}]`)) {
      return;
    }
    tbrgSendActivity(tbrgBuildClickEvent(target));
  }

  let inputDebounceTimer = null;
  let lastInputElement = null;

  function flushDebouncedInput() {
    inputDebounceTimer = null;
    const el = lastInputElement;
    lastInputElement = null;
    if (!(el instanceof Element)) {
      return;
    }
    const selTyping = tbrgBestSelector(el);
    if (tbrgIsSensitiveField(el)) {
      tbrgSendActivity({
        type: 'input',
        inputPhase: 'typing',
        ts: new Date().toISOString(),
        pageUrl: location.href,
        frameUrl: location.href,
        selector: selTyping,
        matchIndex: tbrgMatchIndexForSelector(selTyping, el),
        tagName: el.tagName.toLowerCase(),
        inputType: el instanceof HTMLInputElement ? el.type || 'text' : el.tagName.toLowerCase(),
        sensitive: true
      });
      return;
    }
    let valueLength = 0;
    if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
      valueLength = (el.value || '').length;
    }
    tbrgSendActivity({
      type: 'input',
      inputPhase: 'typing',
      ts: new Date().toISOString(),
      pageUrl: location.href,
      frameUrl: location.href,
      selector: selTyping,
      matchIndex: tbrgMatchIndexForSelector(selTyping, el),
      tagName: el.tagName.toLowerCase(),
      inputType: el instanceof HTMLInputElement ? el.type || 'text' : el.tagName.toLowerCase(),
      valueLength
    });
  }

  function onInputCapture(event) {
    if (tbrgPickerActive()) {
      return;
    }
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    if (!(target instanceof HTMLInputElement) && !(target instanceof HTMLTextAreaElement) && !target.isContentEditable) {
      return;
    }
    lastInputElement = target;
    if (inputDebounceTimer) {
      clearTimeout(inputDebounceTimer);
    }
    inputDebounceTimer = setTimeout(flushDebouncedInput, 450);
  }

  function onChangeCapture(event) {
    if (tbrgPickerActive()) {
      return;
    }
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    if (
      !(target instanceof HTMLInputElement) &&
      !(target instanceof HTMLTextAreaElement) &&
      !(target instanceof HTMLSelectElement)
    ) {
      return;
    }

    const selCommit = tbrgBestSelector(target);
    if (tbrgIsSensitiveField(target)) {
      tbrgSendActivity({
        type: 'input',
        inputPhase: 'commit',
        ts: new Date().toISOString(),
        pageUrl: location.href,
        frameUrl: location.href,
        selector: selCommit,
        matchIndex: tbrgMatchIndexForSelector(selCommit, target),
        tagName: target.tagName.toLowerCase(),
        inputType: target instanceof HTMLInputElement ? target.type || 'text' : target.tagName.toLowerCase(),
        sensitive: true
      });
      return;
    }

    let valueLength = 0;
    let checked = undefined;
    if (target instanceof HTMLInputElement) {
      const t = (target.type || '').toLowerCase();
      if (t === 'checkbox' || t === 'radio') {
        checked = Boolean(target.checked);
      } else if (t === 'file') {
        valueLength = target.files ? target.files.length : 0;
      } else {
        valueLength = (target.value || '').length;
      }
    } else if (target instanceof HTMLSelectElement) {
      valueLength = (target.value || '').length;
    } else {
      valueLength = (target.value || '').length;
    }

    tbrgSendActivity({
      type: 'input',
      inputPhase: 'commit',
      ts: new Date().toISOString(),
      pageUrl: location.href,
      frameUrl: location.href,
      selector: selCommit,
      matchIndex: tbrgMatchIndexForSelector(selCommit, target),
      tagName: target.tagName.toLowerCase(),
      inputType: target instanceof HTMLInputElement ? target.type || 'text' : target.tagName.toLowerCase(),
      valueLength,
      ...(typeof checked === 'boolean' ? { checked } : {})
    });
  }

  document.addEventListener('click', onClickCapture, true);
  document.addEventListener('input', onInputCapture, true);
  document.addEventListener('change', onChangeCapture, true);
})();
