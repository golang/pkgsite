/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Shows a fixed element when a separate element begins to go out of view.
 */
class FixedHeaderController {
  /**
   * @param {Element} el
   * @param {Element} fixedEl
   */
  constructor(el, fixedEl) {
    if (!el || !fixedEl) {
      throw new Error('Must provide sentinel and fixed elements to constructor.');
    }

    /**
     * The element to observe to determine whether to show the fixed element.
     * @type {!Element}
     * @private
     */
    this._el = /** @type {!Element} */ (el);

    /**
     * The element to show when the other begins to go out of view.
     * @type {!Element}
     * @private
     */
    this._fixedEl = /** @type {!Element} */ (fixedEl);

    /**
     * @type {!IntersectionObserver}
     * @private
     */
    this._intersectionObserver = new IntersectionObserver(
      (entries, observer) => this.intersectionObserverCallback(entries, observer),
      {
        threshold: 1.0,
      }
    );
    this._intersectionObserver.observe(this._el);

    // Fixed positioning on Safari iOS is very broken, and without this hack,
    // focusing on the overflow menu will cause all content to scroll.
    // The -webkit-overflow-scroll CSS property is only available on mobile
    // Safari, so check for it and set the appropriate style to fix this.
    if (window.getComputedStyle(document.body)['-webkit-overflow-scrolling'] !== undefined) {
      [document.documentElement, document.body].forEach(el => {
        el.style.overflow = 'auto';
      });
    }
  }

  /**
   * @param {!Array<IntersectionObserverEntry>} entries
   * @param {!IntersectionObserver} observer
   * @private
   */
  intersectionObserverCallback(entries, observer) {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        this._fixedEl.classList.remove('UnitFixedHeader--visible');
      } else {
        this._fixedEl.classList.add('UnitFixedHeader--visible');
      }
    });
  }
}

new FixedHeaderController(
  document.querySelector('.js-fixedHeaderSentinel'),
  document.querySelector('.js-fixedHeader')
);

/**
 * This class decorates an element to copy arbitrary data attached via a data-
 * attribute to the clipboard.
 */
class CopyToClipboardController {
  /**
   * The element that will trigger copying text to the clipboard. The text is
   * expected to be within its data-to-copy attribute.
   * @param {!Element} el
   */
  constructor(el) {
    /**
     * @type {!Element}
     * @private
     */
    this._el = el;

    /**
     * The data to be copied to the clipboard.
     * @type {string}
     * @private
     */
    this._data = el.dataset['toCopy'];

    el.addEventListener('click', e => this.handleCopyClick(/** @type {!Event} */ (e)));
  }

  /**
   * Handles when the primary element is clicked.
   * @param {!Event} e
   * @private
   */
  handleCopyClick(e) {
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
   * @param {string} text
   * @param {number} durationMs
   * @private
   */
  showTooltipText(text, durationMs) {
    this._el.setAttribute('data-tooltip', text);
    setTimeout(() => this._el.setAttribute('data-tooltip', ''), durationMs);
  }
}

document.querySelectorAll('.js-copyToClipboard').forEach(el => {
  new CopyToClipboardController(el);
});

const overflowSelect = document.querySelector('.js-overflowSelect');
if (overflowSelect) {
  overflowSelect.addEventListener('change', e => {
    window.location.href = e.target.value;
  });
}
