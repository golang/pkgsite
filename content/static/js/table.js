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
        var _a;
        t.setAttribute(
          a.replace('data-', ''),
          (_a = t.getAttribute(a)) !== null && _a !== void 0 ? _a : ''
        );
        t.removeAttribute(a);
      });
    }
  }
  attachEventListeners() {
    var _a;
    this.toggles.forEach(t => {
      t.addEventListener('click', e => {
        this.handleToggleClick(e);
      });
    });
    (_a = this.expandAll) === null || _a === void 0
      ? void 0
      : _a.addEventListener('click', () => {
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
    if (!(target === null || target === void 0 ? void 0 : target.hasAttribute('aria-expanded'))) {
      target = this.table.querySelector(
        `button[aria-controls="${
          target === null || target === void 0 ? void 0 : target.getAttribute('aria-controls')
        }"]`
      );
    }
    const isExpanded =
      (target === null || target === void 0 ? void 0 : target.getAttribute('aria-expanded')) ===
      'true';
    target === null || target === void 0
      ? void 0
      : target.setAttribute('aria-expanded', isExpanded ? 'false' : 'true');
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
      var _a;
      const isExpanded =
        (t === null || t === void 0 ? void 0 : t.getAttribute('aria-expanded')) === 'true';
      const rowIds =
        (_a = t === null || t === void 0 ? void 0 : t.getAttribute('aria-controls')) === null ||
        _a === void 0
          ? void 0
          : _a.trimEnd().split(' ');
      rowIds === null || rowIds === void 0
        ? void 0
        : rowIds.forEach(id => {
            const target = document.getElementById(`${id}`);
            if (isExpanded) {
              target === null || target === void 0 ? void 0 : target.classList.add('visible');
              target === null || target === void 0 ? void 0 : target.classList.remove('hidden');
            } else {
              target === null || target === void 0 ? void 0 : target.classList.add('hidden');
              target === null || target === void 0 ? void 0 : target.classList.remove('visible');
            }
          });
    });
  }
}
//# sourceMappingURL=table.js.map
