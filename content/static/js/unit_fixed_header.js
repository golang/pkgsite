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

const overflowSelect = document.querySelector('.js-overflowSelect');
if (overflowSelect) {
  overflowSelect.addEventListener('change', e => {
    window.location.href = e.target.value;
  });
}
