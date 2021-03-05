'use strict';
/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
class FixedHeaderController {
  constructor(el, fixedEl) {
    this.el = el;
    this.fixedEl = fixedEl;
    this.intersectionObserverCallback = entries => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          this.fixedEl.classList.remove('UnitFixedHeader--visible');
        } else {
          this.fixedEl.classList.add('UnitFixedHeader--visible');
        }
      });
    };
    if (!el || !fixedEl) {
      throw new Error('Must provide sentinel and fixed elements to constructor.');
    }
    this.intersectionObserver = new IntersectionObserver(this.intersectionObserverCallback, {
      threshold: 1.0,
    });
    this.intersectionObserver.observe(this.el);
    if (window.getComputedStyle(document.body)['-webkit-overflow-scrolling'] !== undefined) {
      [document.documentElement, document.body].forEach(el => {
        el.style.overflow = 'auto';
      });
    }
  }
}
const fixedHeaderSentinel = document.querySelector('.js-fixedHeaderSentinel');
const fixedHeader = document.querySelector('.js-fixedHeader');
if (fixedHeaderSentinel && fixedHeader) {
  new FixedHeaderController(fixedHeaderSentinel, fixedHeader);
}
const overflowSelect = document.querySelector('.js-overflowSelect');
if (overflowSelect) {
  overflowSelect.addEventListener('change', e => {
    window.location.href = e.target.value;
  });
}
//# sourceMappingURL=unit_fixed_header.js.map
