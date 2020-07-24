/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

new FixedHeaderController(
  document.querySelector('.js-fixedHeaderSentinel'),
  document.querySelector('.js-fixedHeader')
);
document
  .querySelectorAll('.js-overflowingTabList')
  .forEach(el => new OverflowingTabListController(el));
document.querySelectorAll('.js-copyToClipboard').forEach(el => {
  new CopyToClipboardController(el);
});
