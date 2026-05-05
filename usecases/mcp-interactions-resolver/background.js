/**
 * SF Proxy – background service worker
 * Sets or clears Chrome proxy using PAC URL; restores on startup if connected.
 */

const STORAGE_KEY_CONNECTED = 'connected';
const STORAGE_KEY_LAST_PAC_URL = 'lastPacUrl';

/** Base PAC URL; user's public IP is appended as ?myip= */
const PAC_BASE_URL = 'https://freepac.net/fapi/pac/?myip=';

/** In-memory cache for fetched public IP */
let cachedMyIp = null;

const FREEPAC_AUTH_URL = 'https://freepac.net/fapi/auth/';
const IPIFY_URL = 'https://api.ipify.org?format=json';

/**
 * Fetch IP from freepac.net auth API (token.name).
 * @returns {Promise<string|null>}
 */
function fetchMyIpFromFreepac() {
  return fetch(FREEPAC_AUTH_URL, {
    method: 'POST',
    headers: {
      'Accept': 'application/json, text/javascript, */*; q=0.01',
      'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
      'Origin': 'https://freepac.net',
      'Referer': 'https://freepac.net/'
    },
    body: 'server=',
    cache: 'no-store'
  })
    .then((res) => res.json())
    .then((data) => {
      const name = data && data.token && data.token.name;
      return typeof name === 'string' ? name.trim() : null;
    })
    .catch(() => null);
}

/**
 * Fetch IP from ipify (fallback).
 * @returns {Promise<string|null>}
 */
function fetchMyIpFromIpify() {
  return fetch(IPIFY_URL, { method: 'GET', cache: 'no-store' })
    .then((res) => res.json())
    .then((data) => (typeof data.ip === 'string' ? data.ip.trim() : null))
    .catch(() => null);
}

/**
 * Fetch current public IP (cached in memory).
 * Tries freepac.net auth first, then ipify.
 * @returns {Promise<string|null>} Resolved with IP string or null on failure
 */
function fetchMyIp() {
  if (cachedMyIp) return Promise.resolve(cachedMyIp);
  return fetchMyIpFromFreepac()
    .then((ip) => (ip ? ip : fetchMyIpFromIpify()))
    .then((ip) => {
      if (ip) cachedMyIp = ip;
      return ip;
    });
}

/**
 * Build full PAC URL with current IP. Returns base URL only if IP unavailable.
 * @param {string|null} myIp
 * @returns {string}
 */
function buildPacUrl(myIp) {
  if (myIp) return PAC_BASE_URL + encodeURIComponent(myIp);
  return PAC_BASE_URL;
}

/**
 * Apply proxy to use the given PAC URL (scope: regular = Chrome only).
 * @param {string} pacUrl - Full URL to the PAC file
 * @param {() => void} [callback] - Called after proxy is set
 */
function applyProxy(pacUrl, callback) {
  if (!pacUrl || typeof pacUrl !== 'string' || !pacUrl.startsWith('http')) {
    if (callback) callback();
    return;
  }
  const config = {
    mode: 'pac_script',
    pacScript: { url: pacUrl }
  };
  chrome.proxy.settings.set(
    { value: config, scope: 'regular' },
    () => { if (callback) callback(); }
  );
}

/**
 * Clear proxy (direct connection).
 * @param {() => void} [callback] - Called after proxy is cleared
 */
function clearProxy(callback) {
  chrome.proxy.settings.set(
    { value: { mode: 'direct' }, scope: 'regular' },
    () => { if (callback) callback(); }
  );
}

/**
 * Restore proxy on startup if we were previously connected.
 */
function restoreProxyIfConnected() {
  chrome.storage.local.get([STORAGE_KEY_CONNECTED, STORAGE_KEY_LAST_PAC_URL], (result) => {
    if (result[STORAGE_KEY_CONNECTED] && result[STORAGE_KEY_LAST_PAC_URL]) {
      const pacUrl = result[STORAGE_KEY_LAST_PAC_URL];
      if (typeof pacUrl === 'string' && pacUrl.startsWith('http')) applyProxy(pacUrl);
    }
  });
}

// Restore proxy when extension loads (e.g. after browser restart)
restoreProxyIfConnected();

// Handle messages from popup: get status, connect, disconnect
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message.action === 'getStatus') {
    fetchMyIp().then((myIp) => {
      chrome.storage.local.get([STORAGE_KEY_CONNECTED, STORAGE_KEY_LAST_PAC_URL], (result) => {
        const connected = !!result[STORAGE_KEY_CONNECTED];
        const pacUrl = connected && result[STORAGE_KEY_LAST_PAC_URL]
          ? result[STORAGE_KEY_LAST_PAC_URL]
          : buildPacUrl(myIp);
        sendResponse({
          connected,
          myIp: myIp || null,
          pacUrl
        });
      });
    });
    return true; // async response
  }

  if (message.action === 'connect') {
    fetchMyIp().then((myIp) => {
      const pacUrl = buildPacUrl(myIp);
      if (!pacUrl.startsWith('http')) {
        sendResponse({ success: false, error: 'Invalid PAC URL' });
        return;
      }
      if (!myIp) {
        sendResponse({ success: false, error: 'Could not get your IP address' });
        return;
      }
      chrome.storage.local.set({
        [STORAGE_KEY_CONNECTED]: true,
        [STORAGE_KEY_LAST_PAC_URL]: pacUrl
      }, () => {
        applyProxy(pacUrl, () => {
          sendResponse({ success: true, myIp });
        });
      });
    });
    return true;
  }

  if (message.action === 'disconnect') {
    chrome.storage.local.set({ [STORAGE_KEY_CONNECTED]: false }, () => {
      clearProxy(() => {
        sendResponse({ success: true });
      });
    });
    return true;
  }

  if (message.action === 'pingPac') {
    const pacUrl = (message.pacUrl || '').trim();
    if (!pacUrl || (!pacUrl.startsWith('http://') && !pacUrl.startsWith('https://'))) {
      sendResponse({ reachable: false, error: 'Invalid PAC URL' });
      return false;
    }
    const timeoutMs = 5000;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeoutMs);
    fetch(pacUrl, { method: 'GET', cache: 'no-store', signal: controller.signal })
      .then((response) => {
        clearTimeout(timeoutId);
        sendResponse({ reachable: response.ok, error: response.ok ? undefined : 'PAC URL returned ' + response.status });
      })
      .catch((err) => {
        clearTimeout(timeoutId);
        sendResponse({ reachable: false, error: err.message || 'PAC URL unreachable' });
      });
    return true;
  }

  sendResponse({ success: false, error: 'Unknown action' });
  return false;
});
