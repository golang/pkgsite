/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Class names used by OverflowingTabListController.
 * @private @enum {string}
 */
const OverflowingTabListClassName = {
  IS_OVERFLOWING: 'is-overflowing',
};

/**
 * Allows a list of tabs to overflow into a “more” menu depending on the size
 * of the container.
 */
class OverflowingTabListController {
  /**
   * @param {Element} el
   */
  constructor(el) {
    if (!el) {
      throw new Error('Must provide an element.');
    }

    /**
     * @type {!Element}
     * @private
     */
    this._el = el;

    const selectEl = this._el.querySelector('select');
    if (!selectEl) {
      throw new Error('Element must contain a <select> element.');
    }

    /**
     * @type {!Element}
     * @private
     */
    this._selectEl = /** @type {!Element} */ (selectEl);

    /**
     * An array of tab elements’ rightmost points.
     * @type {!Array<number>}
     * @private
     */
    this._tabRects = [];

    if (!window.ResizeObserver) {
      this._el.style.overflowX = 'scroll';
      return;
    }

    let initialLeft = 0;
    this._tabRects = Array.from(this._el.querySelectorAll(`[role='tab']`)).map((el, i) => {
      if (i === 0) {
        initialLeft = el.offsetLeft;
      }
      return el.offsetLeft + el.offsetWidth - initialLeft;
    });

    const resizeObserver = new ResizeObserver(entries => this.resizeObserverCallback(entries));
    resizeObserver.observe(this._el);

    this._selectEl.addEventListener('change', e =>
      this.handleOverflowSelectChange(/** @type {!Event} */ (e))
    );
  }

  /**
   * @param {!Array<ResizeObserverEntry>} entries
   * @private
   */
  resizeObserverCallback(entries) {
    entries.forEach(entry => {
      const containerRect = entry.target.getBoundingClientRect();
      const hiddenEls = [];
      this._el.querySelectorAll(`[role='tab']`).forEach((el, i) => {
        const rect = this._tabRects[i];
        let overflowPoint = containerRect.width;
        const OVERFLOW_MENU_ELEMENT_ADJUSTMENT_PX = 40;
        if (this._el.classList.contains(OverflowingTabListClassName.IS_OVERFLOWING)) {
          overflowPoint -= OVERFLOW_MENU_ELEMENT_ADJUSTMENT_PX;
        }
        const hideEl = rect > overflowPoint;
        el.setAttribute('aria-hidden', hideEl);
        if (hideEl) {
          hiddenEls.push(i);
        }
      });
      this.setOverflowMenuHidden(hiddenEls.length === 0);
      this._selectEl.querySelectorAll('option').forEach((el, i) => {
        el.disabled = !hiddenEls.includes(i) || el.getAttribute('data-always-disabled') === 'true';
      });
    });
  }

  /**
   * @param {boolean} hidden
   * @private
   */
  setOverflowMenuHidden(hidden) {
    this._el.classList.toggle(OverflowingTabListClassName.IS_OVERFLOWING, !hidden);
  }

  /**
   * Handles when the overflow menu select element changes.
   * @param {!Event} e
   */
  handleOverflowSelectChange(e) {
    window.location.href = e.target.value;
  }
}
