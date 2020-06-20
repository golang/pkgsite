/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// This file implements the behavior of the keyboard shortcut which allows
// for users to press 'y' to to change browser URL to the canonical URL
// without triggering a reload.

const canonicalURLPath = document.querySelector('.js-canonicalURLPath').dataset['canonicalUrlPath'];
if (canonicalURLPath && canonicalURLPath !== '') {
  document.addEventListener('keydown', e => {
    // TODO(golang.org/issue/40246): consolidate keyboard shortcut behavior across the site.
    const t = e.target.tagName;
    if (t === 'INPUT' || t === 'SELECT' || t === 'TEXTAREA') {
      return;
    }
    if (e.target.isContentEditable) {
      return;
    }
    if (e.metaKey || e.ctrlKey) {
      return;
    }
    switch (e.key) {
      case 'y':
        window.history.replaceState(null, '', canonicalURLPath);
        break;
    }
  });
}
