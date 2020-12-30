/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * This class decorates an element to copy arbitrary data attached via a data-
 * attribute to the clipboard.
 */
export class CopyToClipboardController {
  private _el: HTMLButtonElement;
  /**
   * The data to be copied to the clipboard.
   */
  private _data: string;

  /**
   * @param el The element that will trigger copying text to the clipboard. The text is
   * expected to be within its data-to-copy attribute.
   */
  constructor(el: HTMLButtonElement) {
    this._el = el;
    this._data = el.dataset['toCopy'] ?? '';
    el.addEventListener('click', e => this.handleCopyClick(e));
  }

  /**
   * Handles when the primary element is clicked.
   */
  handleCopyClick(e: MouseEvent): void {
    e.preventDefault();
    const TOOLTIP_SHOW_DURATION_MS = 1000;

    // This API is not available on iOS.
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

  /**
   * Shows the given text in a tooltip for a specified amount of time, in milliseconds.
   */
  showTooltipText(text: string, durationMs: number): void {
    this._el.setAttribute('data-tooltip', text);
    setTimeout(() => this._el.setAttribute('data-tooltip', ''), durationMs);
  }
}
