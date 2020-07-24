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
  }

  /**
   * @param {!Array<IntersectionObserverEntry>} entries
   * @param {!IntersectionObserver} observer
   * @private
   */
  intersectionObserverCallback(entries, observer) {
    entries.forEach(entry => {
      this._fixedEl.setAttribute('aria-hidden', entry.isIntersecting);
    });
  }
}
