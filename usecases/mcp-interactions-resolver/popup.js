/**
 * SF Proxy – popup UI logic
 * Shows status, connect/disconnect, and pings the default PAC URL before connect.
 */

const statusEl = document.getElementById('status');
const myIpEl = document.getElementById('myIp');
const btnConnect = document.getElementById('btnConnect');
const btnDisconnect = document.getElementById('btnDisconnect');

function setMyIp(myIp) {
  if (myIp) {
    myIpEl.textContent = 'Your IP: ' + myIp;
  } else {
    myIpEl.textContent = 'Your IP: —';
  }
}

function setStatus(connected, errorMessage) {
  statusEl.classList.remove('connected', 'disconnected', 'error');
  if (errorMessage) {
    statusEl.textContent = errorMessage;
    statusEl.classList.add('error');
  } else if (connected) {
    statusEl.textContent = 'Connected';
    statusEl.classList.add('connected');
  } else {
    statusEl.textContent = 'Disconnected';
    statusEl.classList.add('disconnected');
  }
  btnConnect.disabled = connected;
  btnDisconnect.disabled = !connected;
}

function refreshStatus() {
  chrome.runtime.sendMessage({ action: 'getStatus' }, (response) => {
    if (chrome.runtime.lastError) {
      setStatus(false, 'Extension error');
      setMyIp(null);
      return;
    }
    setStatus(response.connected);
    setMyIp(response.myIp);
  });
}

refreshStatus();

btnConnect.addEventListener('click', () => {
  chrome.runtime.sendMessage({ action: 'getStatus' }, (response) => {
    if (chrome.runtime.lastError) {
      setStatus(false, 'Extension error');
      return;
    }
    setMyIp(response.myIp);
    const effectiveUrl = response.pacUrl || '';
    if (!effectiveUrl) {
      setStatus(false, 'No default configuration found');
      return;
    }
    if (!effectiveUrl.startsWith('http://') && !effectiveUrl.startsWith('https://')) {
      setStatus(false, 'Default configuration is invalid');
      return;
    }
    if (!response.myIp) {
      setStatus(false, 'Unable to get IP address');
      return;
    }
    setStatus(response.connected, 'Checking PAC URL…');
    btnConnect.disabled = true;
    chrome.runtime.sendMessage({ action: 'pingPac', pacUrl: effectiveUrl }, (pingRes) => {
      if (chrome.runtime.lastError) {
        setStatus(false, 'Ping failed');
        btnConnect.disabled = false;
        return;
      }
      if (!pingRes || !pingRes.reachable) {
        setStatus(false, pingRes && pingRes.error ? pingRes.error : 'PAC URL unreachable');
        btnConnect.disabled = false;
        return;
      }
      chrome.runtime.sendMessage({ action: 'connect' }, (res) => {
        if (chrome.runtime.lastError) {
          setStatus(false, 'Failed to connect');
        } else if (res && res.success) {
          setStatus(true);
          if (res.myIp) setMyIp(res.myIp);
        } else {
          setStatus(false, (res && res.error) || 'Failed to connect');
        }
        btnConnect.disabled = false;
      });
    });
  });
});

btnDisconnect.addEventListener('click', () => {
  chrome.runtime.sendMessage({ action: 'disconnect' }, (res) => {
    if (chrome.runtime.lastError) {
      setStatus(false, 'Failed to disconnect');
      return;
    }
    if (res && res.success) {
      setStatus(false);
      refreshStatus();
    }
  });
});
