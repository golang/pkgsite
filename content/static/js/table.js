/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class ExpandableRowsTableController {
  constructor(table, expandAll) {
    this.table = table;
    this.expandAll = expandAll;
    this.toggles = table.querySelectorAll('[data-aria-controls]');
    this.setAttributes();
    this.attachEventListeners();
    this.updateVisibleItems();
  }
  setAttributes() {
    for (const a of ['data-aria-controls', 'data-aria-labelledby', 'data-id']) {
      this.table.querySelectorAll(`[${a}]`).forEach(t => {
        t.setAttribute(a.replace('data-', ''), t.getAttribute(a) ?? '');
        t.removeAttribute(a);
      });
    }
  }
  attachEventListeners() {
    this.toggles.forEach(t => {
      t.addEventListener('click', e => {
        this.handleToggleClick(e);
      });
    });
    this.expandAll?.addEventListener('click', () => {
      this.expandAllItems();
    });
    document.addEventListener('keydown', e => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
        this.expandAllItems();
      }
    });
  }
  handleToggleClick(e) {
    let target = e.currentTarget;
    if (!target?.hasAttribute('aria-expanded')) {
      target = this.table.querySelector(
        `button[aria-controls="${target?.getAttribute('aria-controls')}"]`
      );
    }
    const isExpanded = target?.getAttribute('aria-expanded') === 'true';
    target?.setAttribute('aria-expanded', isExpanded ? 'false' : 'true');
    e.stopPropagation();
    this.updateVisibleItems();
  }
  expandAllItems() {
    this.table
      .querySelectorAll('[aria-expanded=false]')
      .forEach(t => t.setAttribute('aria-expanded', 'true'));
    this.updateVisibleItems();
  }
  updateVisibleItems() {
    this.toggles.forEach(t => {
      const isExpanded = t?.getAttribute('aria-expanded') === 'true';
      const rowIds = t?.getAttribute('aria-controls')?.trimEnd().split(' ');
      rowIds?.forEach(id => {
        const target = document.getElementById(`${id}`);
        if (isExpanded) {
          target?.classList.add('visible');
          target?.classList.remove('hidden');
        } else {
          target?.classList.add('hidden');
          target?.classList.remove('visible');
        }
      });
    });
  }
}
//# sourceMappingURL=table.js.map
