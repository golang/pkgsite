/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * VersionsController encapsulates event listeners and UI updates
 * for the versions page. As the the expandable sections containing
 * the symbol history for a package are opened and closed it toggles
 * visiblity of the buttons to expand or collapse them. On page load
 * it adds an indicator to the version that matches the version request
 * by the user for the page or the canonical url path.
 */
export class VersionsController {
  private expand = document.querySelector<HTMLButtonElement>('.js-versionsExpand');
  private collapse = document.querySelector<HTMLButtonElement>('.js-versionsCollapse');
  private details = [...document.querySelectorAll<HTMLDetailsElement>('.js-versionDetails')];

  constructor() {
    if (!this.expand?.parentElement) return;
    if (this.details.some(d => d.tagName === 'DETAILS')) {
      this.expand.parentElement.style.display = 'block';
    }

    for (const d of this.details) {
      d.addEventListener('click', () => {
        this.updateButtons();
      });
    }

    this.expand?.addEventListener('click', () => {
      this.details.map(d => (d.open = true));
      this.updateButtons();
    });

    this.collapse?.addEventListener('click', () => {
      this.details.map(d => (d.open = false));
      this.updateButtons();
    });

    this.updateButtons();
    this.setCurrent();
  }

  /**
   * setCurrent applies the active style to the version dot
   * for the version that matches the canonical URL path.
   */
  private setCurrent() {
    const canonicalPath = document.querySelector<HTMLElement>('.js-canonicalURLPath')?.dataset
      ?.canonicalUrlPath;
    const versionLink = document.querySelector<HTMLElement>(
      `.js-versionLink[href="${canonicalPath}"]`
    );
    if (versionLink) {
      versionLink.style.fontWeight = 'bold';
    }
  }

  private updateButtons() {
    setTimeout(() => {
      if (!this.expand || !this.collapse) return;
      let someOpen, someClosed;
      for (const d of this.details) {
        someOpen = someOpen || d.open;
        someClosed = someClosed || !d.open;
      }
      this.expand.style.display = someClosed ? 'inline-block' : 'none';
      this.collapse.style.display = someClosed ? 'none' : 'inline-block';
    });
  }
}

new VersionsController();
