/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * site.ts contains a set of functions that should be invoked for
 * all page views before other scripts are added to the page.
 */

/**
 * A bit of navigation related code for handling dismissible elements.
 */
(function registerHeaderListeners() {
  'use strict';

  const header = document.querySelector('.js-header');
  const menuButtons = document.querySelectorAll('.js-headerMenuButton');
  menuButtons.forEach(button => {
    button.addEventListener('click', e => {
      e.preventDefault();
      header?.classList.toggle('is-active');
      button.setAttribute('aria-expanded', `${header?.classList.contains('is-active') ?? false}`);
    });
  });

  const scrim = document.querySelector('.js-scrim');
  // eslint-disable-next-line no-prototype-builtins
  if (scrim && scrim.hasOwnProperty('addEventListener')) {
    scrim.addEventListener('click', e => {
      e.preventDefault();
      header?.classList.remove('is-active');
      menuButtons.forEach(button => {
        button.setAttribute('aria-expanded', `${header?.classList.contains('is-active') ?? false}`);
      });
    });
  }
})();

interface TagManagerEvent {
  event: string;
  'gtm.start': number;
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
interface Window {
  dataLayer?: (TagManagerEvent | VoidFunction)[];
}

/**
 * setupGoogleTagManager intializes Google Tag Manager.
 */
(function setupGoogleTagManager() {
  window.dataLayer = window.dataLayer || [];
  window.dataLayer.push({
    'gtm.start': new Date().getTime(),
    event: 'gtm.js',
  });
})();

/**
 * removeUTMSource removes the utm_source GET parameter if present.
 * This is done using JavaScript, so that the utm_source is still
 * captured by Google Analytics.
 */
function removeUTMSource() {
  const urlParams = new URLSearchParams(window.location.search);
  const utmSource = urlParams.get('utm_source');
  if (utmSource !== 'gopls' && utmSource !== 'godoc') {
    return;
  }

  /** Strip the utm_source query parameter and replace the URL. **/
  const newURL = new URL(window.location.href);
  urlParams.delete('utm_source');
  newURL.search = urlParams.toString();
  window.history.replaceState(null, '', newURL.toString());
}

if (document.querySelector<HTMLElement>('.js-gtmID')?.dataset.gtmid && window.dataLayer) {
  window.dataLayer.push(function () {
    removeUTMSource();
  });
} else {
  removeUTMSource();
}
