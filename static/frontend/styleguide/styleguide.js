(() => {
  // static/shared/outline/select.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var SelectNavController = class {
    constructor(el) {
      this.el = el;
      this.el.addEventListener("change", (e) => {
        const target = e.target;
        let href = target.value;
        if (!target.value.startsWith("/")) {
          href = "/" + href;
        }
        window.location.href = href;
      });
    }
  };
  function makeSelectNav(tree) {
    const label = document.createElement("label");
    label.classList.add("go-Label");
    label.setAttribute("aria-label", "Menu");
    const select = document.createElement("select");
    select.classList.add("go-Select", "js-selectNav");
    label.appendChild(select);
    const outline = document.createElement("optgroup");
    outline.label = "Outline";
    select.appendChild(outline);
    const groupMap = {};
    let group;
    for (const t of tree.treeitems) {
      if (Number(t.depth) > 4)
        continue;
      if (t.groupTreeitem) {
        group = groupMap[t.groupTreeitem.label];
        if (!group) {
          group = groupMap[t.groupTreeitem.label] = document.createElement("optgroup");
          group.label = t.groupTreeitem.label;
          select.appendChild(group);
        }
      } else {
        group = outline;
      }
      const o = document.createElement("option");
      o.label = t.label;
      o.textContent = t.label;
      o.value = t.el.href.replace(window.location.origin, "").replace("/", "");
      group.appendChild(o);
    }
    tree.addObserver((t) => {
      const hash = t.el.hash;
      const value = select.querySelector(`[value$="${hash}"]`)?.value;
      if (value) {
        select.value = value;
      }
    }, 50);
    return label;
  }

  // static/shared/outline/tree.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var TreeNavController = class {
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
  };
  var TreeItem = class {
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
  };
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

  // static/frontend/styleguide/styleguide.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  window.addEventListener("load", () => {
    const tree = document.querySelector(".js-tree");
    if (tree) {
      const treeCtrl = new TreeNavController(tree);
      const select = makeSelectNav(treeCtrl);
      document.querySelector(".js-mainNavMobile")?.appendChild(select);
    }
    const guideTree = document.querySelector(".Outline .js-tree");
    if (guideTree) {
      const treeCtrl = new TreeNavController(guideTree);
      const select = makeSelectNav(treeCtrl);
      document.querySelector(".Outline .js-select")?.appendChild(select);
    }
    for (const el of document.querySelectorAll(".js-toggleTheme")) {
      el.addEventListener("click", (e) => {
        const value = e.currentTarget.getAttribute("data-value");
        document.documentElement.setAttribute("data-theme", String(value));
      });
    }
    for (const el of document.querySelectorAll(".js-toggleLayout")) {
      el.addEventListener("click", (e) => {
        const value = e.currentTarget.getAttribute("data-value");
        document.documentElement.setAttribute("data-layout", String(value));
      });
    }
    for (const el of document.querySelectorAll(".js-selectNav")) {
      new SelectNavController(el);
    }
  });
  customElements.define("go-color", class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty("display", "contents");
      const name = this.id;
      this.removeAttribute("id");
      this.innerHTML = `
        <div style="--color: var(${name});" class="GoColor-circle"></div>
        <span>
          <div id="${name}" class="go-textLabel GoColor-title">${name.replace("--color-", "").replaceAll("-", " ")}</div>
          <pre class="StringifyElement-markup">var(${name})</pre>
        </span>
      `;
      this.querySelector("pre")?.addEventListener("click", () => {
        navigator.clipboard.writeText(`var(${name})`);
      });
    }
  });
  customElements.define("go-icon", class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty("display", "contents");
      const name = this.getAttribute("name");
      this.innerHTML = `<p id="icon-${name}" class="go-textLabel GoIcon-title">${name.replaceAll("_", " ")}</p>
        <stringify-el>
          <img class="go-Icon" height="24" width="24" src="/static/shared/icon/${name}_gm_grey_24dp.svg" alt="">
        </stringify-el>
      `;
    }
  });
  customElements.define("clone-el", class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty("display", "contents");
      const selector = this.getAttribute("selector");
      if (!selector)
        return;
      const html = "    " + document.querySelector(selector)?.outerHTML;
      this.innerHTML = `
        <stringify-el collapsed>${html}</stringify-el>
      `;
    }
  });
  customElements.define("stringify-el", class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty("display", "contents");
      const html = this.innerHTML;
      const idAttr = this.id ? ` id="${this.id}"` : "";
      this.removeAttribute("id");
      let markup = `<pre class="StringifyElement-markup">` + escape(trim(html)) + `</pre>`;
      if (this.hasAttribute("collapsed")) {
        markup = `<details class="StringifyElement-details"><summary>Markup</summary>${markup}</details>`;
      }
      this.innerHTML = `<span${idAttr}>${html}</span>${markup}`;
      this.querySelector("pre")?.addEventListener("click", () => {
        navigator.clipboard.writeText(html);
      });
    }
  });
  function trim(html) {
    return html.split("\n").reduce((acc, val) => {
      if (acc.result.length === 0) {
        const start = val.indexOf("<");
        acc.start = start === -1 ? 0 : start;
      }
      val = val.slice(acc.start);
      if (val) {
        acc.result.push(val);
      }
      return acc;
    }, {result: [], start: 0}).result.join("\n");
  }
  function escape(html) {
    return html?.replaceAll("<", "&lt;")?.replaceAll(">", "&gt;");
  }
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiLi4vLi4vc2hhcmVkL291dGxpbmUvc2VsZWN0LnRzIiwgIi4uLy4uL3NoYXJlZC9vdXRsaW5lL3RyZWUudHMiLCAic3R5bGVndWlkZS50cyJdLAogICJzb3VyY2VzQ29udGVudCI6IFsiLyoqXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMjEgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbmltcG9ydCB7IFRyZWVOYXZDb250cm9sbGVyIH0gZnJvbSAnLi90cmVlLmpzJztcblxuZXhwb3J0IGNsYXNzIFNlbGVjdE5hdkNvbnRyb2xsZXIge1xuICBjb25zdHJ1Y3Rvcihwcml2YXRlIGVsOiBFbGVtZW50KSB7XG4gICAgdGhpcy5lbC5hZGRFdmVudExpc3RlbmVyKCdjaGFuZ2UnLCBlID0+IHtcbiAgICAgIGNvbnN0IHRhcmdldCA9IGUudGFyZ2V0IGFzIEhUTUxTZWxlY3RFbGVtZW50O1xuICAgICAgbGV0IGhyZWYgPSB0YXJnZXQudmFsdWU7XG4gICAgICBpZiAoIXRhcmdldC52YWx1ZS5zdGFydHNXaXRoKCcvJykpIHtcbiAgICAgICAgaHJlZiA9ICcvJyArIGhyZWY7XG4gICAgICB9XG4gICAgICB3aW5kb3cubG9jYXRpb24uaHJlZiA9IGhyZWY7XG4gICAgfSk7XG4gIH1cbn1cblxuZXhwb3J0IGZ1bmN0aW9uIG1ha2VTZWxlY3ROYXYodHJlZTogVHJlZU5hdkNvbnRyb2xsZXIpOiBIVE1MTGFiZWxFbGVtZW50IHtcbiAgY29uc3QgbGFiZWwgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdsYWJlbCcpO1xuICBsYWJlbC5jbGFzc0xpc3QuYWRkKCdnby1MYWJlbCcpO1xuICBsYWJlbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtbGFiZWwnLCAnTWVudScpO1xuICBjb25zdCBzZWxlY3QgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdzZWxlY3QnKTtcbiAgc2VsZWN0LmNsYXNzTGlzdC5hZGQoJ2dvLVNlbGVjdCcsICdqcy1zZWxlY3ROYXYnKTtcbiAgbGFiZWwuYXBwZW5kQ2hpbGQoc2VsZWN0KTtcbiAgY29uc3Qgb3V0bGluZSA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ29wdGdyb3VwJyk7XG4gIG91dGxpbmUubGFiZWwgPSAnT3V0bGluZSc7XG4gIHNlbGVjdC5hcHBlbmRDaGlsZChvdXRsaW5lKTtcbiAgY29uc3QgZ3JvdXBNYXA6IFJlY29yZDxzdHJpbmcsIEhUTUxPcHRHcm91cEVsZW1lbnQ+ID0ge307XG4gIGxldCBncm91cDogSFRNTE9wdEdyb3VwRWxlbWVudDtcbiAgZm9yIChjb25zdCB0IG9mIHRyZWUudHJlZWl0ZW1zKSB7XG4gICAgaWYgKE51bWJlcih0LmRlcHRoKSA+IDQpIGNvbnRpbnVlO1xuICAgIGlmICh0Lmdyb3VwVHJlZWl0ZW0pIHtcbiAgICAgIGdyb3VwID0gZ3JvdXBNYXBbdC5ncm91cFRyZWVpdGVtLmxhYmVsXTtcbiAgICAgIGlmICghZ3JvdXApIHtcbiAgICAgICAgZ3JvdXAgPSBncm91cE1hcFt0Lmdyb3VwVHJlZWl0ZW0ubGFiZWxdID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnb3B0Z3JvdXAnKTtcbiAgICAgICAgZ3JvdXAubGFiZWwgPSB0Lmdyb3VwVHJlZWl0ZW0ubGFiZWw7XG4gICAgICAgIHNlbGVjdC5hcHBlbmRDaGlsZChncm91cCk7XG4gICAgICB9XG4gICAgfSBlbHNlIHtcbiAgICAgIGdyb3VwID0gb3V0bGluZTtcbiAgICB9XG4gICAgY29uc3QgbyA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ29wdGlvbicpO1xuICAgIG8ubGFiZWwgPSB0LmxhYmVsO1xuICAgIG8udGV4dENvbnRlbnQgPSB0LmxhYmVsO1xuICAgIG8udmFsdWUgPSAodC5lbCBhcyBIVE1MQW5jaG9yRWxlbWVudCkuaHJlZi5yZXBsYWNlKHdpbmRvdy5sb2NhdGlvbi5vcmlnaW4sICcnKS5yZXBsYWNlKCcvJywgJycpO1xuICAgIGdyb3VwLmFwcGVuZENoaWxkKG8pO1xuICB9XG4gIHRyZWUuYWRkT2JzZXJ2ZXIodCA9PiB7XG4gICAgY29uc3QgaGFzaCA9ICh0LmVsIGFzIEhUTUxBbmNob3JFbGVtZW50KS5oYXNoO1xuICAgIGNvbnN0IHZhbHVlID0gc2VsZWN0LnF1ZXJ5U2VsZWN0b3I8SFRNTE9wdGlvbkVsZW1lbnQ+KGBbdmFsdWUkPVwiJHtoYXNofVwiXWApPy52YWx1ZTtcbiAgICBpZiAodmFsdWUpIHtcbiAgICAgIHNlbGVjdC52YWx1ZSA9IHZhbHVlO1xuICAgIH1cbiAgfSwgNTApO1xuICByZXR1cm4gbGFiZWw7XG59XG4iLCAiLyoqXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMjEgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8qKlxuICogVHJlZU5hdkNvbnRyb2xsZXIgaXMgdGhlIG5hdmlnYXRpb24gdHJlZSBjb21wb25lbnQgb2YgdGhlIGRvY3VtZW50YXRpb24gcGFnZS5cbiAqIEl0IGFkZHMgYWNjZXNzaWJsaXR5IGF0dHJpYnV0ZXMgdG8gYSB0cmVlLCBvYnNlcnZlcyB0aGUgaGVhZGluZyBlbGVtZW50c1xuICogZm9jdXMgdGhlIHRvcG1vc3QgbGluayBmb3IgaGVhZGluZ3MgdmlzaWJsZSBvbiB0aGUgcGFnZSwgYW5kIGltcGxlbWVudHMgdGhlXG4gKiBXQUktQVJJQSBUcmVldmlldyBEZXNpZ24gUGF0dGVybiB3aXRoIGZ1bGxcbiAqIFtrZXlib2FyZCBzdXBwb3J0XShodHRwczovL3d3dy53My5vcmcvVFIvd2FpLWFyaWEtcHJhY3RpY2VzL2V4YW1wbGVzL3RyZWV2aWV3L3RyZWV2aWV3LTIvdHJlZXZpZXctMmEuaHRtbCNrYmRfbGFiZWwpLlxuICovXG5leHBvcnQgY2xhc3MgVHJlZU5hdkNvbnRyb2xsZXIge1xuICB0cmVlaXRlbXM6IFRyZWVJdGVtW107XG5cbiAgLyoqXG4gICAqIGZpcnN0Q2hhcnMgaXMgdGhlIGZpcnN0IGNoYXJhY3RlciBvZiBlYWNoIHRyZWVpdGVtIGluIHRoZSBzYW1lIG9yZGVyXG4gICAqIGFzIHRoaXMudHJlZWl0ZW1zLiBXZSB1c2UgdGhpcyBhcnJheSB0byBzZXQgZm9jdXMgYnkgY2hhcmFjdGVyIHdoZW5cbiAgICogbmF2aWdhdGluZyB0aGUgdHJlZSB3aXRoIGEga2V5Ym9hcmQuXG4gICAqL1xuICBwcml2YXRlIGZpcnN0Q2hhcnM6IHN0cmluZ1tdO1xuICBwcml2YXRlIGZpcnN0VHJlZWl0ZW06IFRyZWVJdGVtIHwgbnVsbDtcbiAgcHJpdmF0ZSBsYXN0VHJlZWl0ZW06IFRyZWVJdGVtIHwgbnVsbDtcbiAgcHJpdmF0ZSBvYnNlcnZlckNhbGxiYWNrczogKCh0OiBUcmVlSXRlbSkgPT4gdm9pZClbXTtcblxuICBjb25zdHJ1Y3Rvcihwcml2YXRlIGVsOiBIVE1MRWxlbWVudCkge1xuICAgIHRoaXMudHJlZWl0ZW1zID0gW107XG4gICAgdGhpcy5maXJzdENoYXJzID0gW107XG4gICAgdGhpcy5maXJzdFRyZWVpdGVtID0gbnVsbDtcbiAgICB0aGlzLmxhc3RUcmVlaXRlbSA9IG51bGw7XG4gICAgdGhpcy5vYnNlcnZlckNhbGxiYWNrcyA9IFtdO1xuICAgIHRoaXMuaW5pdCgpO1xuICB9XG5cbiAgcHJpdmF0ZSBpbml0KCk6IHZvaWQge1xuICAgIHRoaXMuZWwuc3R5bGUuc2V0UHJvcGVydHkoJy0tanMtdHJlZS1oZWlnaHQnLCB0aGlzLmVsLmNsaWVudEhlaWdodCArICdweCcpO1xuICAgIHRoaXMuZmluZFRyZWVJdGVtcygpO1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICAgIHRoaXMub2JzZXJ2ZVRhcmdldHMoKTtcbiAgICBpZiAodGhpcy5maXJzdFRyZWVpdGVtKSB7XG4gICAgICB0aGlzLmZpcnN0VHJlZWl0ZW0uZWwudGFiSW5kZXggPSAwO1xuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgb2JzZXJ2ZVRhcmdldHMoKSB7XG4gICAgdGhpcy5hZGRPYnNlcnZlcih0cmVlaXRlbSA9PiB7XG4gICAgICB0aGlzLmV4cGFuZFRyZWVpdGVtKHRyZWVpdGVtKTtcbiAgICAgIHRoaXMuc2V0U2VsZWN0ZWQodHJlZWl0ZW0pO1xuICAgICAgLy8gVE9ETzogRml4IHNjcm9sbCBpc3N1ZSBpbiBodHRwczovL2dvbGFuZy5vcmcvaXNzdWUvNDc0NTAuXG4gICAgICAvLyB0cmVlaXRlbS5lbC5zY3JvbGxJbnRvVmlldyh7IGJsb2NrOiAnbmVhcmVzdCcgfSk7XG4gICAgfSk7XG5cbiAgICBjb25zdCB0YXJnZXRzID0gbmV3IE1hcDxzdHJpbmcsIGJvb2xlYW4+KCk7XG4gICAgY29uc3Qgb2JzZXJ2ZXIgPSBuZXcgSW50ZXJzZWN0aW9uT2JzZXJ2ZXIoXG4gICAgICBlbnRyaWVzID0+IHtcbiAgICAgICAgZm9yIChjb25zdCBlbnRyeSBvZiBlbnRyaWVzKSB7XG4gICAgICAgICAgdGFyZ2V0cy5zZXQoZW50cnkudGFyZ2V0LmlkLCBlbnRyeS5pc0ludGVyc2VjdGluZyB8fCBlbnRyeS5pbnRlcnNlY3Rpb25SYXRpbyA9PT0gMSk7XG4gICAgICAgIH1cbiAgICAgICAgZm9yIChjb25zdCBbaWQsIGlzSW50ZXJzZWN0aW5nXSBvZiB0YXJnZXRzKSB7XG4gICAgICAgICAgaWYgKGlzSW50ZXJzZWN0aW5nKSB7XG4gICAgICAgICAgICBjb25zdCBhY3RpdmUgPSB0aGlzLnRyZWVpdGVtcy5maW5kKHQgPT5cbiAgICAgICAgICAgICAgKHQuZWwgYXMgSFRNTEFuY2hvckVsZW1lbnQpPy5ocmVmLmVuZHNXaXRoKGAjJHtpZH1gKVxuICAgICAgICAgICAgKTtcbiAgICAgICAgICAgIGlmIChhY3RpdmUpIHtcbiAgICAgICAgICAgICAgZm9yIChjb25zdCBmbiBvZiB0aGlzLm9ic2VydmVyQ2FsbGJhY2tzKSB7XG4gICAgICAgICAgICAgICAgZm4oYWN0aXZlKTtcbiAgICAgICAgICAgICAgfVxuICAgICAgICAgICAgfVxuICAgICAgICAgICAgYnJlYWs7XG4gICAgICAgICAgfVxuICAgICAgICB9XG4gICAgICB9LFxuICAgICAge1xuICAgICAgICB0aHJlc2hvbGQ6IDEuMCxcbiAgICAgICAgcm9vdE1hcmdpbjogJy02MHB4IDBweCAwcHggMHB4JyxcbiAgICAgIH1cbiAgICApO1xuXG4gICAgZm9yIChjb25zdCBocmVmIG9mIHRoaXMudHJlZWl0ZW1zLm1hcCh0ID0+IHQuZWwuZ2V0QXR0cmlidXRlKCdocmVmJykpKSB7XG4gICAgICBpZiAoaHJlZikge1xuICAgICAgICBjb25zdCBpZCA9IGhyZWYucmVwbGFjZSh3aW5kb3cubG9jYXRpb24ub3JpZ2luLCAnJykucmVwbGFjZSgnLycsICcnKS5yZXBsYWNlKCcjJywgJycpO1xuICAgICAgICBjb25zdCB0YXJnZXQgPSBkb2N1bWVudC5nZXRFbGVtZW50QnlJZChpZCk7XG4gICAgICAgIGlmICh0YXJnZXQpIHtcbiAgICAgICAgICBvYnNlcnZlci5vYnNlcnZlKHRhcmdldCk7XG4gICAgICAgIH1cbiAgICAgIH1cbiAgICB9XG4gIH1cblxuICBhZGRPYnNlcnZlcihmbjogKHQ6IFRyZWVJdGVtKSA9PiB2b2lkLCBkZWxheSA9IDIwMCk6IHZvaWQge1xuICAgIHRoaXMub2JzZXJ2ZXJDYWxsYmFja3MucHVzaChkZWJvdW5jZShmbiwgZGVsYXkpKTtcbiAgfVxuXG4gIHNldEZvY3VzVG9OZXh0SXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBsZXQgbmV4dEl0ZW0gPSBudWxsO1xuICAgIGZvciAobGV0IGkgPSBjdXJyZW50SXRlbS5pbmRleCArIDE7IGkgPCB0aGlzLnRyZWVpdGVtcy5sZW5ndGg7IGkrKykge1xuICAgICAgY29uc3QgdGkgPSB0aGlzLnRyZWVpdGVtc1tpXTtcbiAgICAgIGlmICh0aS5pc1Zpc2libGUpIHtcbiAgICAgICAgbmV4dEl0ZW0gPSB0aTtcbiAgICAgICAgYnJlYWs7XG4gICAgICB9XG4gICAgfVxuICAgIGlmIChuZXh0SXRlbSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbShuZXh0SXRlbSk7XG4gICAgfVxuICB9XG5cbiAgc2V0Rm9jdXNUb1ByZXZpb3VzSXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBsZXQgcHJldkl0ZW0gPSBudWxsO1xuICAgIGZvciAobGV0IGkgPSBjdXJyZW50SXRlbS5pbmRleCAtIDE7IGkgPiAtMTsgaS0tKSB7XG4gICAgICBjb25zdCB0aSA9IHRoaXMudHJlZWl0ZW1zW2ldO1xuICAgICAgaWYgKHRpLmlzVmlzaWJsZSkge1xuICAgICAgICBwcmV2SXRlbSA9IHRpO1xuICAgICAgICBicmVhaztcbiAgICAgIH1cbiAgICB9XG4gICAgaWYgKHByZXZJdGVtKSB7XG4gICAgICB0aGlzLnNldEZvY3VzVG9JdGVtKHByZXZJdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c1RvUGFyZW50SXRlbShjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBpZiAoY3VycmVudEl0ZW0uZ3JvdXBUcmVlaXRlbSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbShjdXJyZW50SXRlbS5ncm91cFRyZWVpdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c1RvRmlyc3RJdGVtKCk6IHZvaWQge1xuICAgIHRoaXMuZmlyc3RUcmVlaXRlbSAmJiB0aGlzLnNldEZvY3VzVG9JdGVtKHRoaXMuZmlyc3RUcmVlaXRlbSk7XG4gIH1cblxuICBzZXRGb2N1c1RvTGFzdEl0ZW0oKTogdm9pZCB7XG4gICAgdGhpcy5sYXN0VHJlZWl0ZW0gJiYgdGhpcy5zZXRGb2N1c1RvSXRlbSh0aGlzLmxhc3RUcmVlaXRlbSk7XG4gIH1cblxuICBzZXRTZWxlY3RlZChjdXJyZW50SXRlbTogVHJlZUl0ZW0pOiB2b2lkIHtcbiAgICBmb3IgKGNvbnN0IGwxIG9mIHRoaXMuZWwucXVlcnlTZWxlY3RvckFsbCgnW2FyaWEtZXhwYW5kZWQ9XCJ0cnVlXCJdJykpIHtcbiAgICAgIGlmIChsMSA9PT0gY3VycmVudEl0ZW0uZWwpIGNvbnRpbnVlO1xuICAgICAgaWYgKCFsMS5uZXh0RWxlbWVudFNpYmxpbmc/LmNvbnRhaW5zKGN1cnJlbnRJdGVtLmVsKSkge1xuICAgICAgICBsMS5zZXRBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnLCAnZmFsc2UnKTtcbiAgICAgIH1cbiAgICB9XG4gICAgZm9yIChjb25zdCBsMSBvZiB0aGlzLmVsLnF1ZXJ5U2VsZWN0b3JBbGwoJ1thcmlhLXNlbGVjdGVkXScpKSB7XG4gICAgICBpZiAobDEgIT09IGN1cnJlbnRJdGVtLmVsKSB7XG4gICAgICAgIGwxLnNldEF0dHJpYnV0ZSgnYXJpYS1zZWxlY3RlZCcsICdmYWxzZScpO1xuICAgICAgfVxuICAgIH1cbiAgICBjdXJyZW50SXRlbS5lbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtc2VsZWN0ZWQnLCAndHJ1ZScpO1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICAgIHRoaXMuc2V0Rm9jdXNUb0l0ZW0oY3VycmVudEl0ZW0sIGZhbHNlKTtcbiAgfVxuXG4gIGV4cGFuZFRyZWVpdGVtKHRyZWVpdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBjdXJyZW50SXRlbTogVHJlZUl0ZW0gfCBudWxsID0gdHJlZWl0ZW07XG4gICAgd2hpbGUgKGN1cnJlbnRJdGVtKSB7XG4gICAgICBpZiAoY3VycmVudEl0ZW0uaXNFeHBhbmRhYmxlKSB7XG4gICAgICAgIGN1cnJlbnRJdGVtLmVsLnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICd0cnVlJyk7XG4gICAgICB9XG4gICAgICBjdXJyZW50SXRlbSA9IGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW07XG4gICAgfVxuICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICB9XG5cbiAgZXhwYW5kQWxsU2libGluZ0l0ZW1zKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGZvciAoY29uc3QgdGkgb2YgdGhpcy50cmVlaXRlbXMpIHtcbiAgICAgIGlmICh0aS5ncm91cFRyZWVpdGVtID09PSBjdXJyZW50SXRlbS5ncm91cFRyZWVpdGVtICYmIHRpLmlzRXhwYW5kYWJsZSkge1xuICAgICAgICB0aGlzLmV4cGFuZFRyZWVpdGVtKHRpKTtcbiAgICAgIH1cbiAgICB9XG4gIH1cblxuICBjb2xsYXBzZVRyZWVpdGVtKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBncm91cFRyZWVpdGVtID0gbnVsbDtcblxuICAgIGlmIChjdXJyZW50SXRlbS5pc0V4cGFuZGVkKCkpIHtcbiAgICAgIGdyb3VwVHJlZWl0ZW0gPSBjdXJyZW50SXRlbTtcbiAgICB9IGVsc2Uge1xuICAgICAgZ3JvdXBUcmVlaXRlbSA9IGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW07XG4gICAgfVxuXG4gICAgaWYgKGdyb3VwVHJlZWl0ZW0pIHtcbiAgICAgIGdyb3VwVHJlZWl0ZW0uZWwuc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgJ2ZhbHNlJyk7XG4gICAgICB0aGlzLnVwZGF0ZVZpc2libGVUcmVlaXRlbXMoKTtcbiAgICAgIHRoaXMuc2V0Rm9jdXNUb0l0ZW0oZ3JvdXBUcmVlaXRlbSk7XG4gICAgfVxuICB9XG5cbiAgc2V0Rm9jdXNCeUZpcnN0Q2hhcmFjdGVyKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSwgY2hhcjogc3RyaW5nKTogdm9pZCB7XG4gICAgbGV0IHN0YXJ0OiBudW1iZXIsIGluZGV4OiBudW1iZXI7XG4gICAgY2hhciA9IGNoYXIudG9Mb3dlckNhc2UoKTtcblxuICAgIC8vIEdldCBzdGFydCBpbmRleCBmb3Igc2VhcmNoIGJhc2VkIG9uIHBvc2l0aW9uIG9mIGN1cnJlbnRJdGVtXG4gICAgc3RhcnQgPSBjdXJyZW50SXRlbS5pbmRleCArIDE7XG4gICAgaWYgKHN0YXJ0ID09PSB0aGlzLnRyZWVpdGVtcy5sZW5ndGgpIHtcbiAgICAgIHN0YXJ0ID0gMDtcbiAgICB9XG5cbiAgICAvLyBDaGVjayByZW1haW5pbmcgc2xvdHMgaW4gdGhlIG1lbnVcbiAgICBpbmRleCA9IHRoaXMuZ2V0SW5kZXhGaXJzdENoYXJzKHN0YXJ0LCBjaGFyKTtcblxuICAgIC8vIElmIG5vdCBmb3VuZCBpbiByZW1haW5pbmcgc2xvdHMsIGNoZWNrIGZyb20gYmVnaW5uaW5nXG4gICAgaWYgKGluZGV4ID09PSAtMSkge1xuICAgICAgaW5kZXggPSB0aGlzLmdldEluZGV4Rmlyc3RDaGFycygwLCBjaGFyKTtcbiAgICB9XG5cbiAgICAvLyBJZiBtYXRjaCB3YXMgZm91bmQuLi5cbiAgICBpZiAoaW5kZXggPiAtMSkge1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbSh0aGlzLnRyZWVpdGVtc1tpbmRleF0pO1xuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgZmluZFRyZWVJdGVtcygpIHtcbiAgICBjb25zdCBmaW5kSXRlbXMgPSAoZWw6IEhUTUxFbGVtZW50LCBncm91cDogVHJlZUl0ZW0gfCBudWxsKSA9PiB7XG4gICAgICBsZXQgdGkgPSBncm91cDtcbiAgICAgIGxldCBjdXJyID0gZWwuZmlyc3RFbGVtZW50Q2hpbGQgYXMgSFRNTEVsZW1lbnQ7XG4gICAgICB3aGlsZSAoY3Vycikge1xuICAgICAgICBpZiAoY3Vyci50YWdOYW1lID09PSAnQScgfHwgY3Vyci50YWdOYW1lID09PSAnU1BBTicpIHtcbiAgICAgICAgICB0aSA9IG5ldyBUcmVlSXRlbShjdXJyLCB0aGlzLCBncm91cCk7XG4gICAgICAgICAgdGhpcy50cmVlaXRlbXMucHVzaCh0aSk7XG4gICAgICAgICAgdGhpcy5maXJzdENoYXJzLnB1c2godGkubGFiZWwuc3Vic3RyaW5nKDAsIDEpLnRvTG93ZXJDYXNlKCkpO1xuICAgICAgICB9XG4gICAgICAgIGlmIChjdXJyLmZpcnN0RWxlbWVudENoaWxkKSB7XG4gICAgICAgICAgZmluZEl0ZW1zKGN1cnIsIHRpKTtcbiAgICAgICAgfVxuICAgICAgICBjdXJyID0gY3Vyci5uZXh0RWxlbWVudFNpYmxpbmcgYXMgSFRNTEVsZW1lbnQ7XG4gICAgICB9XG4gICAgfTtcbiAgICBmaW5kSXRlbXModGhpcy5lbCBhcyBIVE1MRWxlbWVudCwgbnVsbCk7XG4gICAgdGhpcy50cmVlaXRlbXMubWFwKCh0aSwgaWR4KSA9PiAodGkuaW5kZXggPSBpZHgpKTtcbiAgfVxuXG4gIHByaXZhdGUgdXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpOiB2b2lkIHtcbiAgICB0aGlzLmZpcnN0VHJlZWl0ZW0gPSB0aGlzLnRyZWVpdGVtc1swXTtcblxuICAgIGZvciAoY29uc3QgdGkgb2YgdGhpcy50cmVlaXRlbXMpIHtcbiAgICAgIGxldCBwYXJlbnQgPSB0aS5ncm91cFRyZWVpdGVtO1xuICAgICAgdGkuaXNWaXNpYmxlID0gdHJ1ZTtcbiAgICAgIHdoaWxlIChwYXJlbnQgJiYgcGFyZW50LmVsICE9PSB0aGlzLmVsKSB7XG4gICAgICAgIGlmICghcGFyZW50LmlzRXhwYW5kZWQoKSkge1xuICAgICAgICAgIHRpLmlzVmlzaWJsZSA9IGZhbHNlO1xuICAgICAgICB9XG4gICAgICAgIHBhcmVudCA9IHBhcmVudC5ncm91cFRyZWVpdGVtO1xuICAgICAgfVxuICAgICAgaWYgKHRpLmlzVmlzaWJsZSkge1xuICAgICAgICB0aGlzLmxhc3RUcmVlaXRlbSA9IHRpO1xuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgc2V0Rm9jdXNUb0l0ZW0odHJlZWl0ZW06IFRyZWVJdGVtLCBmb2N1c0VsID0gdHJ1ZSkge1xuICAgIHRyZWVpdGVtLmVsLnRhYkluZGV4ID0gMDtcbiAgICBpZiAoZm9jdXNFbCkge1xuICAgICAgdHJlZWl0ZW0uZWwuZm9jdXMoKTtcbiAgICB9XG4gICAgZm9yIChjb25zdCB0aSBvZiB0aGlzLnRyZWVpdGVtcykge1xuICAgICAgaWYgKHRpICE9PSB0cmVlaXRlbSkge1xuICAgICAgICB0aS5lbC50YWJJbmRleCA9IC0xO1xuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgZ2V0SW5kZXhGaXJzdENoYXJzKHN0YXJ0SW5kZXg6IG51bWJlciwgY2hhcjogc3RyaW5nKTogbnVtYmVyIHtcbiAgICBmb3IgKGxldCBpID0gc3RhcnRJbmRleDsgaSA8IHRoaXMuZmlyc3RDaGFycy5sZW5ndGg7IGkrKykge1xuICAgICAgaWYgKHRoaXMudHJlZWl0ZW1zW2ldLmlzVmlzaWJsZSAmJiBjaGFyID09PSB0aGlzLmZpcnN0Q2hhcnNbaV0pIHtcbiAgICAgICAgcmV0dXJuIGk7XG4gICAgICB9XG4gICAgfVxuICAgIHJldHVybiAtMTtcbiAgfVxufVxuXG5jbGFzcyBUcmVlSXRlbSB7XG4gIGVsOiBIVE1MRWxlbWVudDtcbiAgZ3JvdXBUcmVlaXRlbTogVHJlZUl0ZW0gfCBudWxsO1xuICBsYWJlbDogc3RyaW5nO1xuICBpc0V4cGFuZGFibGU6IGJvb2xlYW47XG4gIGlzVmlzaWJsZTogYm9vbGVhbjtcbiAgZGVwdGg6IG51bWJlcjtcbiAgaW5kZXg6IG51bWJlcjtcblxuICBwcml2YXRlIHRyZWU6IFRyZWVOYXZDb250cm9sbGVyO1xuICBwcml2YXRlIGlzSW5Hcm91cDogYm9vbGVhbjtcblxuICBjb25zdHJ1Y3RvcihlbDogSFRNTEVsZW1lbnQsIHRyZWVPYmo6IFRyZWVOYXZDb250cm9sbGVyLCBncm91cDogVHJlZUl0ZW0gfCBudWxsKSB7XG4gICAgZWwudGFiSW5kZXggPSAtMTtcbiAgICB0aGlzLmVsID0gZWw7XG4gICAgdGhpcy5ncm91cFRyZWVpdGVtID0gZ3JvdXA7XG4gICAgdGhpcy5sYWJlbCA9IGVsLnRleHRDb250ZW50Py50cmltKCkgPz8gJyc7XG4gICAgdGhpcy50cmVlID0gdHJlZU9iajtcbiAgICB0aGlzLmRlcHRoID0gKGdyb3VwPy5kZXB0aCB8fCAwKSArIDE7XG4gICAgdGhpcy5pbmRleCA9IDA7XG5cbiAgICBjb25zdCBwYXJlbnQgPSBlbC5wYXJlbnRFbGVtZW50O1xuICAgIGlmIChwYXJlbnQ/LnRhZ05hbWUudG9Mb3dlckNhc2UoKSA9PT0gJ2xpJykge1xuICAgICAgcGFyZW50Py5zZXRBdHRyaWJ1dGUoJ3JvbGUnLCAnbm9uZScpO1xuICAgIH1cbiAgICBlbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtbGV2ZWwnLCB0aGlzLmRlcHRoICsgJycpO1xuICAgIGlmIChlbC5nZXRBdHRyaWJ1dGUoJ2FyaWEtbGFiZWwnKSkge1xuICAgICAgdGhpcy5sYWJlbCA9IGVsPy5nZXRBdHRyaWJ1dGUoJ2FyaWEtbGFiZWwnKT8udHJpbSgpID8/ICcnO1xuICAgIH1cblxuICAgIHRoaXMuaXNFeHBhbmRhYmxlID0gZmFsc2U7XG4gICAgdGhpcy5pc1Zpc2libGUgPSBmYWxzZTtcbiAgICB0aGlzLmlzSW5Hcm91cCA9ICEhZ3JvdXA7XG5cbiAgICBsZXQgY3VyciA9IGVsLm5leHRFbGVtZW50U2libGluZztcbiAgICB3aGlsZSAoY3Vycikge1xuICAgICAgaWYgKGN1cnIudGFnTmFtZS50b0xvd2VyQ2FzZSgpID09ICd1bCcpIHtcbiAgICAgICAgY29uc3QgZ3JvdXBJZCA9IGAke2dyb3VwPy5sYWJlbCA/PyAnJ30gbmF2IGdyb3VwICR7dGhpcy5sYWJlbH1gLnJlcGxhY2UoL1tcXFdfXSsvZywgJ18nKTtcbiAgICAgICAgZWwuc2V0QXR0cmlidXRlKCdhcmlhLW93bnMnLCBncm91cElkKTtcbiAgICAgICAgZWwuc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgJ2ZhbHNlJyk7XG4gICAgICAgIGN1cnIuc2V0QXR0cmlidXRlKCdyb2xlJywgJ2dyb3VwJyk7XG4gICAgICAgIGN1cnIuc2V0QXR0cmlidXRlKCdpZCcsIGdyb3VwSWQpO1xuICAgICAgICB0aGlzLmlzRXhwYW5kYWJsZSA9IHRydWU7XG4gICAgICAgIGJyZWFrO1xuICAgICAgfVxuXG4gICAgICBjdXJyID0gY3Vyci5uZXh0RWxlbWVudFNpYmxpbmc7XG4gICAgfVxuICAgIHRoaXMuaW5pdCgpO1xuICB9XG5cbiAgcHJpdmF0ZSBpbml0KCkge1xuICAgIHRoaXMuZWwudGFiSW5kZXggPSAtMTtcbiAgICBpZiAoIXRoaXMuZWwuZ2V0QXR0cmlidXRlKCdyb2xlJykpIHtcbiAgICAgIHRoaXMuZWwuc2V0QXR0cmlidXRlKCdyb2xlJywgJ3RyZWVpdGVtJyk7XG4gICAgfVxuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcigna2V5ZG93bicsIHRoaXMuaGFuZGxlS2V5ZG93bi5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgdGhpcy5oYW5kbGVDbGljay5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2ZvY3VzJywgdGhpcy5oYW5kbGVGb2N1cy5iaW5kKHRoaXMpKTtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2JsdXInLCB0aGlzLmhhbmRsZUJsdXIuYmluZCh0aGlzKSk7XG4gIH1cblxuICBpc0V4cGFuZGVkKCkge1xuICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgcmV0dXJuIHRoaXMuZWwuZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICd0cnVlJztcbiAgICB9XG5cbiAgICByZXR1cm4gZmFsc2U7XG4gIH1cblxuICBpc1NlbGVjdGVkKCkge1xuICAgIHJldHVybiB0aGlzLmVsLmdldEF0dHJpYnV0ZSgnYXJpYS1zZWxlY3RlZCcpID09PSAndHJ1ZSc7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUNsaWNrKGV2ZW50OiBNb3VzZUV2ZW50KSB7XG4gICAgLy8gb25seSBwcm9jZXNzIGNsaWNrIGV2ZW50cyB0aGF0IGRpcmVjdGx5IGhhcHBlbmVkIG9uIHRoaXMgdHJlZWl0ZW1cbiAgICBpZiAoZXZlbnQudGFyZ2V0ICE9PSB0aGlzLmVsICYmIGV2ZW50LnRhcmdldCAhPT0gdGhpcy5lbC5maXJzdEVsZW1lbnRDaGlsZCkge1xuICAgICAgcmV0dXJuO1xuICAgIH1cbiAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgIGlmICh0aGlzLmlzRXhwYW5kZWQoKSAmJiB0aGlzLmlzU2VsZWN0ZWQoKSkge1xuICAgICAgICB0aGlzLnRyZWUuY29sbGFwc2VUcmVlaXRlbSh0aGlzKTtcbiAgICAgIH0gZWxzZSB7XG4gICAgICAgIHRoaXMudHJlZS5leHBhbmRUcmVlaXRlbSh0aGlzKTtcbiAgICAgIH1cbiAgICAgIGV2ZW50LnN0b3BQcm9wYWdhdGlvbigpO1xuICAgIH1cbiAgICB0aGlzLnRyZWUuc2V0U2VsZWN0ZWQodGhpcyk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUZvY3VzKCkge1xuICAgIGxldCBlbCA9IHRoaXMuZWw7XG4gICAgaWYgKHRoaXMuaXNFeHBhbmRhYmxlKSB7XG4gICAgICBlbCA9IChlbC5maXJzdEVsZW1lbnRDaGlsZCBhcyBIVE1MRWxlbWVudCkgPz8gZWw7XG4gICAgfVxuICAgIGVsLmNsYXNzTGlzdC5hZGQoJ2ZvY3VzJyk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZUJsdXIoKSB7XG4gICAgbGV0IGVsID0gdGhpcy5lbDtcbiAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgIGVsID0gKGVsLmZpcnN0RWxlbWVudENoaWxkIGFzIEhUTUxFbGVtZW50KSA/PyBlbDtcbiAgICB9XG4gICAgZWwuY2xhc3NMaXN0LnJlbW92ZSgnZm9jdXMnKTtcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlS2V5ZG93bihldmVudDogS2V5Ym9hcmRFdmVudCkge1xuICAgIGlmIChldmVudC5hbHRLZXkgfHwgZXZlbnQuY3RybEtleSB8fCBldmVudC5tZXRhS2V5KSB7XG4gICAgICByZXR1cm47XG4gICAgfVxuXG4gICAgbGV0IGNhcHR1cmVkID0gZmFsc2U7XG4gICAgc3dpdGNoIChldmVudC5rZXkpIHtcbiAgICAgIGNhc2UgJyAnOlxuICAgICAgY2FzZSAnRW50ZXInOlxuICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGVkKCkgJiYgdGhpcy5pc1NlbGVjdGVkKCkpIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5jb2xsYXBzZVRyZWVpdGVtKHRoaXMpO1xuICAgICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuZXhwYW5kVHJlZWl0ZW0odGhpcyk7XG4gICAgICAgICAgfVxuICAgICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICBldmVudC5zdG9wUHJvcGFnYXRpb24oKTtcbiAgICAgICAgfVxuICAgICAgICB0aGlzLnRyZWUuc2V0U2VsZWN0ZWQodGhpcyk7XG4gICAgICAgIGJyZWFrO1xuXG4gICAgICBjYXNlICdBcnJvd1VwJzpcbiAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9QcmV2aW91c0l0ZW0odGhpcyk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0Fycm93RG93bic6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTmV4dEl0ZW0odGhpcyk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0Fycm93UmlnaHQnOlxuICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGVkKCkpIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTmV4dEl0ZW0odGhpcyk7XG4gICAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5leHBhbmRUcmVlaXRlbSh0aGlzKTtcbiAgICAgICAgICB9XG4gICAgICAgIH1cbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnQXJyb3dMZWZ0JzpcbiAgICAgICAgaWYgKHRoaXMuaXNFeHBhbmRhYmxlICYmIHRoaXMuaXNFeHBhbmRlZCgpKSB7XG4gICAgICAgICAgdGhpcy50cmVlLmNvbGxhcHNlVHJlZWl0ZW0odGhpcyk7XG4gICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgIGlmICh0aGlzLmlzSW5Hcm91cCkge1xuICAgICAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9QYXJlbnRJdGVtKHRoaXMpO1xuICAgICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICAgIH1cbiAgICAgICAgfVxuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnSG9tZSc6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvRmlyc3RJdGVtKCk7XG4gICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0VuZCc6XG4gICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c1RvTGFzdEl0ZW0oKTtcbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgZGVmYXVsdDpcbiAgICAgICAgaWYgKGV2ZW50LmtleS5sZW5ndGggPT09IDEgJiYgZXZlbnQua2V5Lm1hdGNoKC9cXFMvKSkge1xuICAgICAgICAgIGlmIChldmVudC5rZXkgPT0gJyonKSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuZXhwYW5kQWxsU2libGluZ0l0ZW1zKHRoaXMpO1xuICAgICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuc2V0Rm9jdXNCeUZpcnN0Q2hhcmFjdGVyKHRoaXMsIGV2ZW50LmtleSk7XG4gICAgICAgICAgfVxuICAgICAgICAgIGNhcHR1cmVkID0gdHJ1ZTtcbiAgICAgICAgfVxuICAgICAgICBicmVhaztcbiAgICB9XG5cbiAgICBpZiAoY2FwdHVyZWQpIHtcbiAgICAgIGV2ZW50LnN0b3BQcm9wYWdhdGlvbigpO1xuICAgICAgZXZlbnQucHJldmVudERlZmF1bHQoKTtcbiAgICB9XG4gIH1cbn1cblxuLy8gZXNsaW50LWRpc2FibGUtbmV4dC1saW5lIEB0eXBlc2NyaXB0LWVzbGludC9uby1leHBsaWNpdC1hbnlcbmZ1bmN0aW9uIGRlYm91bmNlPFQgZXh0ZW5kcyAoLi4uYXJnczogYW55W10pID0+IGFueT4oZnVuYzogVCwgd2FpdDogbnVtYmVyKSB7XG4gIGxldCB0aW1lb3V0OiBSZXR1cm5UeXBlPHR5cGVvZiBzZXRUaW1lb3V0PiB8IG51bGw7XG4gIHJldHVybiAoLi4uYXJnczogUGFyYW1ldGVyczxUPikgPT4ge1xuICAgIGNvbnN0IGxhdGVyID0gKCkgPT4ge1xuICAgICAgdGltZW91dCA9IG51bGw7XG4gICAgICBmdW5jKC4uLmFyZ3MpO1xuICAgIH07XG4gICAgaWYgKHRpbWVvdXQpIHtcbiAgICAgIGNsZWFyVGltZW91dCh0aW1lb3V0KTtcbiAgICB9XG4gICAgdGltZW91dCA9IHNldFRpbWVvdXQobGF0ZXIsIHdhaXQpO1xuICB9O1xufVxuIiwgIi8qKlxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG5pbXBvcnQgeyBTZWxlY3ROYXZDb250cm9sbGVyLCBtYWtlU2VsZWN0TmF2IH0gZnJvbSAnLi4vLi4vc2hhcmVkL291dGxpbmUvc2VsZWN0JztcbmltcG9ydCB7IFRyZWVOYXZDb250cm9sbGVyIH0gZnJvbSAnLi4vLi4vc2hhcmVkL291dGxpbmUvdHJlZSc7XG5cbndpbmRvdy5hZGRFdmVudExpc3RlbmVyKCdsb2FkJywgKCkgPT4ge1xuICBjb25zdCB0cmVlID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MRWxlbWVudD4oJy5qcy10cmVlJyk7XG4gIGlmICh0cmVlKSB7XG4gICAgY29uc3QgdHJlZUN0cmwgPSBuZXcgVHJlZU5hdkNvbnRyb2xsZXIodHJlZSk7XG4gICAgY29uc3Qgc2VsZWN0ID0gbWFrZVNlbGVjdE5hdih0cmVlQ3RybCk7XG4gICAgZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLW1haW5OYXZNb2JpbGUnKT8uYXBwZW5kQ2hpbGQoc2VsZWN0KTtcbiAgfVxuXG4gIGNvbnN0IGd1aWRlVHJlZSA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEVsZW1lbnQ+KCcuT3V0bGluZSAuanMtdHJlZScpO1xuICBpZiAoZ3VpZGVUcmVlKSB7XG4gICAgY29uc3QgdHJlZUN0cmwgPSBuZXcgVHJlZU5hdkNvbnRyb2xsZXIoZ3VpZGVUcmVlKTtcbiAgICBjb25zdCBzZWxlY3QgPSBtYWtlU2VsZWN0TmF2KHRyZWVDdHJsKTtcbiAgICBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuT3V0bGluZSAuanMtc2VsZWN0Jyk/LmFwcGVuZENoaWxkKHNlbGVjdCk7XG4gIH1cblxuICBmb3IgKGNvbnN0IGVsIG9mIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwoJy5qcy10b2dnbGVUaGVtZScpKSB7XG4gICAgZWwuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgIGNvbnN0IHZhbHVlID0gKGUuY3VycmVudFRhcmdldCBhcyBIVE1MQnV0dG9uRWxlbWVudCkuZ2V0QXR0cmlidXRlKCdkYXRhLXZhbHVlJyk7XG4gICAgICBkb2N1bWVudC5kb2N1bWVudEVsZW1lbnQuc2V0QXR0cmlidXRlKCdkYXRhLXRoZW1lJywgU3RyaW5nKHZhbHVlKSk7XG4gICAgfSk7XG4gIH1cbiAgZm9yIChjb25zdCBlbCBvZiBkb2N1bWVudC5xdWVyeVNlbGVjdG9yQWxsKCcuanMtdG9nZ2xlTGF5b3V0JykpIHtcbiAgICBlbC5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsIGUgPT4ge1xuICAgICAgY29uc3QgdmFsdWUgPSAoZS5jdXJyZW50VGFyZ2V0IGFzIEhUTUxCdXR0b25FbGVtZW50KS5nZXRBdHRyaWJ1dGUoJ2RhdGEtdmFsdWUnKTtcbiAgICAgIGRvY3VtZW50LmRvY3VtZW50RWxlbWVudC5zZXRBdHRyaWJ1dGUoJ2RhdGEtbGF5b3V0JywgU3RyaW5nKHZhbHVlKSk7XG4gICAgfSk7XG4gIH1cblxuICBmb3IgKGNvbnN0IGVsIG9mIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGw8SFRNTFNlbGVjdEVsZW1lbnQ+KCcuanMtc2VsZWN0TmF2JykpIHtcbiAgICBuZXcgU2VsZWN0TmF2Q29udHJvbGxlcihlbCk7XG4gIH1cbn0pO1xuXG5jdXN0b21FbGVtZW50cy5kZWZpbmUoXG4gICdnby1jb2xvcicsXG4gIGNsYXNzIGV4dGVuZHMgSFRNTEVsZW1lbnQge1xuICAgIGNvbnN0cnVjdG9yKCkge1xuICAgICAgc3VwZXIoKTtcbiAgICAgIHRoaXMuc3R5bGUuc2V0UHJvcGVydHkoJ2Rpc3BsYXknLCAnY29udGVudHMnKTtcbiAgICAgIC8vIFRoZSBjdXJyZW50IHZlcnNpb24gb2YgVHlwZVNjcmlwdCBpcyBub3QgYXdhcmUgb2YgU3RyaW5nLnJlcGxhY2VBbGwuXG4gICAgICAvLyBlc2xpbnQtZGlzYWJsZS1uZXh0LWxpbmUgQHR5cGVzY3JpcHQtZXNsaW50L25vLWV4cGxpY2l0LWFueVxuICAgICAgY29uc3QgbmFtZSA9IHRoaXMuaWQgYXMgYW55O1xuICAgICAgdGhpcy5yZW1vdmVBdHRyaWJ1dGUoJ2lkJyk7XG4gICAgICB0aGlzLmlubmVySFRNTCA9IGBcbiAgICAgICAgPGRpdiBzdHlsZT1cIi0tY29sb3I6IHZhcigke25hbWV9KTtcIiBjbGFzcz1cIkdvQ29sb3ItY2lyY2xlXCI+PC9kaXY+XG4gICAgICAgIDxzcGFuPlxuICAgICAgICAgIDxkaXYgaWQ9XCIke25hbWV9XCIgY2xhc3M9XCJnby10ZXh0TGFiZWwgR29Db2xvci10aXRsZVwiPiR7bmFtZVxuICAgICAgICAucmVwbGFjZSgnLS1jb2xvci0nLCAnJylcbiAgICAgICAgLnJlcGxhY2VBbGwoJy0nLCAnICcpfTwvZGl2PlxuICAgICAgICAgIDxwcmUgY2xhc3M9XCJTdHJpbmdpZnlFbGVtZW50LW1hcmt1cFwiPnZhcigke25hbWV9KTwvcHJlPlxuICAgICAgICA8L3NwYW4+XG4gICAgICBgO1xuICAgICAgdGhpcy5xdWVyeVNlbGVjdG9yKCdwcmUnKT8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICAgIG5hdmlnYXRvci5jbGlwYm9hcmQud3JpdGVUZXh0KGB2YXIoJHtuYW1lfSlgKTtcbiAgICAgIH0pO1xuICAgIH1cbiAgfVxuKTtcblxuY3VzdG9tRWxlbWVudHMuZGVmaW5lKFxuICAnZ28taWNvbicsXG4gIGNsYXNzIGV4dGVuZHMgSFRNTEVsZW1lbnQge1xuICAgIGNvbnN0cnVjdG9yKCkge1xuICAgICAgc3VwZXIoKTtcbiAgICAgIHRoaXMuc3R5bGUuc2V0UHJvcGVydHkoJ2Rpc3BsYXknLCAnY29udGVudHMnKTtcbiAgICAgIC8vIFRoZSBjdXJyZW50IHZlcnNpb24gb2YgVHlwZVNjcmlwdCBpcyBub3QgYXdhcmUgb2YgU3RyaW5nLnJlcGxhY2VBbGwuXG4gICAgICAvLyBlc2xpbnQtZGlzYWJsZS1uZXh0LWxpbmUgQHR5cGVzY3JpcHQtZXNsaW50L25vLWV4cGxpY2l0LWFueVxuICAgICAgY29uc3QgbmFtZSA9IHRoaXMuZ2V0QXR0cmlidXRlKCduYW1lJykgYXMgYW55O1xuICAgICAgdGhpcy5pbm5lckhUTUwgPSBgPHAgaWQ9XCJpY29uLSR7bmFtZX1cIiBjbGFzcz1cImdvLXRleHRMYWJlbCBHb0ljb24tdGl0bGVcIj4ke25hbWUucmVwbGFjZUFsbChcbiAgICAgICAgJ18nLFxuICAgICAgICAnICdcbiAgICAgICl9PC9wPlxuICAgICAgICA8c3RyaW5naWZ5LWVsPlxuICAgICAgICAgIDxpbWcgY2xhc3M9XCJnby1JY29uXCIgaGVpZ2h0PVwiMjRcIiB3aWR0aD1cIjI0XCIgc3JjPVwiL3N0YXRpYy9zaGFyZWQvaWNvbi8ke25hbWV9X2dtX2dyZXlfMjRkcC5zdmdcIiBhbHQ9XCJcIj5cbiAgICAgICAgPC9zdHJpbmdpZnktZWw+XG4gICAgICBgO1xuICAgIH1cbiAgfVxuKTtcblxuY3VzdG9tRWxlbWVudHMuZGVmaW5lKFxuICAnY2xvbmUtZWwnLFxuICBjbGFzcyBleHRlbmRzIEhUTUxFbGVtZW50IHtcbiAgICBjb25zdHJ1Y3RvcigpIHtcbiAgICAgIHN1cGVyKCk7XG4gICAgICB0aGlzLnN0eWxlLnNldFByb3BlcnR5KCdkaXNwbGF5JywgJ2NvbnRlbnRzJyk7XG4gICAgICBjb25zdCBzZWxlY3RvciA9IHRoaXMuZ2V0QXR0cmlidXRlKCdzZWxlY3RvcicpO1xuICAgICAgaWYgKCFzZWxlY3RvcikgcmV0dXJuO1xuICAgICAgY29uc3QgaHRtbCA9ICcgICAgJyArIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3Ioc2VsZWN0b3IpPy5vdXRlckhUTUw7XG4gICAgICB0aGlzLmlubmVySFRNTCA9IGBcbiAgICAgICAgPHN0cmluZ2lmeS1lbCBjb2xsYXBzZWQ+JHtodG1sfTwvc3RyaW5naWZ5LWVsPlxuICAgICAgYDtcbiAgICB9XG4gIH1cbik7XG5cbmN1c3RvbUVsZW1lbnRzLmRlZmluZShcbiAgJ3N0cmluZ2lmeS1lbCcsXG4gIGNsYXNzIGV4dGVuZHMgSFRNTEVsZW1lbnQge1xuICAgIGNvbnN0cnVjdG9yKCkge1xuICAgICAgc3VwZXIoKTtcbiAgICAgIHRoaXMuc3R5bGUuc2V0UHJvcGVydHkoJ2Rpc3BsYXknLCAnY29udGVudHMnKTtcbiAgICAgIGNvbnN0IGh0bWwgPSB0aGlzLmlubmVySFRNTDtcbiAgICAgIGNvbnN0IGlkQXR0ciA9IHRoaXMuaWQgPyBgIGlkPVwiJHt0aGlzLmlkfVwiYCA6ICcnO1xuICAgICAgdGhpcy5yZW1vdmVBdHRyaWJ1dGUoJ2lkJyk7XG4gICAgICBsZXQgbWFya3VwID0gYDxwcmUgY2xhc3M9XCJTdHJpbmdpZnlFbGVtZW50LW1hcmt1cFwiPmAgKyBlc2NhcGUodHJpbShodG1sKSkgKyBgPC9wcmU+YDtcbiAgICAgIGlmICh0aGlzLmhhc0F0dHJpYnV0ZSgnY29sbGFwc2VkJykpIHtcbiAgICAgICAgbWFya3VwID0gYDxkZXRhaWxzIGNsYXNzPVwiU3RyaW5naWZ5RWxlbWVudC1kZXRhaWxzXCI+PHN1bW1hcnk+TWFya3VwPC9zdW1tYXJ5PiR7bWFya3VwfTwvZGV0YWlscz5gO1xuICAgICAgfVxuICAgICAgdGhpcy5pbm5lckhUTUwgPSBgPHNwYW4ke2lkQXR0cn0+JHtodG1sfTwvc3Bhbj4ke21hcmt1cH1gO1xuICAgICAgdGhpcy5xdWVyeVNlbGVjdG9yKCdwcmUnKT8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICAgIG5hdmlnYXRvci5jbGlwYm9hcmQud3JpdGVUZXh0KGh0bWwpO1xuICAgICAgfSk7XG4gICAgfVxuICB9XG4pO1xuXG4vKipcbiAqIHRyaW0gcmVtb3ZlcyBleGNlc3MgaW5kZW50YXRpb24gZnJvbSBodG1sIG1hcmt1cCBieVxuICogbWVhc3VyaW5nIHRoZSBudW1iZXIgb2Ygc3BhY2VzIGluIHRoZSBmaXJzdCBsaW5lIG9mXG4gKiB0aGUgZ2l2ZW4gc3RyaW5nIGFuZCByZW1vdmluZyB0aGF0IG51bWJlciBvZiBzcGFjZXNcbiAqIGZyb20gdGhlIGJlZ2lubmluZyBvZiBlYWNoIGxpbmUuXG4gKi9cbmZ1bmN0aW9uIHRyaW0oaHRtbDogc3RyaW5nKSB7XG4gIHJldHVybiBodG1sXG4gICAgLnNwbGl0KCdcXG4nKVxuICAgIC5yZWR1Y2U8eyByZXN1bHQ6IHN0cmluZ1tdOyBzdGFydDogbnVtYmVyIH0+KFxuICAgICAgKGFjYywgdmFsKSA9PiB7XG4gICAgICAgIGlmIChhY2MucmVzdWx0Lmxlbmd0aCA9PT0gMCkge1xuICAgICAgICAgIGNvbnN0IHN0YXJ0ID0gdmFsLmluZGV4T2YoJzwnKTtcbiAgICAgICAgICBhY2Muc3RhcnQgPSBzdGFydCA9PT0gLTEgPyAwIDogc3RhcnQ7XG4gICAgICAgIH1cbiAgICAgICAgdmFsID0gdmFsLnNsaWNlKGFjYy5zdGFydCk7XG4gICAgICAgIGlmICh2YWwpIHtcbiAgICAgICAgICBhY2MucmVzdWx0LnB1c2godmFsKTtcbiAgICAgICAgfVxuICAgICAgICByZXR1cm4gYWNjO1xuICAgICAgfSxcbiAgICAgIHsgcmVzdWx0OiBbXSwgc3RhcnQ6IDAgfVxuICAgIClcbiAgICAucmVzdWx0LmpvaW4oJ1xcbicpO1xufVxuXG5mdW5jdGlvbiBlc2NhcGUoaHRtbDogc3RyaW5nKSB7XG4gIC8vIFRoZSBjdXJyZW50IHZlcnNpb24gb2YgVHlwZVNjcmlwdCBpcyBub3QgYXdhcmUgb2YgU3RyaW5nLnJlcGxhY2VBbGwuXG4gIC8vIGVzbGludC1kaXNhYmxlLW5leHQtbGluZSBAdHlwZXNjcmlwdC1lc2xpbnQvbm8tZXhwbGljaXQtYW55XG4gIHJldHVybiAoaHRtbCBhcyBhbnkpPy5yZXBsYWNlQWxsKCc8JywgJyZsdDsnKT8ucmVwbGFjZUFsbCgnPicsICcmZ3Q7Jyk7XG59XG4iXSwKICAibWFwcGluZ3MiOiAiOztBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQVNPLGtDQUEwQjtBQUFBLElBQy9CLFlBQW9CLElBQWE7QUFBYjtBQUNsQixXQUFLLEdBQUcsaUJBQWlCLFVBQVUsT0FBSztBQUN0QyxjQUFNLFNBQVMsRUFBRTtBQUNqQixZQUFJLE9BQU8sT0FBTztBQUNsQixZQUFJLENBQUMsT0FBTyxNQUFNLFdBQVcsTUFBTTtBQUNqQyxpQkFBTyxNQUFNO0FBQUE7QUFFZixlQUFPLFNBQVMsT0FBTztBQUFBO0FBQUE7QUFBQTtBQUt0Qix5QkFBdUIsTUFBMkM7QUFDdkUsVUFBTSxRQUFRLFNBQVMsY0FBYztBQUNyQyxVQUFNLFVBQVUsSUFBSTtBQUNwQixVQUFNLGFBQWEsY0FBYztBQUNqQyxVQUFNLFNBQVMsU0FBUyxjQUFjO0FBQ3RDLFdBQU8sVUFBVSxJQUFJLGFBQWE7QUFDbEMsVUFBTSxZQUFZO0FBQ2xCLFVBQU0sVUFBVSxTQUFTLGNBQWM7QUFDdkMsWUFBUSxRQUFRO0FBQ2hCLFdBQU8sWUFBWTtBQUNuQixVQUFNLFdBQWdEO0FBQ3RELFFBQUk7QUFDSixlQUFXLEtBQUssS0FBSyxXQUFXO0FBQzlCLFVBQUksT0FBTyxFQUFFLFNBQVM7QUFBRztBQUN6QixVQUFJLEVBQUUsZUFBZTtBQUNuQixnQkFBUSxTQUFTLEVBQUUsY0FBYztBQUNqQyxZQUFJLENBQUMsT0FBTztBQUNWLGtCQUFRLFNBQVMsRUFBRSxjQUFjLFNBQVMsU0FBUyxjQUFjO0FBQ2pFLGdCQUFNLFFBQVEsRUFBRSxjQUFjO0FBQzlCLGlCQUFPLFlBQVk7QUFBQTtBQUFBLGFBRWhCO0FBQ0wsZ0JBQVE7QUFBQTtBQUVWLFlBQU0sSUFBSSxTQUFTLGNBQWM7QUFDakMsUUFBRSxRQUFRLEVBQUU7QUFDWixRQUFFLGNBQWMsRUFBRTtBQUNsQixRQUFFLFFBQVMsRUFBRSxHQUF5QixLQUFLLFFBQVEsT0FBTyxTQUFTLFFBQVEsSUFBSSxRQUFRLEtBQUs7QUFDNUYsWUFBTSxZQUFZO0FBQUE7QUFFcEIsU0FBSyxZQUFZLE9BQUs7QUFDcEIsWUFBTSxPQUFRLEVBQUUsR0FBeUI7QUFDekMsWUFBTSxRQUFRLE9BQU8sY0FBaUMsWUFBWSxXQUFXO0FBQzdFLFVBQUksT0FBTztBQUNULGVBQU8sUUFBUTtBQUFBO0FBQUEsT0FFaEI7QUFDSCxXQUFPO0FBQUE7OztBQzNEVDtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFjTyxnQ0FBd0I7QUFBQSxJQWE3QixZQUFvQixJQUFpQjtBQUFqQjtBQUNsQixXQUFLLFlBQVk7QUFDakIsV0FBSyxhQUFhO0FBQ2xCLFdBQUssZ0JBQWdCO0FBQ3JCLFdBQUssZUFBZTtBQUNwQixXQUFLLG9CQUFvQjtBQUN6QixXQUFLO0FBQUE7QUFBQSxJQUdDLE9BQWE7QUFDbkIsV0FBSyxHQUFHLE1BQU0sWUFBWSxvQkFBb0IsS0FBSyxHQUFHLGVBQWU7QUFDckUsV0FBSztBQUNMLFdBQUs7QUFDTCxXQUFLO0FBQ0wsVUFBSSxLQUFLLGVBQWU7QUFDdEIsYUFBSyxjQUFjLEdBQUcsV0FBVztBQUFBO0FBQUE7QUFBQSxJQUk3QixpQkFBaUI7QUFDdkIsV0FBSyxZQUFZLGNBQVk7QUFDM0IsYUFBSyxlQUFlO0FBQ3BCLGFBQUssWUFBWTtBQUFBO0FBS25CLFlBQU0sVUFBVSxJQUFJO0FBQ3BCLFlBQU0sV0FBVyxJQUFJLHFCQUNuQixhQUFXO0FBQ1QsbUJBQVcsU0FBUyxTQUFTO0FBQzNCLGtCQUFRLElBQUksTUFBTSxPQUFPLElBQUksTUFBTSxrQkFBa0IsTUFBTSxzQkFBc0I7QUFBQTtBQUVuRixtQkFBVyxDQUFDLElBQUksbUJBQW1CLFNBQVM7QUFDMUMsY0FBSSxnQkFBZ0I7QUFDbEIsa0JBQU0sU0FBUyxLQUFLLFVBQVUsS0FBSyxPQUNoQyxFQUFFLElBQTBCLEtBQUssU0FBUyxJQUFJO0FBRWpELGdCQUFJLFFBQVE7QUFDVix5QkFBVyxNQUFNLEtBQUssbUJBQW1CO0FBQ3ZDLG1CQUFHO0FBQUE7QUFBQTtBQUdQO0FBQUE7QUFBQTtBQUFBLFNBSU47QUFBQSxRQUNFLFdBQVc7QUFBQSxRQUNYLFlBQVk7QUFBQTtBQUloQixpQkFBVyxRQUFRLEtBQUssVUFBVSxJQUFJLE9BQUssRUFBRSxHQUFHLGFBQWEsVUFBVTtBQUNyRSxZQUFJLE1BQU07QUFDUixnQkFBTSxLQUFLLEtBQUssUUFBUSxPQUFPLFNBQVMsUUFBUSxJQUFJLFFBQVEsS0FBSyxJQUFJLFFBQVEsS0FBSztBQUNsRixnQkFBTSxTQUFTLFNBQVMsZUFBZTtBQUN2QyxjQUFJLFFBQVE7QUFDVixxQkFBUyxRQUFRO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQSxJQU16QixZQUFZLElBQTJCLFFBQVEsS0FBVztBQUN4RCxXQUFLLGtCQUFrQixLQUFLLFNBQVMsSUFBSTtBQUFBO0FBQUEsSUFHM0MsbUJBQW1CLGFBQTZCO0FBQzlDLFVBQUksV0FBVztBQUNmLGVBQVMsSUFBSSxZQUFZLFFBQVEsR0FBRyxJQUFJLEtBQUssVUFBVSxRQUFRLEtBQUs7QUFDbEUsY0FBTSxLQUFLLEtBQUssVUFBVTtBQUMxQixZQUFJLEdBQUcsV0FBVztBQUNoQixxQkFBVztBQUNYO0FBQUE7QUFBQTtBQUdKLFVBQUksVUFBVTtBQUNaLGFBQUssZUFBZTtBQUFBO0FBQUE7QUFBQSxJQUl4Qix1QkFBdUIsYUFBNkI7QUFDbEQsVUFBSSxXQUFXO0FBQ2YsZUFBUyxJQUFJLFlBQVksUUFBUSxHQUFHLElBQUksSUFBSSxLQUFLO0FBQy9DLGNBQU0sS0FBSyxLQUFLLFVBQVU7QUFDMUIsWUFBSSxHQUFHLFdBQVc7QUFDaEIscUJBQVc7QUFDWDtBQUFBO0FBQUE7QUFHSixVQUFJLFVBQVU7QUFDWixhQUFLLGVBQWU7QUFBQTtBQUFBO0FBQUEsSUFJeEIscUJBQXFCLGFBQTZCO0FBQ2hELFVBQUksWUFBWSxlQUFlO0FBQzdCLGFBQUssZUFBZSxZQUFZO0FBQUE7QUFBQTtBQUFBLElBSXBDLHNCQUE0QjtBQUMxQixXQUFLLGlCQUFpQixLQUFLLGVBQWUsS0FBSztBQUFBO0FBQUEsSUFHakQscUJBQTJCO0FBQ3pCLFdBQUssZ0JBQWdCLEtBQUssZUFBZSxLQUFLO0FBQUE7QUFBQSxJQUdoRCxZQUFZLGFBQTZCO0FBQ3ZDLGlCQUFXLE1BQU0sS0FBSyxHQUFHLGlCQUFpQiwyQkFBMkI7QUFDbkUsWUFBSSxPQUFPLFlBQVk7QUFBSTtBQUMzQixZQUFJLENBQUMsR0FBRyxvQkFBb0IsU0FBUyxZQUFZLEtBQUs7QUFDcEQsYUFBRyxhQUFhLGlCQUFpQjtBQUFBO0FBQUE7QUFHckMsaUJBQVcsTUFBTSxLQUFLLEdBQUcsaUJBQWlCLG9CQUFvQjtBQUM1RCxZQUFJLE9BQU8sWUFBWSxJQUFJO0FBQ3pCLGFBQUcsYUFBYSxpQkFBaUI7QUFBQTtBQUFBO0FBR3JDLGtCQUFZLEdBQUcsYUFBYSxpQkFBaUI7QUFDN0MsV0FBSztBQUNMLFdBQUssZUFBZSxhQUFhO0FBQUE7QUFBQSxJQUduQyxlQUFlLFVBQTBCO0FBQ3ZDLFVBQUksY0FBK0I7QUFDbkMsYUFBTyxhQUFhO0FBQ2xCLFlBQUksWUFBWSxjQUFjO0FBQzVCLHNCQUFZLEdBQUcsYUFBYSxpQkFBaUI7QUFBQTtBQUUvQyxzQkFBYyxZQUFZO0FBQUE7QUFFNUIsV0FBSztBQUFBO0FBQUEsSUFHUCxzQkFBc0IsYUFBNkI7QUFDakQsaUJBQVcsTUFBTSxLQUFLLFdBQVc7QUFDL0IsWUFBSSxHQUFHLGtCQUFrQixZQUFZLGlCQUFpQixHQUFHLGNBQWM7QUFDckUsZUFBSyxlQUFlO0FBQUE7QUFBQTtBQUFBO0FBQUEsSUFLMUIsaUJBQWlCLGFBQTZCO0FBQzVDLFVBQUksZ0JBQWdCO0FBRXBCLFVBQUksWUFBWSxjQUFjO0FBQzVCLHdCQUFnQjtBQUFBLGFBQ1g7QUFDTCx3QkFBZ0IsWUFBWTtBQUFBO0FBRzlCLFVBQUksZUFBZTtBQUNqQixzQkFBYyxHQUFHLGFBQWEsaUJBQWlCO0FBQy9DLGFBQUs7QUFDTCxhQUFLLGVBQWU7QUFBQTtBQUFBO0FBQUEsSUFJeEIseUJBQXlCLGFBQXVCLE1BQW9CO0FBQ2xFLFVBQUksT0FBZTtBQUNuQixhQUFPLEtBQUs7QUFHWixjQUFRLFlBQVksUUFBUTtBQUM1QixVQUFJLFVBQVUsS0FBSyxVQUFVLFFBQVE7QUFDbkMsZ0JBQVE7QUFBQTtBQUlWLGNBQVEsS0FBSyxtQkFBbUIsT0FBTztBQUd2QyxVQUFJLFVBQVUsSUFBSTtBQUNoQixnQkFBUSxLQUFLLG1CQUFtQixHQUFHO0FBQUE7QUFJckMsVUFBSSxRQUFRLElBQUk7QUFDZCxhQUFLLGVBQWUsS0FBSyxVQUFVO0FBQUE7QUFBQTtBQUFBLElBSS9CLGdCQUFnQjtBQUN0QixZQUFNLFlBQVksQ0FBQyxJQUFpQixVQUEyQjtBQUM3RCxZQUFJLEtBQUs7QUFDVCxZQUFJLE9BQU8sR0FBRztBQUNkLGVBQU8sTUFBTTtBQUNYLGNBQUksS0FBSyxZQUFZLE9BQU8sS0FBSyxZQUFZLFFBQVE7QUFDbkQsaUJBQUssSUFBSSxTQUFTLE1BQU0sTUFBTTtBQUM5QixpQkFBSyxVQUFVLEtBQUs7QUFDcEIsaUJBQUssV0FBVyxLQUFLLEdBQUcsTUFBTSxVQUFVLEdBQUcsR0FBRztBQUFBO0FBRWhELGNBQUksS0FBSyxtQkFBbUI7QUFDMUIsc0JBQVUsTUFBTTtBQUFBO0FBRWxCLGlCQUFPLEtBQUs7QUFBQTtBQUFBO0FBR2hCLGdCQUFVLEtBQUssSUFBbUI7QUFDbEMsV0FBSyxVQUFVLElBQUksQ0FBQyxJQUFJLFFBQVMsR0FBRyxRQUFRO0FBQUE7QUFBQSxJQUd0Qyx5QkFBK0I7QUFDckMsV0FBSyxnQkFBZ0IsS0FBSyxVQUFVO0FBRXBDLGlCQUFXLE1BQU0sS0FBSyxXQUFXO0FBQy9CLFlBQUksU0FBUyxHQUFHO0FBQ2hCLFdBQUcsWUFBWTtBQUNmLGVBQU8sVUFBVSxPQUFPLE9BQU8sS0FBSyxJQUFJO0FBQ3RDLGNBQUksQ0FBQyxPQUFPLGNBQWM7QUFDeEIsZUFBRyxZQUFZO0FBQUE7QUFFakIsbUJBQVMsT0FBTztBQUFBO0FBRWxCLFlBQUksR0FBRyxXQUFXO0FBQ2hCLGVBQUssZUFBZTtBQUFBO0FBQUE7QUFBQTtBQUFBLElBS2xCLGVBQWUsVUFBb0IsVUFBVSxNQUFNO0FBQ3pELGVBQVMsR0FBRyxXQUFXO0FBQ3ZCLFVBQUksU0FBUztBQUNYLGlCQUFTLEdBQUc7QUFBQTtBQUVkLGlCQUFXLE1BQU0sS0FBSyxXQUFXO0FBQy9CLFlBQUksT0FBTyxVQUFVO0FBQ25CLGFBQUcsR0FBRyxXQUFXO0FBQUE7QUFBQTtBQUFBO0FBQUEsSUFLZixtQkFBbUIsWUFBb0IsTUFBc0I7QUFDbkUsZUFBUyxJQUFJLFlBQVksSUFBSSxLQUFLLFdBQVcsUUFBUSxLQUFLO0FBQ3hELFlBQUksS0FBSyxVQUFVLEdBQUcsYUFBYSxTQUFTLEtBQUssV0FBVyxJQUFJO0FBQzlELGlCQUFPO0FBQUE7QUFBQTtBQUdYLGFBQU87QUFBQTtBQUFBO0FBSVgsdUJBQWU7QUFBQSxJQVliLFlBQVksSUFBaUIsU0FBNEIsT0FBd0I7QUFDL0UsU0FBRyxXQUFXO0FBQ2QsV0FBSyxLQUFLO0FBQ1YsV0FBSyxnQkFBZ0I7QUFDckIsV0FBSyxRQUFRLEdBQUcsYUFBYSxVQUFVO0FBQ3ZDLFdBQUssT0FBTztBQUNaLFdBQUssUUFBUyxRQUFPLFNBQVMsS0FBSztBQUNuQyxXQUFLLFFBQVE7QUFFYixZQUFNLFNBQVMsR0FBRztBQUNsQixVQUFJLFFBQVEsUUFBUSxrQkFBa0IsTUFBTTtBQUMxQyxnQkFBUSxhQUFhLFFBQVE7QUFBQTtBQUUvQixTQUFHLGFBQWEsY0FBYyxLQUFLLFFBQVE7QUFDM0MsVUFBSSxHQUFHLGFBQWEsZUFBZTtBQUNqQyxhQUFLLFFBQVEsSUFBSSxhQUFhLGVBQWUsVUFBVTtBQUFBO0FBR3pELFdBQUssZUFBZTtBQUNwQixXQUFLLFlBQVk7QUFDakIsV0FBSyxZQUFZLENBQUMsQ0FBQztBQUVuQixVQUFJLE9BQU8sR0FBRztBQUNkLGFBQU8sTUFBTTtBQUNYLFlBQUksS0FBSyxRQUFRLGlCQUFpQixNQUFNO0FBQ3RDLGdCQUFNLFVBQVUsR0FBRyxPQUFPLFNBQVMsZ0JBQWdCLEtBQUssUUFBUSxRQUFRLFdBQVc7QUFDbkYsYUFBRyxhQUFhLGFBQWE7QUFDN0IsYUFBRyxhQUFhLGlCQUFpQjtBQUNqQyxlQUFLLGFBQWEsUUFBUTtBQUMxQixlQUFLLGFBQWEsTUFBTTtBQUN4QixlQUFLLGVBQWU7QUFDcEI7QUFBQTtBQUdGLGVBQU8sS0FBSztBQUFBO0FBRWQsV0FBSztBQUFBO0FBQUEsSUFHQyxPQUFPO0FBQ2IsV0FBSyxHQUFHLFdBQVc7QUFDbkIsVUFBSSxDQUFDLEtBQUssR0FBRyxhQUFhLFNBQVM7QUFDakMsYUFBSyxHQUFHLGFBQWEsUUFBUTtBQUFBO0FBRS9CLFdBQUssR0FBRyxpQkFBaUIsV0FBVyxLQUFLLGNBQWMsS0FBSztBQUM1RCxXQUFLLEdBQUcsaUJBQWlCLFNBQVMsS0FBSyxZQUFZLEtBQUs7QUFDeEQsV0FBSyxHQUFHLGlCQUFpQixTQUFTLEtBQUssWUFBWSxLQUFLO0FBQ3hELFdBQUssR0FBRyxpQkFBaUIsUUFBUSxLQUFLLFdBQVcsS0FBSztBQUFBO0FBQUEsSUFHeEQsYUFBYTtBQUNYLFVBQUksS0FBSyxjQUFjO0FBQ3JCLGVBQU8sS0FBSyxHQUFHLGFBQWEscUJBQXFCO0FBQUE7QUFHbkQsYUFBTztBQUFBO0FBQUEsSUFHVCxhQUFhO0FBQ1gsYUFBTyxLQUFLLEdBQUcsYUFBYSxxQkFBcUI7QUFBQTtBQUFBLElBRzNDLFlBQVksT0FBbUI7QUFFckMsVUFBSSxNQUFNLFdBQVcsS0FBSyxNQUFNLE1BQU0sV0FBVyxLQUFLLEdBQUcsbUJBQW1CO0FBQzFFO0FBQUE7QUFFRixVQUFJLEtBQUssY0FBYztBQUNyQixZQUFJLEtBQUssZ0JBQWdCLEtBQUssY0FBYztBQUMxQyxlQUFLLEtBQUssaUJBQWlCO0FBQUEsZUFDdEI7QUFDTCxlQUFLLEtBQUssZUFBZTtBQUFBO0FBRTNCLGNBQU07QUFBQTtBQUVSLFdBQUssS0FBSyxZQUFZO0FBQUE7QUFBQSxJQUdoQixjQUFjO0FBQ3BCLFVBQUksS0FBSyxLQUFLO0FBQ2QsVUFBSSxLQUFLLGNBQWM7QUFDckIsYUFBTSxHQUFHLHFCQUFxQztBQUFBO0FBRWhELFNBQUcsVUFBVSxJQUFJO0FBQUE7QUFBQSxJQUdYLGFBQWE7QUFDbkIsVUFBSSxLQUFLLEtBQUs7QUFDZCxVQUFJLEtBQUssY0FBYztBQUNyQixhQUFNLEdBQUcscUJBQXFDO0FBQUE7QUFFaEQsU0FBRyxVQUFVLE9BQU87QUFBQTtBQUFBLElBR2QsY0FBYyxPQUFzQjtBQUMxQyxVQUFJLE1BQU0sVUFBVSxNQUFNLFdBQVcsTUFBTSxTQUFTO0FBQ2xEO0FBQUE7QUFHRixVQUFJLFdBQVc7QUFDZixjQUFRLE1BQU07QUFBQSxhQUNQO0FBQUEsYUFDQTtBQUNILGNBQUksS0FBSyxjQUFjO0FBQ3JCLGdCQUFJLEtBQUssZ0JBQWdCLEtBQUssY0FBYztBQUMxQyxtQkFBSyxLQUFLLGlCQUFpQjtBQUFBLG1CQUN0QjtBQUNMLG1CQUFLLEtBQUssZUFBZTtBQUFBO0FBRTNCLHVCQUFXO0FBQUEsaUJBQ047QUFDTCxrQkFBTTtBQUFBO0FBRVIsZUFBSyxLQUFLLFlBQVk7QUFDdEI7QUFBQSxhQUVHO0FBQ0gsZUFBSyxLQUFLLHVCQUF1QjtBQUNqQyxxQkFBVztBQUNYO0FBQUEsYUFFRztBQUNILGVBQUssS0FBSyxtQkFBbUI7QUFDN0IscUJBQVc7QUFDWDtBQUFBLGFBRUc7QUFDSCxjQUFJLEtBQUssY0FBYztBQUNyQixnQkFBSSxLQUFLLGNBQWM7QUFDckIsbUJBQUssS0FBSyxtQkFBbUI7QUFBQSxtQkFDeEI7QUFDTCxtQkFBSyxLQUFLLGVBQWU7QUFBQTtBQUFBO0FBRzdCLHFCQUFXO0FBQ1g7QUFBQSxhQUVHO0FBQ0gsY0FBSSxLQUFLLGdCQUFnQixLQUFLLGNBQWM7QUFDMUMsaUJBQUssS0FBSyxpQkFBaUI7QUFDM0IsdUJBQVc7QUFBQSxpQkFDTjtBQUNMLGdCQUFJLEtBQUssV0FBVztBQUNsQixtQkFBSyxLQUFLLHFCQUFxQjtBQUMvQix5QkFBVztBQUFBO0FBQUE7QUFHZjtBQUFBLGFBRUc7QUFDSCxlQUFLLEtBQUs7QUFDVixxQkFBVztBQUNYO0FBQUEsYUFFRztBQUNILGVBQUssS0FBSztBQUNWLHFCQUFXO0FBQ1g7QUFBQTtBQUdBLGNBQUksTUFBTSxJQUFJLFdBQVcsS0FBSyxNQUFNLElBQUksTUFBTSxPQUFPO0FBQ25ELGdCQUFJLE1BQU0sT0FBTyxLQUFLO0FBQ3BCLG1CQUFLLEtBQUssc0JBQXNCO0FBQUEsbUJBQzNCO0FBQ0wsbUJBQUssS0FBSyx5QkFBeUIsTUFBTSxNQUFNO0FBQUE7QUFFakQsdUJBQVc7QUFBQTtBQUViO0FBQUE7QUFHSixVQUFJLFVBQVU7QUFDWixjQUFNO0FBQ04sY0FBTTtBQUFBO0FBQUE7QUFBQTtBQU1aLG9CQUFxRCxNQUFTLE1BQWM7QUFDMUUsUUFBSTtBQUNKLFdBQU8sSUFBSSxTQUF3QjtBQUNqQyxZQUFNLFFBQVEsTUFBTTtBQUNsQixrQkFBVTtBQUNWLGFBQUssR0FBRztBQUFBO0FBRVYsVUFBSSxTQUFTO0FBQ1gscUJBQWE7QUFBQTtBQUVmLGdCQUFVLFdBQVcsT0FBTztBQUFBO0FBQUE7OztBQzFkaEM7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBVUEsU0FBTyxpQkFBaUIsUUFBUSxNQUFNO0FBQ3BDLFVBQU0sT0FBTyxTQUFTLGNBQTJCO0FBQ2pELFFBQUksTUFBTTtBQUNSLFlBQU0sV0FBVyxJQUFJLGtCQUFrQjtBQUN2QyxZQUFNLFNBQVMsY0FBYztBQUM3QixlQUFTLGNBQWMsc0JBQXNCLFlBQVk7QUFBQTtBQUczRCxVQUFNLFlBQVksU0FBUyxjQUEyQjtBQUN0RCxRQUFJLFdBQVc7QUFDYixZQUFNLFdBQVcsSUFBSSxrQkFBa0I7QUFDdkMsWUFBTSxTQUFTLGNBQWM7QUFDN0IsZUFBUyxjQUFjLHdCQUF3QixZQUFZO0FBQUE7QUFHN0QsZUFBVyxNQUFNLFNBQVMsaUJBQWlCLG9CQUFvQjtBQUM3RCxTQUFHLGlCQUFpQixTQUFTLE9BQUs7QUFDaEMsY0FBTSxRQUFTLEVBQUUsY0FBb0MsYUFBYTtBQUNsRSxpQkFBUyxnQkFBZ0IsYUFBYSxjQUFjLE9BQU87QUFBQTtBQUFBO0FBRy9ELGVBQVcsTUFBTSxTQUFTLGlCQUFpQixxQkFBcUI7QUFDOUQsU0FBRyxpQkFBaUIsU0FBUyxPQUFLO0FBQ2hDLGNBQU0sUUFBUyxFQUFFLGNBQW9DLGFBQWE7QUFDbEUsaUJBQVMsZ0JBQWdCLGFBQWEsZUFBZSxPQUFPO0FBQUE7QUFBQTtBQUloRSxlQUFXLE1BQU0sU0FBUyxpQkFBb0Msa0JBQWtCO0FBQzlFLFVBQUksb0JBQW9CO0FBQUE7QUFBQTtBQUk1QixpQkFBZSxPQUNiLFlBQ0EsY0FBYyxZQUFZO0FBQUEsSUFDeEIsY0FBYztBQUNaO0FBQ0EsV0FBSyxNQUFNLFlBQVksV0FBVztBQUdsQyxZQUFNLE9BQU8sS0FBSztBQUNsQixXQUFLLGdCQUFnQjtBQUNyQixXQUFLLFlBQVk7QUFBQSxtQ0FDWTtBQUFBO0FBQUEscUJBRWQsNENBQTRDLEtBQ3hELFFBQVEsWUFBWSxJQUNwQixXQUFXLEtBQUs7QUFBQSxxREFDNEI7QUFBQTtBQUFBO0FBRy9DLFdBQUssY0FBYyxRQUFRLGlCQUFpQixTQUFTLE1BQU07QUFDekQsa0JBQVUsVUFBVSxVQUFVLE9BQU87QUFBQTtBQUFBO0FBQUE7QUFNN0MsaUJBQWUsT0FDYixXQUNBLGNBQWMsWUFBWTtBQUFBLElBQ3hCLGNBQWM7QUFDWjtBQUNBLFdBQUssTUFBTSxZQUFZLFdBQVc7QUFHbEMsWUFBTSxPQUFPLEtBQUssYUFBYTtBQUMvQixXQUFLLFlBQVksZUFBZSwyQ0FBMkMsS0FBSyxXQUM5RSxLQUNBO0FBQUE7QUFBQSxpRkFHeUU7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQU9qRixpQkFBZSxPQUNiLFlBQ0EsY0FBYyxZQUFZO0FBQUEsSUFDeEIsY0FBYztBQUNaO0FBQ0EsV0FBSyxNQUFNLFlBQVksV0FBVztBQUNsQyxZQUFNLFdBQVcsS0FBSyxhQUFhO0FBQ25DLFVBQUksQ0FBQztBQUFVO0FBQ2YsWUFBTSxPQUFPLFNBQVMsU0FBUyxjQUFjLFdBQVc7QUFDeEQsV0FBSyxZQUFZO0FBQUEsa0NBQ1c7QUFBQTtBQUFBO0FBQUE7QUFNbEMsaUJBQWUsT0FDYixnQkFDQSxjQUFjLFlBQVk7QUFBQSxJQUN4QixjQUFjO0FBQ1o7QUFDQSxXQUFLLE1BQU0sWUFBWSxXQUFXO0FBQ2xDLFlBQU0sT0FBTyxLQUFLO0FBQ2xCLFlBQU0sU0FBUyxLQUFLLEtBQUssUUFBUSxLQUFLLFFBQVE7QUFDOUMsV0FBSyxnQkFBZ0I7QUFDckIsVUFBSSxTQUFTLDBDQUEwQyxPQUFPLEtBQUssU0FBUztBQUM1RSxVQUFJLEtBQUssYUFBYSxjQUFjO0FBQ2xDLGlCQUFTLHNFQUFzRTtBQUFBO0FBRWpGLFdBQUssWUFBWSxRQUFRLFVBQVUsY0FBYztBQUNqRCxXQUFLLGNBQWMsUUFBUSxpQkFBaUIsU0FBUyxNQUFNO0FBQ3pELGtCQUFVLFVBQVUsVUFBVTtBQUFBO0FBQUE7QUFBQTtBQVl0QyxnQkFBYyxNQUFjO0FBQzFCLFdBQU8sS0FDSixNQUFNLE1BQ04sT0FDQyxDQUFDLEtBQUssUUFBUTtBQUNaLFVBQUksSUFBSSxPQUFPLFdBQVcsR0FBRztBQUMzQixjQUFNLFFBQVEsSUFBSSxRQUFRO0FBQzFCLFlBQUksUUFBUSxVQUFVLEtBQUssSUFBSTtBQUFBO0FBRWpDLFlBQU0sSUFBSSxNQUFNLElBQUk7QUFDcEIsVUFBSSxLQUFLO0FBQ1AsWUFBSSxPQUFPLEtBQUs7QUFBQTtBQUVsQixhQUFPO0FBQUEsT0FFVCxDQUFFLFFBQVEsSUFBSSxPQUFPLElBRXRCLE9BQU8sS0FBSztBQUFBO0FBR2pCLGtCQUFnQixNQUFjO0FBRzVCLFdBQVEsTUFBYyxXQUFXLEtBQUssU0FBUyxXQUFXLEtBQUs7QUFBQTsiLAogICJuYW1lcyI6IFtdCn0K
