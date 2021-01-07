/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class CopyToClipboardController {
    constructor(el) {
        this._el = el;
        this._data = el.dataset['toCopy'] ?? '';
        el.addEventListener('click', e => this.handleCopyClick(e));
    }
    handleCopyClick(e) {
        e.preventDefault();
        const TOOLTIP_SHOW_DURATION_MS = 1000;
        if (!navigator.clipboard) {
            this.showTooltipText('Unable to copy', TOOLTIP_SHOW_DURATION_MS);
            return;
        }
        navigator.clipboard
            .writeText(this._data)
            .then(() => {
            this.showTooltipText('Copied!', TOOLTIP_SHOW_DURATION_MS);
        })
            .catch(() => {
            this.showTooltipText('Unable to copy', TOOLTIP_SHOW_DURATION_MS);
        });
    }
    showTooltipText(text, durationMs) {
        this._el.setAttribute('data-tooltip', text);
        setTimeout(() => this._el.setAttribute('data-tooltip', ''), durationMs);
    }
}
//# sourceMappingURL=clipboard.js.map