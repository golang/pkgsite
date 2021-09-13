/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const headerHeight = 3.5;

// Append a div above the site header to use for the sticky header transition.
const siteHeader = document.querySelector('.js-siteHeader');
const headerSentinel = document.createElement('div');
siteHeader?.prepend(headerSentinel);

/**
 * headerObserver watches the headerSentinel. When the headerSentinel is out of view a
 * callback function transitions the search results header in to the sticky position.
 */
const headerObserver = new IntersectionObserver(
  ([e]) => {
    if (e.intersectionRatio < 1) {
      for (const x of document.querySelectorAll('[class^="SearchResults-header"')) {
        x.setAttribute('data-fixed', 'true');
      }
    } else {
      for (const x of document.querySelectorAll('[class^="SearchResults-header"')) {
        x.removeAttribute('data-fixed');
      }
    }
  },
  { threshold: 1, rootMargin: `${headerHeight * 16 * 3}px` }
);
headerObserver.observe(headerSentinel);

// Add an event listener to scroll to the top of the page when the whitespace on the
// header is double clicked.
const searchHeader = document.querySelector('.js-searchHeader');
searchHeader?.addEventListener('dblclick', e => {
  const target = e.target;
  if (target === searchHeader || target === searchHeader.lastElementChild) {
    window.getSelection()?.removeAllRanges();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }
});

export {};
