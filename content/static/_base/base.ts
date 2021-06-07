/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import '../_header/header.js';
import '../_keyboard/keyboard.js';
import { ClipboardController } from '../_clipboard/clipboard.js';
import { ToolTipController } from '../_tooltip/tooltip.js';
import { SelectNavController } from '../_outline/select.js';
import { ModalController } from '../_modal/modal.js';
import { ExpandableRowsTableController } from '../_table/table.js';

for (const el of document.querySelectorAll<HTMLButtonElement>('.js-clipboard')) {
  new ClipboardController(el);
}

for (const el of document.querySelectorAll<HTMLDialogElement>('.js-modal')) {
  new ModalController(el);
}

for (const t of document.querySelectorAll<HTMLDetailsElement>('.js-tooltip')) {
  new ToolTipController(t);
}

for (const el of document.querySelectorAll<HTMLSelectElement>('.js-selectNav')) {
  new SelectNavController(el);
}

for (const el of document.querySelectorAll<HTMLTableElement>('.js-expandableTable')) {
  new ExpandableRowsTableController(el);
}

interface TagManagerEvent {
  event: string;
  'gtm.start': number;
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
declare global {
  interface Window {
    dataLayer?: (TagManagerEvent | VoidFunction)[];
  }
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
  if (utmSource !== 'gopls' && utmSource !== 'godoc' && utmSource !== 'pkggodev') {
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
