/**
 * SF Proxy – options page
 * Save and load PAC URL in storage.
 */

const STORAGE_KEY_PAC_URL = 'pacUrl';

const pacUrlInput = document.getElementById('pacUrl');
const btnSave = document.getElementById('btnSave');
const messageEl = document.getElementById('message');

function showMessage(text, isError) {
  messageEl.textContent = text;
  messageEl.className = 'message ' + (isError ? 'error' : 'success');
  messageEl.style.display = 'block';
  setTimeout(() => {
    messageEl.style.display = 'none';
  }, 3000);
}

chrome.storage.local.get([STORAGE_KEY_PAC_URL], (result) => {
  if (result[STORAGE_KEY_PAC_URL]) {
    pacUrlInput.value = result[STORAGE_KEY_PAC_URL];
  }
});

btnSave.addEventListener('click', () => {
  const value = (pacUrlInput.value || '').trim();
  if (!value) {
    showMessage('Please enter a PAC URL.', true);
    return;
  }
  if (!value.startsWith('http://') && !value.startsWith('https://')) {
    showMessage('PAC URL must start with http:// or https://', true);
    return;
  }
  chrome.storage.local.set({ [STORAGE_KEY_PAC_URL]: value }, () => {
    showMessage('PAC URL saved.');
  });
});
