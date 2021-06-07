/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * ToolTipController handles closing tooltips on external clicks.
 */
export class ToolTipController {
  constructor(private el: HTMLDetailsElement) {
    document.addEventListener('click', e => {
      const insideTooltip = this.el.contains(e.target as Element);
      if (!insideTooltip) {
        this.el.removeAttribute('open');
      }
    });
  }
}
