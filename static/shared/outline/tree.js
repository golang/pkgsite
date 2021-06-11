/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class TreeNavController {
  constructor(el) {
    this.el = el;
    this.treeitems = [];
    this.firstChars = [];
    this.firstTreeitem = null;
    this.lastTreeitem = null;
    this.observerCallbacks = [];
    this.init();
  }
  init() {
    this.el.style.setProperty("--js-tree-height", this.el.clientHeight + "px");
    this.findTreeItems();
    this.updateVisibleTreeitems();
    this.observeTargets();
    if (this.firstTreeitem) {
      this.firstTreeitem.el.tabIndex = 0;
    }
  }
  observeTargets() {
    this.addObserver((treeitem) => {
      this.expandTreeitem(treeitem);
      this.setSelected(treeitem);
      if (treeitem.el?.scrollIntoViewIfNeeded) {
        treeitem.el?.scrollIntoViewIfNeeded();
      }
    });
    const targets = new Map();
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        targets.set(entry.target.id, entry.isIntersecting || entry.intersectionRatio === 1);
      }
      for (const [id, isIntersecting] of targets) {
        if (isIntersecting) {
          const active = this.treeitems.find((t) => t.el?.href.endsWith(`#${id}`));
          if (active) {
            for (const fn of this.observerCallbacks) {
              fn(active);
            }
          }
          break;
        }
      }
    }, {
      threshold: 1,
      rootMargin: "-60px 0px 0px 0px"
    });
    for (const href of this.treeitems.map((t) => t.el.getAttribute("href"))) {
      if (href) {
        const id = href.replace(window.location.origin, "").replace("/", "").replace("#", "");
        const target = document.getElementById(id);
        if (target) {
          observer.observe(target);
        }
      }
    }
  }
  addObserver(fn, delay = 200) {
    this.observerCallbacks.push(debounce(fn, delay));
  }
  setFocusToNextItem(currentItem) {
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
  setFocusToPreviousItem(currentItem) {
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
  setFocusToParentItem(currentItem) {
    if (currentItem.groupTreeitem) {
      this.setFocusToItem(currentItem.groupTreeitem);
    }
  }
  setFocusToFirstItem() {
    this.firstTreeitem && this.setFocusToItem(this.firstTreeitem);
  }
  setFocusToLastItem() {
    this.lastTreeitem && this.setFocusToItem(this.lastTreeitem);
  }
  setSelected(currentItem) {
    for (const l1 of this.el.querySelectorAll('[aria-expanded="true"]')) {
      if (l1 === currentItem.el)
        continue;
      if (!l1.nextElementSibling?.contains(currentItem.el)) {
        l1.setAttribute("aria-expanded", "false");
      }
    }
    for (const l1 of this.el.querySelectorAll("[aria-selected]")) {
      if (l1 !== currentItem.el) {
        l1.setAttribute("aria-selected", "false");
      }
    }
    currentItem.el.setAttribute("aria-selected", "true");
    this.updateVisibleTreeitems();
    this.setFocusToItem(currentItem, false);
  }
  expandTreeitem(treeitem) {
    let currentItem = treeitem;
    while (currentItem) {
      if (currentItem.isExpandable) {
        currentItem.el.setAttribute("aria-expanded", "true");
      }
      currentItem = currentItem.groupTreeitem;
    }
    this.updateVisibleTreeitems();
  }
  expandAllSiblingItems(currentItem) {
    for (const ti of this.treeitems) {
      if (ti.groupTreeitem === currentItem.groupTreeitem && ti.isExpandable) {
        this.expandTreeitem(ti);
      }
    }
  }
  collapseTreeitem(currentItem) {
    let groupTreeitem = null;
    if (currentItem.isExpanded()) {
      groupTreeitem = currentItem;
    } else {
      groupTreeitem = currentItem.groupTreeitem;
    }
    if (groupTreeitem) {
      groupTreeitem.el.setAttribute("aria-expanded", "false");
      this.updateVisibleTreeitems();
      this.setFocusToItem(groupTreeitem);
    }
  }
  setFocusByFirstCharacter(currentItem, char) {
    let start, index;
    char = char.toLowerCase();
    start = currentItem.index + 1;
    if (start === this.treeitems.length) {
      start = 0;
    }
    index = this.getIndexFirstChars(start, char);
    if (index === -1) {
      index = this.getIndexFirstChars(0, char);
    }
    if (index > -1) {
      this.setFocusToItem(this.treeitems[index]);
    }
  }
  findTreeItems() {
    const findItems = (el, group) => {
      let ti = group;
      let curr = el.firstElementChild;
      while (curr) {
        if (curr.tagName === "A" || curr.tagName === "SPAN") {
          ti = new TreeItem(curr, this, group);
          this.treeitems.push(ti);
          this.firstChars.push(ti.label.substring(0, 1).toLowerCase());
        }
        if (curr.firstElementChild) {
          findItems(curr, ti);
        }
        curr = curr.nextElementSibling;
      }
    };
    findItems(this.el, null);
    this.treeitems.map((ti, idx) => ti.index = idx);
  }
  updateVisibleTreeitems() {
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
  setFocusToItem(treeitem, focusEl = true) {
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
  getIndexFirstChars(startIndex, char) {
    for (let i = startIndex; i < this.firstChars.length; i++) {
      if (this.treeitems[i].isVisible && char === this.firstChars[i]) {
        return i;
      }
    }
    return -1;
  }
}
class TreeItem {
  constructor(el, treeObj, group) {
    el.tabIndex = -1;
    this.el = el;
    this.groupTreeitem = group;
    this.label = el.textContent?.trim() ?? "";
    this.tree = treeObj;
    this.depth = (group?.depth || 0) + 1;
    this.index = 0;
    const parent = el.parentElement;
    if (parent?.tagName.toLowerCase() === "li") {
      parent?.setAttribute("role", "none");
    }
    el.setAttribute("aria-level", this.depth + "");
    if (el.getAttribute("aria-label")) {
      this.label = el?.getAttribute("aria-label")?.trim() ?? "";
    }
    this.isExpandable = false;
    this.isVisible = false;
    this.isInGroup = !!group;
    let curr = el.nextElementSibling;
    while (curr) {
      if (curr.tagName.toLowerCase() == "ul") {
        const groupId = `${group?.label ?? ""} nav group ${this.label}`.replace(/[\W_]+/g, "_");
        el.setAttribute("aria-owns", groupId);
        el.setAttribute("aria-expanded", "false");
        curr.setAttribute("role", "group");
        curr.setAttribute("id", groupId);
        this.isExpandable = true;
        break;
      }
      curr = curr.nextElementSibling;
    }
    this.init();
  }
  init() {
    this.el.tabIndex = -1;
    if (!this.el.getAttribute("role")) {
      this.el.setAttribute("role", "treeitem");
    }
    this.el.addEventListener("keydown", this.handleKeydown.bind(this));
    this.el.addEventListener("click", this.handleClick.bind(this));
    this.el.addEventListener("focus", this.handleFocus.bind(this));
    this.el.addEventListener("blur", this.handleBlur.bind(this));
  }
  isExpanded() {
    if (this.isExpandable) {
      return this.el.getAttribute("aria-expanded") === "true";
    }
    return false;
  }
  isSelected() {
    return this.el.getAttribute("aria-selected") === "true";
  }
  handleClick(event) {
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
  handleFocus() {
    let el = this.el;
    if (this.isExpandable) {
      el = el.firstElementChild ?? el;
    }
    el.classList.add("focus");
  }
  handleBlur() {
    let el = this.el;
    if (this.isExpandable) {
      el = el.firstElementChild ?? el;
    }
    el.classList.remove("focus");
  }
  handleKeydown(event) {
    if (event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }
    let captured = false;
    switch (event.key) {
      case " ":
      case "Enter":
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
      case "ArrowUp":
        this.tree.setFocusToPreviousItem(this);
        captured = true;
        break;
      case "ArrowDown":
        this.tree.setFocusToNextItem(this);
        captured = true;
        break;
      case "ArrowRight":
        if (this.isExpandable) {
          if (this.isExpanded()) {
            this.tree.setFocusToNextItem(this);
          } else {
            this.tree.expandTreeitem(this);
          }
        }
        captured = true;
        break;
      case "ArrowLeft":
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
      case "Home":
        this.tree.setFocusToFirstItem();
        captured = true;
        break;
      case "End":
        this.tree.setFocusToLastItem();
        captured = true;
        break;
      default:
        if (event.key.length === 1 && event.key.match(/\S/)) {
          if (event.key == "*") {
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
function debounce(func, wait) {
  let timeout;
  return (...args) => {
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
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidHJlZS50cyJdLAogICJzb3VyY2VzQ29udGVudCI6IFsiLyoqXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMjEgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8qKlxuICogVHJlZU5hdkNvbnRyb2xsZXIgaXMgdGhlIG5hdmlnYXRpb24gdHJlZSBjb21wb25lbnQgb2YgdGhlIGRvY3VtZW50YXRpb24gcGFnZS5cbiAqIEl0IGFkZHMgYWNjZXNzaWJsaXR5IGF0dHJpYnV0ZXMgdG8gYSB0cmVlLCBvYnNlcnZlcyB0aGUgaGVhZGluZyBlbGVtZW50c1xuICogZm9jdXMgdGhlIHRvcG1vc3QgbGluayBmb3IgaGVhZGluZ3MgdmlzaWJsZSBvbiB0aGUgcGFnZSwgYW5kIGltcGxlbWVudHMgdGhlXG4gKiBXQUktQVJJQSBUcmVldmlldyBEZXNpZ24gUGF0dGVybiB3aXRoIGZ1bGxcbiAqIFtrZXlib2FyZCBzdXBwb3J0XShodHRwczovL3d3dy53My5vcmcvVFIvd2FpLWFyaWEtcHJhY3RpY2VzL2V4YW1wbGVzL3RyZWV2aWV3L3RyZWV2aWV3LTIvdHJlZXZpZXctMmEuaHRtbCNrYmRfbGFiZWwpLlxuICovXG5leHBvcnQgY2xhc3MgVHJlZU5hdkNvbnRyb2xsZXIge1xuICB0cmVlaXRlbXM6IFRyZWVJdGVtW107XG5cbiAgLyoqXG4gICAqIGZpcnN0Q2hhcnMgaXMgdGhlIGZpcnN0IGNoYXJhY3RlciBvZiBlYWNoIHRyZWVpdGVtIGluIHRoZSBzYW1lIG9yZGVyXG4gICAqIGFzIHRoaXMudHJlZWl0ZW1zLiBXZSB1c2UgdGhpcyBhcnJheSB0byBzZXQgZm9jdXMgYnkgY2hhcmFjdGVyIHdoZW5cbiAgICogbmF2aWdhdGluZyB0aGUgdHJlZSB3aXRoIGEga2V5Ym9hcmQuXG4gICAqL1xuICBwcml2YXRlIGZpcnN0Q2hhcnM6IHN0cmluZ1tdO1xuICBwcml2YXRlIGZpcnN0VHJlZWl0ZW06IFRyZWVJdGVtIHwgbnVsbDtcbiAgcHJpdmF0ZSBsYXN0VHJlZWl0ZW06IFRyZWVJdGVtIHwgbnVsbDtcbiAgcHJpdmF0ZSBvYnNlcnZlckNhbGxiYWNrczogKCh0OiBUcmVlSXRlbSkgPT4gdm9pZClbXTtcblxuICBjb25zdHJ1Y3Rvcihwcml2YXRlIGVsOiBIVE1MRWxlbWVudCkge1xuICAgIHRoaXMudHJlZWl0ZW1zID0gW107XG4gICAgdGhpcy5maXJzdENoYXJzID0gW107XG4gICAgdGhpcy5maXJzdFRyZWVpdGVtID0gbnVsbDtcbiAgICB0aGlzLmxhc3RUcmVlaXRlbSA9IG51bGw7XG4gICAgdGhpcy5vYnNlcnZlckNhbGxiYWNrcyA9IFtdO1xuICAgIHRoaXMuaW5pdCgpO1xuICB9XG5cbiAgcHJpdmF0ZSBpbml0KCk6IHZvaWQge1xuICAgIHRoaXMuZWwuc3R5bGUuc2V0UHJvcGVydHkoJy0tanMtdHJlZS1oZWlnaHQnLCB0aGlzLmVsLmNsaWVudEhlaWdodCArICdweCcpO1xuICAgIHRoaXMuZmluZFRyZWVJdGVtcygpO1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICAgIHRoaXMub2JzZXJ2ZVRhcmdldHMoKTtcbiAgICBpZiAodGhpcy5maXJzdFRyZWVpdGVtKSB7XG4gICAgICB0aGlzLmZpcnN0VHJlZWl0ZW0uZWwudGFiSW5kZXggPSAwO1xuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgb2JzZXJ2ZVRhcmdldHMoKSB7XG4gICAgdGhpcy5hZGRPYnNlcnZlcih0cmVlaXRlbSA9PiB7XG4gICAgICB0aGlzLmV4cGFuZFRyZWVpdGVtKHRyZWVpdGVtKTtcbiAgICAgIHRoaXMuc2V0U2VsZWN0ZWQodHJlZWl0ZW0pO1xuICAgICAgLy8gVGhlIGN1cnJlbnQgdmVyc2lvbiBvZiBUeXBlU2NyaXB0IGlzIG5vdCBhd2FyZSBvZiBIVE1MRWxlbWVudC5zY3JvbGxJbnRvVmlld0lmTmVlZGVkLlxuICAgICAgLy8gZXNsaW50LWRpc2FibGUtbmV4dC1saW5lIEB0eXBlc2NyaXB0LWVzbGludC9uby1leHBsaWNpdC1hbnlcbiAgICAgIGlmICgodHJlZWl0ZW0uZWwgYXMgYW55KT8uc2Nyb2xsSW50b1ZpZXdJZk5lZWRlZCkge1xuICAgICAgICAvLyBUaGUgY3VycmVudCB2ZXJzaW9uIG9mIFR5cGVTY3JpcHQgaXMgbm90IGF3YXJlIG9mIEhUTUxFbGVtZW50LnNjcm9sbEludG9WaWV3SWZOZWVkZWQuXG4gICAgICAgIC8vIGVzbGludC1kaXNhYmxlLW5leHQtbGluZSBAdHlwZXNjcmlwdC1lc2xpbnQvbm8tZXhwbGljaXQtYW55XG4gICAgICAgICh0cmVlaXRlbS5lbCBhcyBhbnkpPy5zY3JvbGxJbnRvVmlld0lmTmVlZGVkKCk7XG4gICAgICB9XG4gICAgfSk7XG5cbiAgICBjb25zdCB0YXJnZXRzID0gbmV3IE1hcDxzdHJpbmcsIGJvb2xlYW4+KCk7XG4gICAgY29uc3Qgb2JzZXJ2ZXIgPSBuZXcgSW50ZXJzZWN0aW9uT2JzZXJ2ZXIoXG4gICAgICBlbnRyaWVzID0+IHtcbiAgICAgICAgZm9yIChjb25zdCBlbnRyeSBvZiBlbnRyaWVzKSB7XG4gICAgICAgICAgdGFyZ2V0cy5zZXQoZW50cnkudGFyZ2V0LmlkLCBlbnRyeS5pc0ludGVyc2VjdGluZyB8fCBlbnRyeS5pbnRlcnNlY3Rpb25SYXRpbyA9PT0gMSk7XG4gICAgICAgIH1cbiAgICAgICAgZm9yIChjb25zdCBbaWQsIGlzSW50ZXJzZWN0aW5nXSBvZiB0YXJnZXRzKSB7XG4gICAgICAgICAgaWYgKGlzSW50ZXJzZWN0aW5nKSB7XG4gICAgICAgICAgICBjb25zdCBhY3RpdmUgPSB0aGlzLnRyZWVpdGVtcy5maW5kKHQgPT5cbiAgICAgICAgICAgICAgKHQuZWwgYXMgSFRNTEFuY2hvckVsZW1lbnQpPy5ocmVmLmVuZHNXaXRoKGAjJHtpZH1gKVxuICAgICAgICAgICAgKTtcbiAgICAgICAgICAgIGlmIChhY3RpdmUpIHtcbiAgICAgICAgICAgICAgZm9yIChjb25zdCBmbiBvZiB0aGlzLm9ic2VydmVyQ2FsbGJhY2tzKSB7XG4gICAgICAgICAgICAgICAgZm4oYWN0aXZlKTtcbiAgICAgICAgICAgICAgfVxuICAgICAgICAgICAgfVxuICAgICAgICAgICAgYnJlYWs7XG4gICAgICAgICAgfVxuICAgICAgICB9XG4gICAgICB9LFxuICAgICAge1xuICAgICAgICB0aHJlc2hvbGQ6IDEuMCxcbiAgICAgICAgcm9vdE1hcmdpbjogJy02MHB4IDBweCAwcHggMHB4JyxcbiAgICAgIH1cbiAgICApO1xuXG4gICAgZm9yIChjb25zdCBocmVmIG9mIHRoaXMudHJlZWl0ZW1zLm1hcCh0ID0+IHQuZWwuZ2V0QXR0cmlidXRlKCdocmVmJykpKSB7XG4gICAgICBpZiAoaHJlZikge1xuICAgICAgICBjb25zdCBpZCA9IGhyZWYucmVwbGFjZSh3aW5kb3cubG9jYXRpb24ub3JpZ2luLCAnJykucmVwbGFjZSgnLycsICcnKS5yZXBsYWNlKCcjJywgJycpO1xuICAgICAgICBjb25zdCB0YXJnZXQgPSBkb2N1bWVudC5nZXRFbGVtZW50QnlJZChpZCk7XG4gICAgICAgIGlmICh0YXJnZXQpIHtcbiAgICAgICAgICBvYnNlcnZlci5vYnNlcnZlKHRhcmdldCk7XG4gICAgICAgIH1cbiAgICAgIH1cbiAgICB9XG4gIH1cblxuICBhZGRPYnNlcnZlcihmbjogKHQ6IFRyZWVJdGVtKSA9PiB2b2lkLCBkZWxheSA9IDIwMCk6IHZvaWQge1xuICAgIHRoaXMub2JzZXJ2ZXJDYWxsYmFja3MucHVzaChkZWJvdW5jZShmbiwgZGVsYXkpKTtcbiAgfVxuXG4gIHNldEZvY3VzVG9OZXh0SXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBsZXQgbmV4dEl0ZW0gPSBudWxsO1xuICAgIGZvciAobGV0IGkgPSBjdXJyZW50SXRlbS5pbmRleCArIDE7IGkgPCB0aGlzLnRyZWVpdGVtcy5sZW5ndGg7IGkrKykge1xuICAgICAgY29uc3QgdGkgPSB0aGlzLnRyZWVpdGVtc1tpXTtcbiAgICAgIGlmICh0aS5pc1Zpc2libGUpIHtcbiAgICAgICAgbmV4dEl0ZW0gPSB0aTtcbiAgICAgICAgYnJlYWs7XG4gICAgICB9XG4gICAgfVxuICAgIGlmIChuZXh0SXRlbSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbShuZXh0SXRlbSk7XG4gICAgfVxuICB9XG5cbiAgc2V0Rm9jdXNUb1ByZXZpb3VzSXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBsZXQgcHJldkl0ZW0gPSBudWxsO1xuICAgIGZvciAobGV0IGkgPSBjdXJyZW50SXRlbS5pbmRleCAtIDE7IGkgPiAtMTsgaS0tKSB7XG4gICAgICBjb25zdCB0aSA9IHRoaXMudHJlZWl0ZW1zW2ldO1xuICAgICAgaWYgKHRpLmlzVmlzaWJsZSkge1xuICAgICAgICBwcmV2SXRlbSA9IHRpO1xuICAgICAgICBicmVhaztcbiAgICAgIH1cbiAgICB9XG4gICAgaWYgKHByZXZJdGVtKSB7XG4gICAgICB0aGlzLnNldEZvY3VzVG9JdGVtKHByZXZJdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c1RvUGFyZW50SXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBpZiAoY3VycmVudEl0ZW0uZ3JvdXBUcmVlaXRlbSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbShjdXJyZW50SXRlbS5ncm91cFRyZWVpdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c1RvRmlyc3RJdGVtKCk6IHZvaWQge1xuICAgIHRoaXMuZmlyc3RUcmVlaXRlbSAmJiB0aGlzLnNldEZvY3VzVG9JdGVtKHRoaXMuZmlyc3RUcmVlaXRlbSk7XG4gIH1cblxuICBzZXRGb2N1c1RvTGFzdEl0ZW0oKTogdm9pZCB7XG4gICAgdGhpcy5sYXN0VHJlZWl0ZW0gJiYgdGhpcy5zZXRGb2N1c1RvSXRlbSh0aGlzLmxhc3RUcmVlaXRlbSk7XG4gIH1cblxuICBzZXRTZWxlY3RlZChjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBmb3IgKGNvbnN0IGwxIG9mIHRoaXMuZWwucXVlcnlTZWxlY3RvckFsbCgnW2FyaWEtZXhwYW5kZWQ9XCJ0cnVlXCJdJykpIHtcbiAgICAgIGlmIChsMSA9PT0gY3VycmVudEl0ZW0uZWwpIGNvbnRpbnVlO1xuICAgICAgaWYgKCFsMS5uZXh0RWxlbWVudFNpYmxpbmc/LmNvbnRhaW5zKGN1cnJlbnRJdGVtLmVsKSkge1xuICAgICAgICBsMS5zZXRBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnLCAnZmFsc2UnKTtcbiAgICAgIH1cbiAgICB9XG4gICAgZm9yIChjb25zdCBsMSBvZiB0aGlzLmVsLnF1ZXJ5U2VsZWN0b3JBbGwoJ1thcmlhLXNlbGVjdGVkXScpKSB7XG4gICAgICBpZiAobDEgIT09IGN1cnJlbnRJdGVtLmVsKSB7XG4gICAgICAgIGwxLnNldEF0dHJpYnV0ZSgnYXJpYS1zZWxlY3RlZCcsICdmYWxzZScpO1xuICAgICAgfVxuICAgIH1cbiAgICBjdXJyZW50SXRlbS5lbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtc2VsZWN0ZWQnLCAndHJ1ZScpO1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICAgIHRoaXMuc2V0Rm9jdXNUb0l0ZW0oY3VycmVudEl0ZW0sIGZhbHNlKTtcbiAgfVxuXG4gIGV4cGFuZFRyZWVpdGVtKHRyZWVpdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBjdXJyZW50SXRlbTogVHJlZUl0ZW0gfCBudWxsID0gdHJlZWl0ZW07XG4gICAgd2hpbGUgKGN1cnJlbnRJdGVtKSB7XG4gICAgICBpZiAoY3VycmVudEl0ZW0uaXNFeHBhbmRhYmxlKSB7XG4gICAgICAgIGN1cnJlbnRJdGVtLmVsLnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICd0cnVlJyk7XG4gICAgICB9XG4gICAgICBjdXJyZW50SXRlbSA9IGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW07XG4gICAgfVxuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICB9XG5cbiAgZXhwYW5kQWxsU2libGluZ0l0ZW1zKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGZvciAoY29uc3QgdGkgb2YgdGhpcy50cmVlaXRlbXMpIHtcbiAgICAgIGlmICh0aS5ncm91cFRyZWVpdGVtID09PSBjdXJyZW50SXRlbS5ncm91cFRyZWVpdGVtICYmIHRpLmlzRXhwYW5kYWJsZSkge1xuICAgICAgICB0aGlzLmV4cGFuZFRyZWVpdGVtKHRpKTtcbiAgICAgIH1cbiAgICB9XG4gIH1cblxuICBjb2xsYXBzZVRyZWVpdGVtKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBncm91cFRyZWVpdGVtID0gbnVsbDtcblxuICAgIGlmIChjdXJyZW50SXRlbS5pc0V4cGFuZGVkKCkpIHtcbiAgICAgIGdyb3VwVHJlZWl0ZW0gPSBjdXJyZW50SXRlbTtcbiAgICB9IGVsc2Uge1xuICAgICAgZ3JvdXBUcmVlaXRlbSA9IGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW07XG4gICAgfVxuXG4gICAgaWYgKGdyb3VwVHJlZWl0ZW0pIHtcbiAgICAgIGdyb3VwVHJlZWl0ZW0uZWwuc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgJ2ZhbHNlJyk7XG4gICAgICB0aGlzLnVwZGF0ZVZpc2libGVUcmVlaXRlbXMoKTtcbiAgICAgIHRoaXMuc2V0Rm9jdXNUb0l0ZW0oZ3JvdXBUcmVlaXRlbSk7XG4gICAgfVxuICB9XG5cbiAgc2V0Rm9jdXNCeUZpcnN0Q2hhcmFjdGVyKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSwgY2hhcjogc3RyaW5nKTogdm9pZCB7XG4gICAgbGV0IHN0YXJ0OiBudW1iZXIsIGluZGV4OiBudW1iZXI7XG4gICAgY2hhciA9IGNoYXIudG9Mb3dlckNhc2UoKTtcblxuICAgIC8vIEdldCBzdGFydCBpbmRleCBmb3Igc2VhcmNoIGJhc2VkIG9uIHBvc2l0aW9uIG9mIGN1cnJlbnRJdGVtXG4gICAgc3RhcnQgPSBjdXJyZW50SXRlbS5pbmRleCArIDE7XG4gICAgaWYgKHN0YXJ0ID09PSB0aGlzLnRyZWVpdGVtcy5sZW5ndGgpIHtcbiAgICAgIHN0YXJ0ID0gMDtcbiAgICB9XG5cbiAgICAvLyBDaGVjayByZW1haW5pbmcgc2xvdHMgaW4gdGhlIG1lbnVcbiAgICBpbmRleCA9IHRoaXMuZ2V0SW5kZXhGaXJzdENoYXJzKHN0YXJ0LCBjaGFyKTtcblxuICAgIC8vIElmIG5vdCBmb3VuZCBpbiByZW1haW5pbmcgc2xvdHMsIGNoZWNrIGZyb20gYmVnaW5uaW5nXG4gICAgaWYgKGluZGV4ID09PSAtMSkge1xuICAgICAgaW5kZXggPSB0aGlzLmdldEluZGV4Rmlyc3RDaGFycygwLCBjaGFyKTtcbiAgICB9XG5cbiAgICAvLyBJZiBtYXRjaCB3YXMgZm91bmQuLi5cbiAgICBpZiAoaW5kZXggPiAtMSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbSh0aGlzLnRyZWVpdGVtc1tpbmRleF0pO1xuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgZmluZFRyZWVJdGVtcygpIHtcbiAgICBjb25zdCBmaW5kSXRlbXMgPSAoZWw6IEhUTUxFbGVtZW50LCBncm91cDogVHJlZUl0ZW0gfCBudWxsKSA9PiB7XG4gICAgICBsZXQgdGkgPSBncm91cDtcbiAgICAgIGxldCBjdXJyID0gZWwuZmlyc3RFbGVtZW50Q2hpbGQgYXMgSFRNTEVsZW1lbnQ7XG4gICAgICB3aGlsZSAoY3Vycikge1xuICAgICAgICBpZiAoY3Vyci50YWdOYW1lID09PSAnQScgfHwgY3Vyci50YWdOYW1lID09PSAnU1BBTicpIHtcbiAgICAgICAgICB0aSA9IG5ldyBUcmVlSXRlbShjdXJyLCB0aGlzLCBncm91cCk7XG4gICAgICAgICAgdGhpcy50cmVlaXRlbXMucHVzaCh0aSk7XG4gICAgICAgICAgdGhpcy5maXJzdENoYXJzLnB1c2godGkubGFiZWwuc3Vic3RyaW5nKDAsIDEpLnRvTG93ZXJDYXNlKCkpO1xuICAgICAgICB9XG4gICAgICAgIGlmIChjdXJyLmZpcnN0RWxlbWVudENoaWxkKSB7XG4gICAgICAgICAgZmluZEl0ZW1zKGN1cnIsIHRpKTtcbiAgICAgICAgfVxuICAgICAgICBjdXJyID0gY3Vyci5uZXh0RWxlbWVudFNpYmxpbmcgYXMgSFRNTEVsZW1lbnQ7XG4gICAgICB9XG4gICAgfTtcbiAgICBmaW5kSXRlbXModGhpcy5lbCBhcyBIVE1MRWxlbWVudCwgbnVsbCk7XG4gICAgdGhpcy50cmVlaXRlbXMubWFwKCh0aSwgaWR4KSA9PiAodGkuaW5kZXggPSBpZHgpKTtcbiAgfVxuXG4gIHByaXZhdGUgdXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpOiB2b2lkIHtcbiAgICB0aGlzLmZpcnN0VHJlZWl0ZW0gPSB0aGlzLnRyZWVpdGVtc1swXTtcblxuICAgIGZvciAoY29uc3QgdGkgb2YgdGhpcy50cmVlaXRlbXMpIHtcbiAgICAgIGxldCBwYXJlbnQgPSB0aS5ncm91cFRyZWVpdGVtO1xuICAgICAgdGkuaXNWaXNpYmxlID0gdHJ1ZTtcbiAgICAgIHdoaWxlIChwYXJlbnQgJiYgcGFyZW50LmVsICE9PSB0aGlzLmVsKSB7XG4gICAgICAgIGlmICghcGFyZW50LmlzRXhwYW5kZWQoKSkge1xuICAgICAgICAgIHRpLmlzVmlzaWJsZSA9IGZhbHNlO1xuICAgICAgICB9XG4gICAgICAgIHBhcmVudCA9IHBhcmVudC5ncm91cFRyZWVpdGVtO1xuICAgICAgfVxuICAgICAgaWYgKHRpLmlzVmlzaWJsZSkge1xuICAgICAgICB0aGlzLmxhc3RUcmVlaXRlbSA9IHRpO1xuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgc2V0Rm9jdXNUb0l0ZW0odHJlZWl0ZW06IFRyZWVJdGVtLCBmb2N1c0VsID0gdHJ1ZSkge1xuICAgIHRyZWVpdGVtLmVsLnRhYkluZGV4ID0gMDtcbiAgICBpZiAoZm9jdXNFbCkge1xuICAgICAgdHJlZWl0ZW0uZWwuZm9jdXMoKTtcbiAgICB9XG4gICAgZm9yIChjb25zdCB0aSBvZiB0aGlzLnRyZWVpdGVtcykge1xuICAgICAgaWYgKHRpICE9PSB0cmVlaXRlbSkge1xuICAgICAgICB0aS5lbC50YWJJbmRleCA9IC0xO1xuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgZ2V0SW5kZXhGaXJzdENoYXJzKHN0YXJ0SW5kZXg6IG51bWJlciwgY2hhcjogc3RyaW5nKTogbnVtYmVyIHtcbiAgICBmb3IgKGxldCBpID0gc3RhcnRJbmRleDsgaSA8IHRoaXMuZmlyc3RDaGFycy5sZW5ndGg7IGkrKykge1xuICAgICAgaWYgKHRoaXMudHJlZWl0ZW1zW2ldLmlzVmlzaWJsZSAmJiBjaGFyID09PSB0aGlzLmZpcnN0Q2hhcnNbaV0pIHtcbiAgICAgICAgcmV0dXJuIGk7XG4gICAgICB9XG4gICAgfVxuICAgIHJldHVybiAtMTtcbiAgfVxufVxuXG5jbGFzcyBUcmVlSXRlbSB7XG4gIGVsOiBIVE1MRWxlbWVudDtcbiAgZ3JvdXBUcmVlaXRlbTogVHJlZUl0ZW0gfCBudWxsO1xuICBsYWJlbDogc3RyaW5nO1xuICBpc0V4cGFuZGFibGU6IGJvb2xlYW47XG4gIGlzVmlzaWJsZTogYm9vbGVhbjtcbiAgZGVwdGg6IG51bWJlcjtcbiAgaW5kZXg6IG51bWJlcjtcblxuICBwcml2YXRlIHRyZWU6IFRyZWVOYXZDb250cm9sbGVyO1xuICBwcml2YXRlIGlzSW5Hcm91cDogYm9vbGVhbjtcblxuICBjb25zdHJ1Y3RvcihlbDogSFRNTEVsZW1lbnQsIHRyZWVPYmo6IFRyZWVOYXZDb250cm9sbGVyLCBncm91cDogVHJlZUl0ZW0gfCBudWxsKSB7XG4gICAgZWwudGFiSW5kZXggPSAtMTtcbiAgICB0aGlzLmVsID0gZWw7XG4gICAgdGhpcy5ncm91cFRyZWVpdGVtID0gZ3JvdXA7XG4gICAgdGhpcy5sYWJlbCA9IGVsLnRleHRDb250ZW50Py50cmltKCkgPz8gJyc7XG4gICAgdGhpcy50cmVlID0gdHJlZU9iajtcbiAgICB0aGlzLmRlcHRoID0gKGdyb3VwPy5kZXB0aCB8fCAwKSArIDE7XG4gICAgdGhpcy5pbmRleCA9IDA7XG5cbiAgICBjb25zdCBwYXJlbnQgPSBlbC5wYXJlbnRFbGVtZW50O1xuICAgIGlmIChwYXJlbnQ/LnRhZ05hbWUudG9Mb3dlckNhc2UoKSA9PT0gJ2xpJykge1xuICAgICAgcGFyZW50Py5zZXRBdHRyaWJ1dGUoJ3JvbGUnLCAnbm9uZScpO1xuICAgIH1cbiAgICBlbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtbGV2ZWwnLCB0aGlzLmRlcHRoICsgJycpO1xuICAgIGlmIChlbC5nZXRBdHRyaWJ1dGUoJ2FyaWEtbGFiZWwnKSkge1xuICAgICAgdGhpcy5sYWJlbCA9IGVsPy5nZXRBdHRyaWJ1dGUoJ2FyaWEtbGFiZWwnKT8udHJpbSgpID8/ICcnO1xuICAgIH1cblxuICAgIHRoaXMuaXNFeHBhbmRhYmxlID0gZmFsc2U7XG4gICAgdGhpcy5pc1Zpc2libGUgPSBmYWxzZTtcbiAgICB0aGlzLmlzSW5Hcm91cCA9ICEhZ3JvdXA7XG5cbiAgICBsZXQgY3VyciA9IGVsLm5leHRFbGVtZW50U2libGluZztcbiAgICB3aGlsZSAoY3Vycikge1xuICAgICAgaWYgKGN1cnIudGFnTmFtZS50b0xvd2VyQ2FzZSgpID09ICd1bCcpIHtcbiAgICAgICAgY29uc3QgZ3JvdXBJZCA9IGAke2dyb3VwPy5sYWJlbCA/PyAnJ30gbmF2IGdyb3VwICR7dGhpcy5sYWJlbH1gLnJlcGxhY2UoL1tcXFdfXSsvZywgJ18nKTtcbiAgICAgICAgZWwuc2V0QXR0cmlidXRlKCdhcmlhLW93bnMnLCBncm91cElkKTtcbiAgICAgICAgZWwuc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgJ2ZhbHNlJyk7XG4gICAgICAgIGN1cnIuc2V0QXR0cmlidXRlKCdyb2xlJywgJ2dyb3VwJyk7XG4gICAgICAgIGN1cnIuc2V0QXR0cmlidXRlKCdpZCcsIGdyb3VwSWQpO1xuICAgICAgICB0aGlzLmlzRXhwYW5kYWJsZSA9IHRydWU7XG4gICAgICAgIGJyZWFrO1xuICAgICAgfVxuXG4gICAgICBjdXJyID0gY3Vyci5uZXh0RWxlbWVudFNpYmxpbmc7XG4gICAgfVxuICAgIHRoaXMuaW5pdCgpO1xuICB9XG5cbiAgcHJpdmF0ZSBpbml0KCkge1xuICAgIHRoaXMuZWwudGFiSW5kZXggPSAtMTtcbiAgICBpZiAoIXRoaXMuZWwuZ2V0QXR0cmlidXRlKCdyb2xlJykpIHtcbiAgICAgIHRoaXMuZWwuc2V0QXR0cmlidXRlKCdyb2xlJywgJ3RyZWVpdGVtJyk7XG4gICAgfVxuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcigna2V5ZG93bicsIHRoaXMuaGFuZGxlS2V5ZG93bi5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgdGhpcy5oYW5kbGVDbGljay5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2ZvY3VzJywgdGhpcy5oYW5kbGVGb2N1cy5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2JsdXInLCB0aGlzLmhhbmRsZUJsdXIuYmluZCh0aGlzKSk7XG4gIH1cblxuICBpc0V4cGFuZGVkKCkge1xuICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgcmV0dXJuIHRoaXMuZWwuZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICd0cnVlJztcbiAgICB9XG5cbiAgICByZXR1cm4gZmFsc2U7XG4gIH1cblxuICBpc1NlbGVjdGVkKCkge1xuICAgIHJldHVybiB0aGlzLmVsLmdldEF0dHJpYnV0ZSgnYXJpYS1zZWxlY3RlZCcpID09PSAndHJ1ZSc7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUNsaWNrKGV2ZW50OiBNb3VzZUV2ZW50KSB7XG4gICAgLy8gb25seSBwcm9jZXNzIGNsaWNrIGV2ZW50cyB0aGF0IGRpcmVjdGx5IGhhcHBlbmVkIG9uIHRoaXMgdHJlZWl0ZW1cbiAgICBpZiAoZXZlbnQudGFyZ2V0ICE9PSB0aGlzLmVsICYmIGV2ZW50LnRhcmdldCAhPT0gdGhpcy5lbC5maXJzdEVsZW1lbnRDaGlsZCkge1xuICAgICAgcmV0dXJuO1xuICAgIH1cbiAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgIGlmICh0aGlzLmlzRXhwYW5kZWQoKSAmJiB0aGlzLmlzU2VsZWN0ZWQoKSkge1xuICAgICAgICB0aGlzLnRyZWUuY29sbGFwc2VUcmVlaXRlbSh0aGlzKTtcbiAgICAgIH0gZWxzZSB7XG4gICAgICAgIHRoaXMudHJlZS5leHBhbmRUcmVlaXRlbSh0aGlzKTtcbiAgICAgIH1cbiAgICAgIGV2ZW50LnN0b3BQcm9wYWdhdGlvbigpO1xuICAgIH1cbiAgICB0aGlzLnRyZWUuc2V0U2VsZWN0ZWQodGhpcyk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUZvY3VzKCkge1xuICAgIGxldCBlbCA9IHRoaXMuZWw7XG4gICAgaWYgKHRoaXMuaXNFeHBhbmRhYmxlKSB7XG4gICAgICBlbCA9IChlbC5maXJzdEVsZW1lbnRDaGlsZCBhcyBIVE1MRWxlbWVudCkgPz8gZWw7XG4gICAgfVxuICAgIGVsLmNsYXNzTGlzdC5hZGQoJ2ZvY3VzJyk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUJsdXIoKSB7XG4gICAgbGV0IGVsID0gdGhpcy5lbDtcbiAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgIGVsID0gKGVsLmZpcnN0RWxlbWVudENoaWxkIGFzIEhUTUxFbGVtZW50KSA/PyBlbDtcbiAgICB9XG4gICAgZWwuY2xhc3NMaXN0LnJlbW92ZSgnZm9jdXMnKTtcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlS2V5ZG93bihldmVudDogS2V5Ym9hcmRFdmVudCkge1xuICAgIGlmIChldmVudC5hbHRLZXkgfHwgZXZlbnQuY3RybEtleSB8fCBldmVudC5tZXRhS2V5KSB7XG4gICAgICByZXR1cm47XG4gICAgfVxuXG4gICAgbGV0IGNhcHR1cmVkID0gZmFsc2U7XG4gICAgc3dpdGNoIChldmVudC5rZXkpIHtcbiAgICAgIGNhc2UgJyAnOlxuICAgICAgY2FzZSAnRW50ZXInOlxuICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGVkKCkgJiYgdGhpcy5pc1NlbGVjdGVkKCkpIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5jb2xsYXBzZVRyZWVpdGVtKHRoaXMpO1xuICAgICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuZXhwYW5kVHJlZWl0ZW0odGhpcyk7XG4gICAgICAgICAgfVxuICAgICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICBldmVudC5zdG9wUHJvcGFnYXRpb24oKTtcbiAgICAgICAgfVxuICAgICAgICB0aGlzLnRyZWUuc2V0U2VsZWN0ZWQodGhpcyk7XG4gICAgICAgIGJyZWFrO1xuXG4gICAgICBjYXNlICdBcnJvd1VwJzpcbiAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9QcmV2aW91c0l0ZW0odGhpcyk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0Fycm93RG93bic6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTmV4dEl0ZW0odGhpcyk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0Fycm93UmlnaHQnOlxuICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGVkKCkpIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTmV4dEl0ZW0odGhpcyk7XG4gICAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5leHBhbmRUcmVlaXRlbSh0aGlzKTtcbiAgICAgICAgICB9XG4gICAgICAgIH1cbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnQXJyb3dMZWZ0JzpcbiAgICAgICAgaWYgKHRoaXMuaXNFeHBhbmRhYmxlICYmIHRoaXMuaXNFeHBhbmRlZCgpKSB7XG4gICAgICAgICAgdGhpcy50cmVlLmNvbGxhcHNlVHJlZWl0ZW0odGhpcyk7XG4gICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgIGlmICh0aGlzLmlzSW5Hcm91cCkge1xuICAgICAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9QYXJlbnRJdGVtKHRoaXMpO1xuICAgICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICAgIH1cbiAgICAgICAgfVxuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnSG9tZSc6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvRmlyc3RJdGVtKCk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0VuZCc6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTGFzdEl0ZW0oKTtcbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgZGVmYXVsdDpcbiAgICAgICAgaWYgKGV2ZW50LmtleS5sZW5ndGggPT09IDEgJiYgZXZlbnQua2V5Lm1hdGNoKC9cXFMvKSkge1xuICAgICAgICAgIGlmIChldmVudC5rZXkgPT0gJyonKSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuZXhwYW5kQWxsU2libGluZ0l0ZW1zKHRoaXMpO1xuICAgICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuc2V0Rm9jdXNCeUZpcnN0Q2hhcmFjdGVyKHRoaXMsIGV2ZW50LmtleSk7XG4gICAgICAgICAgfVxuICAgICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgfVxuICAgICAgICBicmVhaztcbiAgICB9XG5cbiAgICBpZiAoY2FwdHVyZWQpIHtcbiAgICAgIGV2ZW50LnN0b3BQcm9wYWdhdGlvbigpO1xuICAgICAgZXZlbnQucHJldmVudERlZmF1bHQoKTtcbiAgICB9XG4gIH1cbn1cblxuLy8gZXNsaW50LWRpc2FibGUtbmV4dC1saW5lIEB0eXBlc2NyaXB0LWVzbGludC9uby1leHBsaWNpdC1hbnlcbmZ1bmN0aW9uIGRlYm91bmNlPFQgZXh0ZW5kcyAoLi4uYXJnczogYW55W10pID0+IGFueT4oZnVuYzogVCwgd2FpdDogbnVtYmVyKSB7XG4gIGxldCB0aW1lb3V0OiBSZXR1cm5UeXBlPHR5cGVvZiBzZXRUaW1lb3V0PiB8IG51bGw7XG4gIHJldHVybiAoLi4uYXJnczogUGFyYW1ldGVyczxUPikgPT4ge1xuICAgIGNvbnN0IGxhdGVyID0gKCkgPT4ge1xuICAgICAgdGltZW91dCA9IG51bGw7XG4gICAgICBmdW5jKC4uLmFyZ3MpO1xuICAgIH07XG4gICAgaWYgKHRpbWVvdXQpIHtcbiAgICAgIGNsZWFyVGltZW91dCh0aW1lb3V0KTtcbiAgICB9XG4gICAgdGltZW91dCA9IHNldFRpbWVvdXQobGF0ZXIsIHdhaXQpO1xuICB9O1xufVxuIl0sCiAgIm1hcHBpbmdzIjogIkFBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBY08sK0JBQXdCO0FBQUEsRUFhN0IsWUFBb0IsSUFBaUI7QUFBakI7QUFDbEIsU0FBSyxZQUFZO0FBQ2pCLFNBQUssYUFBYTtBQUNsQixTQUFLLGdCQUFnQjtBQUNyQixTQUFLLGVBQWU7QUFDcEIsU0FBSyxvQkFBb0I7QUFDekIsU0FBSztBQUFBO0FBQUEsRUFHQyxPQUFhO0FBQ25CLFNBQUssR0FBRyxNQUFNLFlBQVksb0JBQW9CLEtBQUssR0FBRyxlQUFlO0FBQ3JFLFNBQUs7QUFDTCxTQUFLO0FBQ0wsU0FBSztBQUNMLFFBQUksS0FBSyxlQUFlO0FBQ3RCLFdBQUssY0FBYyxHQUFHLFdBQVc7QUFBQTtBQUFBO0FBQUEsRUFJN0IsaUJBQWlCO0FBQ3ZCLFNBQUssWUFBWSxjQUFZO0FBQzNCLFdBQUss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
