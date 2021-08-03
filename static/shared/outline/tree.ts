/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * TreeNavController is the navigation tree component of the documentation page.
 * It adds accessiblity attributes to a tree, observes the heading elements
 * focus the topmost link for headings visible on the page, and implements the
 * WAI-ARIA Treeview Design Pattern with full
 * [keyboard support](https://www.w3.org/TR/wai-aria-practices/examples/treeview/treeview-2/treeview-2a.html#kbd_label).
 */
export class TreeNavController {
  treeitems: TreeItem[];

  /**
   * firstChars is the first character of each treeitem in the same order
   * as this.treeitems. We use this array to set focus by character when
   * navigating the tree with a keyboard.
   */
  private firstChars: string[];
  private firstTreeitem: TreeItem | null;
  private lastTreeitem: TreeItem | null;
  private observerCallbacks: ((t: TreeItem) => void)[];

  constructor(private el: HTMLElement) {
    this.treeitems = [];
    this.firstChars = [];
    this.firstTreeitem = null;
    this.lastTreeitem = null;
    this.observerCallbacks = [];
    this.init();
  }

  private init(): void {
    this.el.style.setProperty('--js-tree-height', this.el.clientHeight + 'px');
    this.findTreeItems();
    this.updateVisibleTreeitems();
    this.observeTargets();
    if (this.firstTreeitem) {
      this.firstTreeitem.el.tabIndex = 0;
    }
  }

  private observeTargets() {
    this.addObserver(treeitem => {
      this.expandTreeitem(treeitem);
      this.setSelected(treeitem);
      // TODO: Fix scroll issue in https://golang.org/issue/47450.
      // treeitem.el.scrollIntoView({ block: 'nearest' });
    });

    const targets = new Map<string, boolean>();
    const observer = new IntersectionObserver(
      entries => {
        for (const entry of entries) {
          targets.set(entry.target.id, entry.isIntersecting || entry.intersectionRatio === 1);
        }
        for (const [id, isIntersecting] of targets) {
          if (isIntersecting) {
            const active = this.treeitems.find(t =>
              (t.el as HTMLAnchorElement)?.href.endsWith(`#${id}`)
            );
            if (active) {
              for (const fn of this.observerCallbacks) {
                fn(active);
              }
            }
            break;
          }
        }
      },
      {
        threshold: 1.0,
        rootMargin: '-60px 0px 0px 0px',
      }
    );

    for (const href of this.treeitems.map(t => t.el.getAttribute('href'))) {
      if (href) {
        const id = href.replace(window.location.origin, '').replace('/', '').replace('#', '');
        const target = document.getElementById(id);
        if (target) {
          observer.observe(target);
        }
      }
    }
  }

  addObserver(fn: (t: TreeItem) => void, delay = 200): void {
    this.observerCallbacks.push(debounce(fn, delay));
  }

  setFocusToNextItem(currentItem: TreeItem): void {
    let nextItem = null;
    for (let i = currentItem.index + 1; i < this.treeitems.length; i++) {
      const ti = this.treeitems[i];
      if (ti.isVisible) {
        nextItem = ti;
        break;
      }
    }
    if (nextItem) {
      this.setFocusToItem(nextItem);
    }
  }

  setFocusToPreviousItem(currentItem: TreeItem): void {
    let prevItem = null;
    for (let i = currentItem.index - 1; i > -1; i--) {
      const ti = this.treeitems[i];
      if (ti.isVisible) {
        prevItem = ti;
        break;
      }
    }
    if (prevItem) {
      this.setFocusToItem(prevItem);
    }
  }

  setFocusToParentItem(currentItem: TreeItem): void {
    if (currentItem.groupTreeitem) {
      this.setFocusToItem(currentItem.groupTreeitem);
    }
  }

  setFocusToFirstItem(): void {
    this.firstTreeitem && this.setFocusToItem(this.firstTreeitem);
  }

  setFocusToLastItem(): void {
    this.lastTreeitem && this.setFocusToItem(this.lastTreeitem);
  }

  setSelected(currentItem: TreeItem): void {
    for (const l1 of this.el.querySelectorAll('[aria-expanded="true"]')) {
      if (l1 === currentItem.el) continue;
      if (!l1.nextElementSibling?.contains(currentItem.el)) {
        l1.setAttribute('aria-expanded', 'false');
      }
    }
    for (const l1 of this.el.querySelectorAll('[aria-selected]')) {
      if (l1 !== currentItem.el) {
        l1.setAttribute('aria-selected', 'false');
      }
    }
    currentItem.el.setAttribute('aria-selected', 'true');
    this.updateVisibleTreeitems();
    this.setFocusToItem(currentItem, false);
  }

  expandTreeitem(treeitem: TreeItem): void {
    let currentItem: TreeItem | null = treeitem;
    while (currentItem) {
      if (currentItem.isExpandable) {
        currentItem.el.setAttribute('aria-expanded', 'true');
      }
      currentItem = currentItem.groupTreeitem;
    }
    this.updateVisibleTreeitems();
  }

  expandAllSiblingItems(currentItem: TreeItem): void {
    for (const ti of this.treeitems) {
      if (ti.groupTreeitem === currentItem.groupTreeitem && ti.isExpandable) {
        this.expandTreeitem(ti);
      }
    }
  }

  collapseTreeitem(currentItem: TreeItem): void {
    let groupTreeitem = null;

    if (currentItem.isExpanded()) {
      groupTreeitem = currentItem;
    } else {
      groupTreeitem = currentItem.groupTreeitem;
    }

    if (groupTreeitem) {
      groupTreeitem.el.setAttribute('aria-expanded', 'false');
      this.updateVisibleTreeitems();
      this.setFocusToItem(groupTreeitem);
    }
  }

  setFocusByFirstCharacter(currentItem: TreeItem, char: string): void {
    let start: number, index: number;
    char = char.toLowerCase();

    // Get start index for search based on position of currentItem
    start = currentItem.index + 1;
    if (start === this.treeitems.length) {
      start = 0;
    }

    // Check remaining slots in the menu
    index = this.getIndexFirstChars(start, char);

    // If not found in remaining slots, check from beginning
    if (index === -1) {
      index = this.getIndexFirstChars(0, char);
    }

    // If match was found...
    if (index > -1) {
      this.setFocusToItem(this.treeitems[index]);
    }
  }

  private findTreeItems() {
    const findItems = (el: HTMLElement, group: TreeItem | null) => {
      let ti = group;
      let curr = el.firstElementChild as HTMLElement;
      while (curr) {
        if (curr.tagName === 'A' || curr.tagName === 'SPAN') {
          ti = new TreeItem(curr, this, group);
          this.treeitems.push(ti);
          this.firstChars.push(ti.label.substring(0, 1).toLowerCase());
        }
        if (curr.firstElementChild) {
          findItems(curr, ti);
        }
        curr = curr.nextElementSibling as HTMLElement;
      }
    };
    findItems(this.el as HTMLElement, null);
    this.treeitems.map((ti, idx) => (ti.index = idx));
  }

  private updateVisibleTreeitems(): void {
    this.firstTreeitem = this.treeitems[0];

    for (const ti of this.treeitems) {
      let parent = ti.groupTreeitem;
      ti.isVisible = true;
      while (parent && parent.el !== this.el) {
        if (!parent.isExpanded()) {
          ti.isVisible = false;
        }
        parent = parent.groupTreeitem;
      }
      if (ti.isVisible) {
        this.lastTreeitem = ti;
      }
    }
  }

  private setFocusToItem(treeitem: TreeItem, focusEl = true) {
    treeitem.el.tabIndex = 0;
    if (focusEl) {
      treeitem.el.focus();
    }
    for (const ti of this.treeitems) {
      if (ti !== treeitem) {
        ti.el.tabIndex = -1;
      }
    }
  }

  private getIndexFirstChars(startIndex: number, char: string): number {
    for (let i = startIndex; i < this.firstChars.length; i++) {
      if (this.treeitems[i].isVisible && char === this.firstChars[i]) {
        return i;
      }
    }
    return -1;
  }
}

class TreeItem {
  el: HTMLElement;
  groupTreeitem: TreeItem | null;
  label: string;
  isExpandable: boolean;
  isVisible: boolean;
  depth: number;
  index: number;

  private tree: TreeNavController;
  private isInGroup: boolean;

  constructor(el: HTMLElement, treeObj: TreeNavController, group: TreeItem | null) {
    el.tabIndex = -1;
    this.el = el;
    this.groupTreeitem = group;
    this.label = el.textContent?.trim() ?? '';
    this.tree = treeObj;
    this.depth = (group?.depth || 0) + 1;
    this.index = 0;

    const parent = el.parentElement;
    if (parent?.tagName.toLowerCase() === 'li') {
      parent?.setAttribute('role', 'none');
    }
    el.setAttribute('aria-level', this.depth + '');
    if (el.getAttribute('aria-label')) {
      this.label = el?.getAttribute('aria-label')?.trim() ?? '';
    }

    this.isExpandable = false;
    this.isVisible = false;
    this.isInGroup = !!group;

    let curr = el.nextElementSibling;
    while (curr) {
      if (curr.tagName.toLowerCase() == 'ul') {
        const groupId = `${group?.label ?? ''} nav group ${this.label}`.replace(/[\W_]+/g, '_');
        el.setAttribute('aria-owns', groupId);
        el.setAttribute('aria-expanded', 'false');
        curr.setAttribute('role', 'group');
        curr.setAttribute('id', groupId);
        this.isExpandable = true;
        break;
      }

      curr = curr.nextElementSibling;
    }
    this.init();
  }

  private init() {
    this.el.tabIndex = -1;
    if (!this.el.getAttribute('role')) {
      this.el.setAttribute('role', 'treeitem');
    }
    this.el.addEventListener('keydown', this.handleKeydown.bind(this));
    this.el.addEventListener('click', this.handleClick.bind(this));
    this.el.addEventListener('focus', this.handleFocus.bind(this));
    this.el.addEventListener('blur', this.handleBlur.bind(this));
  }

  isExpanded() {
    if (this.isExpandable) {
      return this.el.getAttribute('aria-expanded') === 'true';
    }

    return false;
  }

  isSelected() {
    return this.el.getAttribute('aria-selected') === 'true';
  }

  private handleClick(event: MouseEvent) {
    // only process click events that directly happened on this treeitem
    if (event.target !== this.el && event.target !== this.el.firstElementChild) {
      return;
    }
    if (this.isExpandable) {
      if (this.isExpanded() && this.isSelected()) {
        this.tree.collapseTreeitem(this);
      } else {
        this.tree.expandTreeitem(this);
      }
      event.stopPropagation();
    }
    this.tree.setSelected(this);
  }

  private handleFocus() {
    let el = this.el;
    if (this.isExpandable) {
      el = (el.firstElementChild as HTMLElement) ?? el;
    }
    el.classList.add('focus');
  }

  private handleBlur() {
    let el = this.el;
    if (this.isExpandable) {
      el = (el.firstElementChild as HTMLElement) ?? el;
    }
    el.classList.remove('focus');
  }

  private handleKeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }

    let captured = false;
    switch (event.key) {
      case ' ':
      case 'Enter':
        if (this.isExpandable) {
          if (this.isExpanded() && this.isSelected()) {
            this.tree.collapseTreeitem(this);
          } else {
            this.tree.expandTreeitem(this);
          }
          captured = true;
        } else {
          event.stopPropagation();
        }
        this.tree.setSelected(this);
        break;

      case 'ArrowUp':
        this.tree.setFocusToPreviousItem(this);
        captured = true;
        break;

      case 'ArrowDown':
        this.tree.setFocusToNextItem(this);
        captured = true;
        break;

      case 'ArrowRight':
        if (this.isExpandable) {
          if (this.isExpanded()) {
            this.tree.setFocusToNextItem(this);
          } else {
            this.tree.expandTreeitem(this);
          }
        }
        captured = true;
        break;

      case 'ArrowLeft':
        if (this.isExpandable && this.isExpanded()) {
          this.tree.collapseTreeitem(this);
          captured = true;
        } else {
          if (this.isInGroup) {
            this.tree.setFocusToParentItem(this);
            captured = true;
          }
        }
        break;

      case 'Home':
        this.tree.setFocusToFirstItem();
        captured = true;
        break;

      case 'End':
        this.tree.setFocusToLastItem();
        captured = true;
        break;

      default:
        if (event.key.length === 1 && event.key.match(/\S/)) {
          if (event.key == '*') {
            this.tree.expandAllSiblingItems(this);
          } else {
            this.tree.setFocusByFirstCharacter(this, event.key);
          }
          captured = true;
        }
        break;
    }

    if (captured) {
      event.stopPropagation();
      event.preventDefault();
    }
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function debounce<T extends (...args: any[]) => any>(func: T, wait: number) {
  let timeout: ReturnType<typeof setTimeout> | null;
  return (...args: Parameters<T>) => {
    const later = () => {
      timeout = null;
      func(...args);
    };
    if (timeout) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(later, wait);
  };
}
