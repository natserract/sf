(() => {
  const ROOT_ID = '__tbrg_embed_overlay_root__';
  const STYLE_ID = '__tbrg_embed_overlay_style__';

  function tbrgEmbedRemove() {
    document.getElementById(ROOT_ID)?.remove();
    document.getElementById(STYLE_ID)?.remove();
  }

  self.__TBRG_EMBED_HIDE__ = function tbrgEmbedHide() {
    tbrgEmbedRemove();
  };

  self.__TBRG_EMBED_SHOW__ = function tbrgEmbedShow(payload) {
    const url = typeof payload?.url === 'string' ? payload.url.trim() : '';
    const title = typeof payload?.title === 'string' ? payload.title.trim() : 'Embedded tool';
    if (!url || !url.startsWith('https://')) {
      return;
    }

    tbrgEmbedRemove();

    const style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = `
      #${ROOT_ID} {
        position: fixed;
        inset: 0;
        z-index: 2147483646;
        display: flex;
        flex-direction: column;
        background: #0f172a;
        font: 14px/1.4 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
      }
      #${ROOT_ID} .tbrg-embed-header {
        flex: 0 0 auto;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        padding: 10px 14px;
        background: #1e293b;
        color: #f1f5f9;
        border-bottom: 1px solid rgba(148, 163, 184, 0.35);
        min-height: 48px;
        box-sizing: border-box;
      }
      #${ROOT_ID} .tbrg-embed-title {
        font-weight: 600;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      #${ROOT_ID} .tbrg-embed-close {
        flex: 0 0 auto;
        padding: 8px 14px;
        border-radius: 8px;
        border: 1px solid rgba(148, 163, 184, 0.5);
        background: #334155;
        color: #f8fafc;
        font: inherit;
        font-weight: 600;
        cursor: pointer;
      }
      #${ROOT_ID} .tbrg-embed-close:hover {
        background: #475569;
      }
      #${ROOT_ID} .tbrg-embed-frame-wrap {
        flex: 1 1 auto;
        min-height: 0;
        background: #020617;
      }
      #${ROOT_ID} iframe {
        display: block;
        width: 100%;
        height: 100%;
        border: 0;
      }
    `;
    document.documentElement.appendChild(style);

    const root = document.createElement('div');
    root.id = ROOT_ID;
    root.setAttribute('data-tbrg-embed-overlay', '');

    const header = document.createElement('div');
    header.className = 'tbrg-embed-header';

    const titleEl = document.createElement('div');
    titleEl.className = 'tbrg-embed-title';
    titleEl.textContent = title;

    const closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'tbrg-embed-close';
    closeBtn.textContent = 'Close';
    closeBtn.addEventListener('click', tbrgEmbedRemove);

    header.appendChild(titleEl);
    header.appendChild(closeBtn);

    const wrap = document.createElement('div');
    wrap.className = 'tbrg-embed-frame-wrap';

    const iframe = document.createElement('iframe');
    iframe.setAttribute('referrerpolicy', 'strict-origin-when-cross-origin');
    iframe.src = url;

    wrap.appendChild(iframe);
    root.appendChild(header);
    root.appendChild(wrap);
    document.documentElement.appendChild(root);
  };
})();
