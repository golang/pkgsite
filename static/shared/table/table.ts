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
  private rows: HTMLTableRowElement[];
  private toggles: HTMLButtonElement[];

  /**
   * Create a table controller.
   * @param table - The table element to which the controller binds.
   */
  constructor(private table: HTMLTableElement, private toggleAll?: HTMLButtonElement | null) {
    this.rows = Array.from(table.querySelectorAll<HTMLTableRowElement>('[data-aria-controls]'));
    this.toggles = Array.from(this.table.querySelectorAll('[aria-expanded]'));
    this.setAttributes();
    this.attachEventListeners();
    this.update();
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
    this.rows.forEach(t => {
      t.addEventListener('click', e => {
        this.handleToggleClick(e);
      });
    });
    this.toggleAll?.addEventListener('click', () => {
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
    this.update();
  }

  expandAllItems = (): void => {
    this.toggles.map(t => t.setAttribute('aria-expanded', 'true'));
    this.update();
  };

  private collapseAllItems = () => {
    this.toggles.map(t => t.setAttribute('aria-expanded', 'false'));
    this.update();
  };

  private update = () => {
    this.updateVisibleItems();
    setTimeout(() => this.updateGlobalToggle());
  };

  private updateVisibleItems() {
    this.rows.map(t => {
      const isExpanded = t?.getAttribute('aria-expanded') === 'true';
      const rowIds = t?.getAttribute('aria-controls')?.trimEnd().split(' ');
      rowIds?.map(id => {
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

  private updateGlobalToggle() {
    if (!this.toggleAll) return;
    if (this.rows.some(t => t.hasAttribute('aria-expanded'))) {
      this.toggleAll.style.display = 'block';
    }
    const someCollapsed = this.toggles.some(el => el.getAttribute('aria-expanded') === 'false');
    if (someCollapsed) {
      this.toggleAll.innerText = 'Expand all';
      this.toggleAll.onclick = this.expandAllItems;
    } else {
      this.toggleAll.innerText = 'Collapse all';
      this.toggleAll.onclick = this.collapseAllItems;
    }
  }
}
