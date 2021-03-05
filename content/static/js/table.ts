/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Controller for a table element with expandable rows. Adds event listeners to
 * a toggle within a table row that controls visiblity of additional related
 * rows in the table.
 *
 * @example
 * ```typescript
 * import {ExpandableRowsTableController} from '/static/js/table';
 *
 * const el = document .querySelector<HTMLTableElement>('.js-myTableElement')
 * new ExpandableRowsTableController(el));
 * ```
 */
export class ExpandableRowsTableController {
  private toggles: NodeListOf<HTMLTableRowElement>;

  /**
   * Create a table controller.
   * @param table - The table element to which the controller binds.
   */
  constructor(private table: HTMLTableElement, private expandAll?: HTMLButtonElement | null) {
    this.toggles = table.querySelectorAll<HTMLTableRowElement>('[data-aria-controls]');
    this.setAttributes();
    this.attachEventListeners();
    this.updateVisibleItems();
  }

  /**
   * setAttributes sets data-aria-* and data-id attributes to regular
   * html attributes as a workaround for limitations from safehtml.
   */
  private setAttributes() {
    for (const a of ['data-aria-controls', 'data-aria-labelledby', 'data-id']) {
      this.table.querySelectorAll(`[${a}]`).forEach(t => {
        t.setAttribute(a.replace('data-', ''), t.getAttribute(a) ?? '');
        t.removeAttribute(a);
      });
    }
  }

  private attachEventListeners() {
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

  private handleToggleClick(e: MouseEvent) {
    let target = e.currentTarget as HTMLTableRowElement | null;
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

  private expandAllItems() {
    this.table
      .querySelectorAll('[aria-expanded=false]')
      .forEach(t => t.setAttribute('aria-expanded', 'true'));
    this.updateVisibleItems();
  }

  private updateVisibleItems() {
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
