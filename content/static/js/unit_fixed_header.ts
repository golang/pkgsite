/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Shows a fixed element when a separate element begins to go out of view.
 */
class FixedHeaderController {
  private intersectionObserver: IntersectionObserver;

  /**
   * @param el The element to observe to determine whether to show the fixed element.
   * @param fixedEl The element to show when the other begins to go out of view.
   */
  constructor(private el: Element, private fixedEl: Element) {
    if (!el || !fixedEl) {
      throw new Error('Must provide sentinel and fixed elements to constructor.');
    }

    this.intersectionObserver = new IntersectionObserver(this.intersectionObserverCallback, {
      threshold: 1.0,
    });
    this.intersectionObserver.observe(this.el);

    // Fixed positioning on Safari iOS is very broken, and without this hack,
    // focusing on the overflow menu will cause all content to scroll.
    // The -webkit-overflow-scroll CSS property is only available on mobile
    // Safari, so check for it and set the appropriate style to fix this.
    if (
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (window.getComputedStyle(document.body) as any)['-webkit-overflow-scrolling'] !== undefined
    ) {
      [document.documentElement, document.body].forEach(el => {
        el.style.overflow = 'auto';
      });
    }
  }

  private intersectionObserverCallback: IntersectionObserverCallback = entries => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        this.fixedEl.classList.remove('UnitFixedHeader--visible');
      } else {
        this.fixedEl.classList.add('UnitFixedHeader--visible');
      }
    });
  };
}

const fixedHeaderSentinel = document.querySelector('.js-fixedHeaderSentinel');
const fixedHeader = document.querySelector('.js-fixedHeader');
if (fixedHeaderSentinel && fixedHeader) {
  new FixedHeaderController(fixedHeaderSentinel, fixedHeader);
}

const overflowSelect = document.querySelector<HTMLSelectElement>('.js-overflowSelect');
if (overflowSelect) {
  overflowSelect.addEventListener('change', e => {
    window.location.href = (e.target as HTMLSelectElement).value;
  });
}
