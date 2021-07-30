(() => {
  // static/shared/jump/jump.ts
  /*!
   * @license
   * Copyright 2019-2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var jumpDialog = document.querySelector(".JumpDialog");
  var jumpBody = jumpDialog?.querySelector(".JumpDialog-body");
  var jumpList = jumpDialog?.querySelector(".JumpDialog-list");
  var jumpFilter = jumpDialog?.querySelector(".JumpDialog-input");
  var doc = document.querySelector(".js-documentation");
  var jumpListItems;
  function collectJumpListItems() {
    const items = [];
    if (!doc)
      return;
    for (const el of doc.querySelectorAll("[data-kind]")) {
      items.push(newJumpListItem(el));
    }
    for (const item of items) {
      item.link.addEventListener("click", function() {
        jumpDialog?.close();
      });
    }
    items.sort(function(a, b) {
      return a.lower.localeCompare(b.lower);
    });
    return items;
  }
  function newJumpListItem(el) {
    const a = document.createElement("a");
    const name = el.getAttribute("id");
    a.setAttribute("href", "#" + name);
    a.setAttribute("tabindex", "-1");
    a.setAttribute("data-gtmc", "jump to link");
    const kind = el.getAttribute("data-kind");
    return {
      link: a,
      name: name ?? "",
      kind: kind ?? "",
      lower: name?.toLowerCase() ?? ""
    };
  }
  var lastFilterValue;
  var activeJumpItem = -1;
  function updateJumpList(filter) {
    lastFilterValue = filter;
    if (!jumpListItems) {
      jumpListItems = collectJumpListItems();
    }
    setActiveJumpItem(-1);
    while (jumpList?.firstChild) {
      jumpList.firstChild.remove();
    }
    if (filter) {
      const filterLowerCase = filter.toLowerCase();
      const exactMatches = [];
      const prefixMatches = [];
      const infixMatches = [];
      const makeLinkHtml = (item, boldStart, boldEnd) => {
        return item.name.substring(0, boldStart) + "<b>" + item.name.substring(boldStart, boldEnd) + "</b>" + item.name.substring(boldEnd);
      };
      for (const item of jumpListItems ?? []) {
        const nameLowerCase = item.name.toLowerCase();
        if (nameLowerCase === filterLowerCase) {
          item.link.innerHTML = makeLinkHtml(item, 0, item.name.length);
          exactMatches.push(item);
        } else if (nameLowerCase.startsWith(filterLowerCase)) {
          item.link.innerHTML = makeLinkHtml(item, 0, filter.length);
          prefixMatches.push(item);
        } else {
          const index = nameLowerCase.indexOf(filterLowerCase);
          if (index > -1) {
            item.link.innerHTML = makeLinkHtml(item, index, index + filter.length);
            infixMatches.push(item);
          }
        }
      }
      for (const item of exactMatches.concat(prefixMatches).concat(infixMatches)) {
        jumpList?.appendChild(item.link);
      }
    } else {
      if (!jumpListItems || jumpListItems.length === 0) {
        const msg = document.createElement("i");
        msg.innerHTML = "There are no identifiers on this page.";
        jumpList?.appendChild(msg);
      }
      for (const item of jumpListItems ?? []) {
        item.link.innerHTML = item.name + " <i>" + item.kind + "</i>";
        jumpList?.appendChild(item.link);
      }
    }
    if (jumpBody) {
      jumpBody.scrollTop = 0;
    }
    if (jumpListItems?.length && jumpList && jumpList.children.length > 0) {
      setActiveJumpItem(0);
    }
  }
  function setActiveJumpItem(n) {
    const cs = jumpList?.children;
    if (!cs || !jumpBody) {
      return;
    }
    if (activeJumpItem >= 0) {
      cs[activeJumpItem].classList.remove("JumpDialog-active");
    }
    if (n >= cs.length) {
      n = cs.length - 1;
    }
    if (n >= 0) {
      cs[n].classList.add("JumpDialog-active");
      const activeTop = cs[n].offsetTop - cs[0].offsetTop;
      const activeBottom = activeTop + cs[n].clientHeight;
      if (activeTop < jumpBody.scrollTop) {
        jumpBody.scrollTop = activeTop;
      } else if (activeBottom > jumpBody.scrollTop + jumpBody.clientHeight) {
        jumpBody.scrollTop = activeBottom - jumpBody.clientHeight;
      }
    }
    activeJumpItem = n;
  }
  function incActiveJumpItem(delta) {
    if (activeJumpItem < 0) {
      return;
    }
    let n = activeJumpItem + delta;
    if (n < 0) {
      n = 0;
    }
    setActiveJumpItem(n);
  }
  jumpFilter?.addEventListener("keyup", function() {
    if (jumpFilter.value.toUpperCase() != lastFilterValue.toUpperCase()) {
      updateJumpList(jumpFilter.value);
    }
  });
  jumpFilter?.addEventListener("keydown", function(event) {
    const upArrow = 38;
    const downArrow = 40;
    const enterKey = 13;
    switch (event.which) {
      case upArrow:
        incActiveJumpItem(-1);
        event.preventDefault();
        break;
      case downArrow:
        incActiveJumpItem(1);
        event.preventDefault();
        break;
      case enterKey:
        if (activeJumpItem >= 0) {
          if (jumpList) {
            jumpList.children[activeJumpItem].click();
            event.preventDefault();
          }
        }
        break;
    }
  });
  var shortcutsDialog = document.querySelector(".ShortcutsDialog");
  document.addEventListener("keypress", function(e) {
    if (jumpDialog?.open || shortcutsDialog?.open) {
      return;
    }
    const target = e.target;
    const t = target?.tagName;
    if (t == "INPUT" || t == "SELECT" || t == "TEXTAREA") {
      return;
    }
    if (target?.contentEditable == "true") {
      return;
    }
    if (e.metaKey || e.ctrlKey) {
      return;
    }
    const ch = String.fromCharCode(e.which);
    switch (ch) {
      case "f":
      case "F":
        e.preventDefault();
        if (jumpFilter) {
          jumpFilter.value = "";
        }
        jumpDialog?.showModal();
        jumpFilter?.focus();
        updateJumpList("");
        break;
      case "?":
        shortcutsDialog?.showModal();
        break;
    }
  });
  var jumpOutlineInput = document.querySelector(".js-jumpToInput");
  if (jumpOutlineInput) {
    jumpOutlineInput.addEventListener("click", () => {
      if (jumpFilter) {
        jumpFilter.value = "";
      }
      updateJumpList("");
    });
  }

  // static/shared/playground/playground.ts
  /*!
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var PlayExampleClassName = {
    PLAY_HREF: ".js-exampleHref",
    PLAY_CONTAINER: ".js-exampleContainer",
    EXAMPLE_INPUT: ".Documentation-exampleCode",
    EXAMPLE_OUTPUT: ".Documentation-exampleOutput",
    EXAMPLE_ERROR: ".Documentation-exampleError",
    PLAY_BUTTON: ".Documentation-examplePlayButton",
    SHARE_BUTTON: ".Documentation-exampleShareButton",
    FORMAT_BUTTON: ".Documentation-exampleFormatButton",
    RUN_BUTTON: ".Documentation-exampleRunButton"
  };
  var PlaygroundExampleController = class {
    constructor(exampleEl) {
      this.exampleEl = exampleEl;
      this.exampleEl = exampleEl;
      this.anchorEl = exampleEl.querySelector("a");
      this.errorEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_ERROR);
      this.playButtonEl = exampleEl.querySelector(PlayExampleClassName.PLAY_BUTTON);
      this.shareButtonEl = exampleEl.querySelector(PlayExampleClassName.SHARE_BUTTON);
      this.formatButtonEl = exampleEl.querySelector(PlayExampleClassName.FORMAT_BUTTON);
      this.runButtonEl = exampleEl.querySelector(PlayExampleClassName.RUN_BUTTON);
      this.inputEl = this.makeTextArea(exampleEl.querySelector(PlayExampleClassName.EXAMPLE_INPUT));
      this.outputEl = exampleEl.querySelector(PlayExampleClassName.EXAMPLE_OUTPUT);
      this.playButtonEl?.addEventListener("click", () => this.handleShareButtonClick());
      this.shareButtonEl?.addEventListener("click", () => this.handleShareButtonClick());
      this.formatButtonEl?.addEventListener("click", () => this.handleFormatButtonClick());
      this.runButtonEl?.addEventListener("click", () => this.handleRunButtonClick());
      if (!this.inputEl)
        return;
      this.resize();
      this.inputEl.addEventListener("keyup", () => this.resize());
      this.inputEl.addEventListener("keydown", (e) => this.onKeydown(e));
    }
    makeTextArea(el) {
      const t = document.createElement("textarea");
      t.classList.add("Documentation-exampleCode", "code");
      t.spellcheck = false;
      t.value = el?.textContent ?? "";
      el?.parentElement?.replaceChild(t, el);
      return t;
    }
    getAnchorHash() {
      return this.anchorEl?.hash;
    }
    expand() {
      this.exampleEl.open = true;
    }
    resize() {
      if (this.inputEl?.value) {
        const numLineBreaks = (this.inputEl.value.match(/\n/g) || []).length;
        this.inputEl.style.height = `${(20 + numLineBreaks * 20 + 12 + 2) / 16}rem`;
      }
    }
    onKeydown(e) {
      if (e.key === "Tab") {
        document.execCommand("insertText", false, "	");
        e.preventDefault();
      }
    }
    setInputText(output) {
      if (this.inputEl) {
        this.inputEl.value = output;
      }
    }
    setOutputText(output) {
      if (this.outputEl) {
        this.outputEl.textContent = output;
      }
    }
    setErrorText(err) {
      if (this.errorEl) {
        this.errorEl.textContent = err;
      }
      this.setOutputText("An error has occurred\u2026");
    }
    handleShareButtonClick() {
      const PLAYGROUND_BASE_URL = "https://play.golang.org/p/";
      this.setOutputText("Waiting for remote server\u2026");
      fetch("/play/share", {
        method: "POST",
        body: this.inputEl?.value
      }).then((res) => res.text()).then((shareId) => {
        const href = PLAYGROUND_BASE_URL + shareId;
        this.setOutputText(`<a href="${href}">${href}</a>`);
        window.open(href);
      }).catch((err) => {
        this.setErrorText(err);
      });
    }
    handleFormatButtonClick() {
      this.setOutputText("Waiting for remote server\u2026");
      const body = new FormData();
      body.append("body", this.inputEl?.value ?? "");
      fetch("/play/fmt", {
        method: "POST",
        body
      }).then((res) => res.json()).then(({Body, Error}) => {
        this.setOutputText(Error || "Done.");
        if (Body) {
          this.setInputText(Body);
          this.resize();
        }
      }).catch((err) => {
        this.setErrorText(err);
      });
    }
    handleRunButtonClick() {
      this.setOutputText("Waiting for remote server\u2026");
      fetch("/play/compile", {
        method: "POST",
        body: JSON.stringify({body: this.inputEl?.value, version: 2})
      }).then((res) => res.json()).then(async ({Events, Errors}) => {
        this.setOutputText(Errors || "");
        for (const e of Events || []) {
          this.setOutputText(e.Message);
          await new Promise((resolve) => setTimeout(resolve, e.Delay / 1e6));
        }
      }).catch((err) => {
        this.setErrorText(err);
      });
    }
  };
  var exampleHashRegex = location.hash.match(/^#(example-.*)$/);
  if (exampleHashRegex) {
    const exampleHashEl = document.getElementById(exampleHashRegex[1]);
    if (exampleHashEl) {
      exampleHashEl.open = true;
    }
  }
  var exampleHrefs = [
    ...document.querySelectorAll(PlayExampleClassName.PLAY_HREF)
  ];
  var findExampleHash = (playContainer) => exampleHrefs.find((ex) => {
    return ex.hash === playContainer.getAnchorHash();
  });
  for (const el of document.querySelectorAll(PlayExampleClassName.PLAY_CONTAINER)) {
    const playContainer = new PlaygroundExampleController(el);
    const exampleHref = findExampleHash(playContainer);
    if (exampleHref) {
      exampleHref.addEventListener("click", () => {
        playContainer.expand();
      });
    } else {
      console.warn("example href not found");
    }
  }

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

  // static/frontend/unit/main/main.ts
  var treeEl = document.querySelector(".js-tree");
  if (treeEl) {
    const treeCtrl = new TreeNavController(treeEl);
    const select = makeSelectNav(treeCtrl);
    const mobileNav = document.querySelector(".js-mainNavMobile");
    if (mobileNav && mobileNav.firstElementChild) {
      mobileNav?.replaceChild(select, mobileNav.firstElementChild);
    }
    if (select.firstElementChild) {
      new SelectNavController(select.firstElementChild);
    }
  }
  var readme = document.querySelector(".js-readme");
  var readmeContent = document.querySelector(".js-readmeContent");
  var readmeOutline = document.querySelector(".js-readmeOutline");
  var readmeExpand = document.querySelectorAll(".js-readmeExpand");
  var readmeCollapse = document.querySelector(".js-readmeCollapse");
  var mobileNavSelect = document.querySelector(".DocNavMobile-select");
  if (readme && readmeContent && readmeOutline && readmeExpand.length && readmeCollapse) {
    if (window.location.hash.includes("readme")) {
      readme.classList.add("UnitReadme--expanded");
    }
    mobileNavSelect?.addEventListener("change", (e) => {
      if (e.target.value.startsWith("readme-")) {
        readme.classList.add("UnitReadme--expanded");
      }
    });
    readmeExpand.forEach((el) => el.addEventListener("click", (e) => {
      e.preventDefault();
      readme.classList.add("UnitReadme--expanded");
      readme.scrollIntoView();
    }));
    readmeCollapse.addEventListener("click", (e) => {
      e.preventDefault();
      readme.classList.remove("UnitReadme--expanded");
      if (readmeExpand[1]) {
        readmeExpand[1].scrollIntoView({block: "center"});
      }
    });
    readmeContent.addEventListener("keyup", () => {
      readme.classList.add("UnitReadme--expanded");
    });
    readmeContent.addEventListener("click", () => {
      readme.classList.add("UnitReadme--expanded");
    });
    readmeOutline.addEventListener("click", () => {
      readme.classList.add("UnitReadme--expanded");
    });
    document.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "f") {
        readme.classList.add("UnitReadme--expanded");
      }
    });
  }
  function openDeprecatedSymbol() {
    if (!location.hash)
      return;
    const heading = document.querySelector(location.hash);
    const grandParent = heading?.parentElement?.parentElement;
    if (grandParent?.nodeName === "DETAILS") {
      grandParent.open = true;
    }
  }
  openDeprecatedSymbol();
  window.addEventListener("hashchange", () => openDeprecatedSymbol());
  document.querySelectorAll(".js-buildContextSelect").forEach((el) => {
    el.addEventListener("change", (e) => {
      window.location.search = `?GOOS=${e.target.value}`;
    });
  });
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiLi4vLi4vLi4vc2hhcmVkL2p1bXAvanVtcC50cyIsICIuLi8uLi8uLi9zaGFyZWQvcGxheWdyb3VuZC9wbGF5Z3JvdW5kLnRzIiwgIi4uLy4uLy4uL3NoYXJlZC9vdXRsaW5lL3NlbGVjdC50cyIsICIuLi8uLi8uLi9zaGFyZWQvb3V0bGluZS90cmVlLnRzIiwgIm1haW4udHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDE5LTIwMjAgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8vIFRoaXMgZmlsZSBpbXBsZW1lbnRzIHRoZSBiZWhhdmlvciBvZiB0aGUgXCJqdW1wIHRvIGlkZW50aWZlclwiIGRpYWxvZyBmb3IgR29cbi8vIHBhY2thZ2UgZG9jdW1lbnRhdGlvbiwgYXMgd2VsbCBhcyB0aGUgc2ltcGxlIGRpYWxvZyB0aGF0IGRpc3BsYXlzIGtleWJvYXJkXG4vLyBzaG9ydGN1dHMuXG5cbi8vIFRoZSBET00gZm9yIHRoZSBkaWFsb2dzIGlzIGF0IHRoZSBib3R0b20gb2Ygc3RhdGljL2Zyb250ZW5kL3VuaXQvbWFpbi9fbW9kYWxzLnRtcGwuXG4vLyBUaGUgQ1NTIGlzIGluIHN0YXRpYy9mcm9udGVuZC91bml0L21haW4vX21vZGFscy5jc3MuXG5cbi8vIFRoZSBkaWFsb2cgaXMgYWN0aXZhdGVkIGJ5IHByZXNzaW5nIHRoZSAnZicga2V5LiBJdCBwcmVzZW50cyBhIGxpc3Rcbi8vICgjSnVtcERpYWxvZy1saXN0KSBvZiBhbGwgR28gaWRlbnRpZmllcnMgZGlzcGxheWVkIGluIHRoZSBkb2N1bWVudGF0aW9uLlxuLy8gRW50ZXJpbmcgdGV4dCBpbiB0aGUgZGlhbG9nJ3MgdGV4dCBib3ggKCNKdW1wRGlhbG9nLWZpbHRlcikgcmVzdHJpY3RzIHRoZVxuLy8gbGlzdCB0byBpZGVudGlmaWVycyBjb250YWluaW5nIHRoZSB0ZXh0LiBDbGlja2luZyBvbiBhbiBpZGVudGlmaWVyIGp1bXBzIHRvXG4vLyBpdHMgZG9jdW1lbnRhdGlvbi5cblxuLy8gVGhpcyBjb2RlIGlzIGJhc2VkIG9uXG4vLyBodHRwczovL2dvLmdvb2dsZXNvdXJjZS5jb20vZ2Rkby8rL3JlZnMvaGVhZHMvbWFzdGVyL2dkZG8tc2VydmVyL2Fzc2V0cy9zaXRlLmpzLlxuLy8gSXQgd2FzIG1vZGlmaWVkIHRvIHJlbW92ZSB0aGUgZGVwZW5kZW5jZSBvbiBqcXVlcnkgYW5kIGJvb3RzdHJhcC5cblxuY29uc3QganVtcERpYWxvZyA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTERpYWxvZ0VsZW1lbnQ+KCcuSnVtcERpYWxvZycpO1xuY29uc3QganVtcEJvZHkgPSBqdW1wRGlhbG9nPy5xdWVyeVNlbGVjdG9yPEhUTUxEaXZFbGVtZW50PignLkp1bXBEaWFsb2ctYm9keScpO1xuY29uc3QganVtcExpc3QgPSBqdW1wRGlhbG9nPy5xdWVyeVNlbGVjdG9yPEhUTUxEaXZFbGVtZW50PignLkp1bXBEaWFsb2ctbGlzdCcpO1xuY29uc3QganVtcEZpbHRlciA9IGp1bXBEaWFsb2c/LnF1ZXJ5U2VsZWN0b3I8SFRNTElucHV0RWxlbWVudD4oJy5KdW1wRGlhbG9nLWlucHV0Jyk7XG5jb25zdCBkb2MgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxEaXZFbGVtZW50PignLmpzLWRvY3VtZW50YXRpb24nKTtcblxuaW50ZXJmYWNlIEp1bXBMaXN0SXRlbSB7XG4gIGxpbms6IEhUTUxBbmNob3JFbGVtZW50O1xuICBuYW1lOiBzdHJpbmc7XG4gIGtpbmQ6IHN0cmluZztcbiAgbG93ZXI6IHN0cmluZztcbn1cblxubGV0IGp1bXBMaXN0SXRlbXM6IEp1bXBMaXN0SXRlbVtdIHwgdW5kZWZpbmVkOyAvLyBBbGwgdGhlIGlkZW50aWZpZXJzIGluIHRoZSBkb2M7IGNvbXB1dGVkIG9ubHkgb25jZS5cblxuLy8gY29sbGVjdEp1bXBMaXN0SXRlbXMgcmV0dXJucyBhIGxpc3Qgb2YgaXRlbXMsIG9uZSBmb3IgZWFjaCBpZGVudGlmaWVyIGluIHRoZVxuLy8gZG9jdW1lbnRhdGlvbiBvbiB0aGUgY3VycmVudCBwYWdlLlxuLy9cbi8vIEl0IHVzZXMgdGhlIGRhdGEta2luZCBhdHRyaWJ1dGUgZ2VuZXJhdGVkIGluIHRoZSBkb2N1bWVudGF0aW9uIEhUTUwgdG8gZmluZFxuLy8gdGhlIGlkZW50aWZpZXJzIGFuZCB0aGVpciBpZCBhdHRyaWJ1dGVzLlxuLy9cbi8vIElmIHRoZXJlIGFyZSBubyBkYXRhLWtpbmQgYXR0cmlidXRlcywgdGhlbiB3ZSBoYXZlIG9sZGVyIGRvYzsgZmFsbCBiYWNrIHRvXG4vLyBhIGxlc3MgcHJlY2lzZSBtZXRob2QuXG5mdW5jdGlvbiBjb2xsZWN0SnVtcExpc3RJdGVtcygpIHtcbiAgY29uc3QgaXRlbXMgPSBbXTtcbiAgaWYgKCFkb2MpIHJldHVybjtcbiAgZm9yIChjb25zdCBlbCBvZiBkb2MucXVlcnlTZWxlY3RvckFsbCgnW2RhdGEta2luZF0nKSkge1xuICAgIGl0ZW1zLnB1c2gobmV3SnVtcExpc3RJdGVtKGVsKSk7XG4gIH1cblxuICAvLyBDbGlja2luZyBvbiBhbnkgb2YgdGhlIGxpbmtzIGNsb3NlcyB0aGUgZGlhbG9nLlxuICBmb3IgKGNvbnN0IGl0ZW0gb2YgaXRlbXMpIHtcbiAgICBpdGVtLmxpbmsuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBmdW5jdGlvbiAoKSB7XG4gICAgICBqdW1wRGlhbG9nPy5jbG9zZSgpO1xuICAgIH0pO1xuICB9XG4gIC8vIFNvcnQgY2FzZS1pbnNlbnNpdGl2ZWx5IGJ5IGlkZW50aWZpZXIgbmFtZS5cbiAgaXRlbXMuc29ydChmdW5jdGlvbiAoYSwgYikge1xuICAgIHJldHVybiBhLmxvd2VyLmxvY2FsZUNvbXBhcmUoYi5sb3dlcik7XG4gIH0pO1xuICByZXR1cm4gaXRlbXM7XG59XG5cbi8vIG5ld0p1bXBMaXN0SXRlbSBjcmVhdGVzIGEgbmV3IGl0ZW0gZm9yIHRoZSBET00gZWxlbWVudCBlbC5cbi8vIEFuIGl0ZW0gaXMgYW4gb2JqZWN0IHdpdGg6XG4vLyAtIG5hbWU6IHRoZSBlbGVtZW50J3MgaWQgKHdoaWNoIGlzIHRoZSBpZGVudGlmZXIgbmFtZSlcbi8vIC0ga2luZDogdGhlIGVsZW1lbnQncyBraW5kIChmdW5jdGlvbiwgdmFyaWFibGUsIGV0Yy4pLFxuLy8gLSBsaW5rOiBhIGxpbmsgKCdhJyB0YWcpIHRvIHRoZSBlbGVtZW50XG4vLyAtIGxvd2VyOiB0aGUgbmFtZSBpbiBsb3dlciBjYXNlLCBqdXN0IGZvciBzb3J0aW5nXG5mdW5jdGlvbiBuZXdKdW1wTGlzdEl0ZW0oZWw6IEVsZW1lbnQpOiBKdW1wTGlzdEl0ZW0ge1xuICBjb25zdCBhID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnYScpO1xuICBjb25zdCBuYW1lID0gZWwuZ2V0QXR0cmlidXRlKCdpZCcpO1xuICBhLnNldEF0dHJpYnV0ZSgnaHJlZicsICcjJyArIG5hbWUpO1xuICBhLnNldEF0dHJpYnV0ZSgndGFiaW5kZXgnLCAnLTEnKTtcbiAgYS5zZXRBdHRyaWJ1dGUoJ2RhdGEtZ3RtYycsICdqdW1wIHRvIGxpbmsnKTtcbiAgY29uc3Qga2luZCA9IGVsLmdldEF0dHJpYnV0ZSgnZGF0YS1raW5kJyk7XG4gIHJldHVybiB7XG4gICAgbGluazogYSxcbiAgICBuYW1lOiBuYW1lID8/ICcnLFxuICAgIGtpbmQ6IGtpbmQgPz8gJycsXG4gICAgbG93ZXI6IG5hbWU/LnRvTG93ZXJDYXNlKCkgPz8gJycsIC8vIGZvciBzb3J0aW5nXG4gIH07XG59XG5cbmxldCBsYXN0RmlsdGVyVmFsdWU6IHN0cmluZzsgLy8gVGhlIGxhc3QgY29udGVudHMgb2YgdGhlIGZpbHRlciB0ZXh0IGJveC5cbmxldCBhY3RpdmVKdW1wSXRlbSA9IC0xOyAvLyBUaGUgaW5kZXggb2YgdGhlIGN1cnJlbnRseSBhY3RpdmUgaXRlbSBpbiB0aGUgbGlzdC5cblxuLy8gdXBkYXRlSnVtcExpc3Qgc2V0cyB0aGUgZWxlbWVudHMgb2YgdGhlIGRpYWxvZyBsaXN0IHRvXG4vLyBldmVyeXRoaW5nIHdob3NlIG5hbWUgY29udGFpbnMgZmlsdGVyLlxuZnVuY3Rpb24gdXBkYXRlSnVtcExpc3QoZmlsdGVyOiBzdHJpbmcpIHtcbiAgbGFzdEZpbHRlclZhbHVlID0gZmlsdGVyO1xuICBpZiAoIWp1bXBMaXN0SXRlbXMpIHtcbiAgICBqdW1wTGlzdEl0ZW1zID0gY29sbGVjdEp1bXBMaXN0SXRlbXMoKTtcbiAgfVxuICBzZXRBY3RpdmVKdW1wSXRlbSgtMSk7XG5cbiAgLy8gUmVtb3ZlIGFsbCBjaGlsZHJlbiBmcm9tIGxpc3QuXG4gIHdoaWxlIChqdW1wTGlzdD8uZmlyc3RDaGlsZCkge1xuICAgIGp1bXBMaXN0LmZpcnN0Q2hpbGQucmVtb3ZlKCk7XG4gIH1cblxuICBpZiAoZmlsdGVyKSB7XG4gICAgLy8gQSBmaWx0ZXIgaXMgc2V0LiBXZSB0cmVhdCB0aGUgZmlsdGVyIGFzIGEgc3Vic3RyaW5nIHRoYXQgY2FuIGFwcGVhciBpblxuICAgIC8vIGFuIGl0ZW0gbmFtZSAoY2FzZSBpbnNlbnNpdGl2ZSksIGFuZCBmaW5kIHRoZSBmb2xsb3dpbmcgbWF0Y2hlcyAtIGluXG4gICAgLy8gb3JkZXIgb2YgcHJpb3JpdHk6XG4gICAgLy9cbiAgICAvLyAxLiBFeGFjdCBtYXRjaGVzICh0aGUgZmlsdGVyIG1hdGNoZXMgdGhlIGl0ZW0ncyBuYW1lIGV4YWN0bHkpXG4gICAgLy8gMi4gUHJlZml4IG1hdGNoZXMgKHRoZSBpdGVtJ3MgbmFtZSBzdGFydHMgd2l0aCBmaWx0ZXIpXG4gICAgLy8gMy4gSW5maXggbWF0Y2hlcyAodGhlIGZpbHRlciBpcyBhIHN1YnN0cmluZyBvZiB0aGUgaXRlbSdzIG5hbWUpXG4gICAgY29uc3QgZmlsdGVyTG93ZXJDYXNlID0gZmlsdGVyLnRvTG93ZXJDYXNlKCk7XG5cbiAgICBjb25zdCBleGFjdE1hdGNoZXMgPSBbXTtcbiAgICBjb25zdCBwcmVmaXhNYXRjaGVzID0gW107XG4gICAgY29uc3QgaW5maXhNYXRjaGVzID0gW107XG5cbiAgICAvLyBtYWtlTGlua0h0bWwgY3JlYXRlcyB0aGUgbGluayBuYW1lIEhUTUwgZm9yIGEgbGlzdCBpdGVtLiBpdGVtIGlzIHRoZSBET01cbiAgICAvLyBpdGVtLiBpdGVtLm5hbWUuc3Vic3RyKGJvbGRTdGFydCwgYm9sZEVuZCkgd2lsbCBiZSBib2xkZWQuXG4gICAgY29uc3QgbWFrZUxpbmtIdG1sID0gKGl0ZW06IEp1bXBMaXN0SXRlbSwgYm9sZFN0YXJ0OiBudW1iZXIsIGJvbGRFbmQ6IG51bWJlcikgPT4ge1xuICAgICAgcmV0dXJuIChcbiAgICAgICAgaXRlbS5uYW1lLnN1YnN0cmluZygwLCBib2xkU3RhcnQpICtcbiAgICAgICAgJzxiPicgK1xuICAgICAgICBpdGVtLm5hbWUuc3Vic3RyaW5nKGJvbGRTdGFydCwgYm9sZEVuZCkgK1xuICAgICAgICAnPC9iPicgK1xuICAgICAgICBpdGVtLm5hbWUuc3Vic3RyaW5nKGJvbGRFbmQpXG4gICAgICApO1xuICAgIH07XG5cbiAgICBmb3IgKGNvbnN0IGl0ZW0gb2YganVtcExpc3RJdGVtcyA/PyBbXSkge1xuICAgICAgY29uc3QgbmFtZUxvd2VyQ2FzZSA9IGl0ZW0ubmFtZS50b0xvd2VyQ2FzZSgpO1xuXG4gICAgICBpZiAobmFtZUxvd2VyQ2FzZSA9PT0gZmlsdGVyTG93ZXJDYXNlKSB7XG4gICAgICAgIGl0ZW0ubGluay5pbm5lckhUTUwgPSBtYWtlTGlua0h0bWwoaXRlbSwgMCwgaXRlbS5uYW1lLmxlbmd0aCk7XG4gICAgICAgIGV4YWN0TWF0Y2hlcy5wdXNoKGl0ZW0pO1xuICAgICAgfSBlbHNlIGlmIChuYW1lTG93ZXJDYXNlLnN0YXJ0c1dpdGgoZmlsdGVyTG93ZXJDYXNlKSkge1xuICAgICAgICBpdGVtLmxpbmsuaW5uZXJIVE1MID0gbWFrZUxpbmtIdG1sKGl0ZW0sIDAsIGZpbHRlci5sZW5ndGgpO1xuICAgICAgICBwcmVmaXhNYXRjaGVzLnB1c2goaXRlbSk7XG4gICAgICB9IGVsc2Uge1xuICAgICAgICBjb25zdCBpbmRleCA9IG5hbWVMb3dlckNhc2UuaW5kZXhPZihmaWx0ZXJMb3dlckNhc2UpO1xuICAgICAgICBpZiAoaW5kZXggPiAtMSkge1xuICAgICAgICAgIGl0ZW0ubGluay5pbm5lckhUTUwgPSBtYWtlTGlua0h0bWwoaXRlbSwgaW5kZXgsIGluZGV4ICsgZmlsdGVyLmxlbmd0aCk7XG4gICAgICAgICAgaW5maXhNYXRjaGVzLnB1c2goaXRlbSk7XG4gICAgICAgIH1cbiAgICAgIH1cbiAgICB9XG5cbiAgICBmb3IgKGNvbnN0IGl0ZW0gb2YgZXhhY3RNYXRjaGVzLmNvbmNhdChwcmVmaXhNYXRjaGVzKS5jb25jYXQoaW5maXhNYXRjaGVzKSkge1xuICAgICAganVtcExpc3Q/LmFwcGVuZENoaWxkKGl0ZW0ubGluayk7XG4gICAgfVxuICB9IGVsc2Uge1xuICAgIGlmICghanVtcExpc3RJdGVtcyB8fCBqdW1wTGlzdEl0ZW1zLmxlbmd0aCA9PT0gMCkge1xuICAgICAgY29uc3QgbXNnID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnaScpO1xuICAgICAgbXNnLmlubmVySFRNTCA9ICdUaGVyZSBhcmUgbm8gaWRlbnRpZmllcnMgb24gdGhpcyBwYWdlLic7XG4gICAgICBqdW1wTGlzdD8uYXBwZW5kQ2hpbGQobXNnKTtcbiAgICB9XG4gICAgLy8gTm8gZmlsdGVyIHNldDsgZGlzcGxheSBhbGwgaXRlbXMgaW4gdGhlaXIgZXhpc3Rpbmcgb3JkZXIuXG4gICAgZm9yIChjb25zdCBpdGVtIG9mIGp1bXBMaXN0SXRlbXMgPz8gW10pIHtcbiAgICAgIGl0ZW0ubGluay5pbm5lckhUTUwgPSBpdGVtLm5hbWUgKyAnIDxpPicgKyBpdGVtLmtpbmQgKyAnPC9pPic7XG4gICAgICBqdW1wTGlzdD8uYXBwZW5kQ2hpbGQoaXRlbS5saW5rKTtcbiAgICB9XG4gIH1cblxuICBpZiAoanVtcEJvZHkpIHtcbiAgICBqdW1wQm9keS5zY3JvbGxUb3AgPSAwO1xuICB9XG4gIGlmIChqdW1wTGlzdEl0ZW1zPy5sZW5ndGggJiYganVtcExpc3QgJiYganVtcExpc3QuY2hpbGRyZW4ubGVuZ3RoID4gMCkge1xuICAgIHNldEFjdGl2ZUp1bXBJdGVtKDApO1xuICB9XG59XG5cbi8vIFNldCB0aGUgYWN0aXZlIGp1bXAgaXRlbSB0byBuLlxuZnVuY3Rpb24gc2V0QWN0aXZlSnVtcEl0ZW0objogbnVtYmVyKSB7XG4gIGNvbnN0IGNzID0ganVtcExpc3Q/LmNoaWxkcmVuIGFzIEhUTUxDb2xsZWN0aW9uT2Y8SFRNTEVsZW1lbnQ+IHwgbnVsbCB8IHVuZGVmaW5lZDtcbiAgaWYgKCFjcyB8fCAhanVtcEJvZHkpIHtcbiAgICByZXR1cm47XG4gIH1cbiAgaWYgKGFjdGl2ZUp1bXBJdGVtID49IDApIHtcbiAgICBjc1thY3RpdmVKdW1wSXRlbV0uY2xhc3NMaXN0LnJlbW92ZSgnSnVtcERpYWxvZy1hY3RpdmUnKTtcbiAgfVxuICBpZiAobiA+PSBjcy5sZW5ndGgpIHtcbiAgICBuID0gY3MubGVuZ3RoIC0gMTtcbiAgfVxuICBpZiAobiA+PSAwKSB7XG4gICAgY3Nbbl0uY2xhc3NMaXN0LmFkZCgnSnVtcERpYWxvZy1hY3RpdmUnKTtcblxuICAgIC8vIFNjcm9sbCBzbyB0aGUgYWN0aXZlIGl0ZW0gaXMgdmlzaWJsZS5cbiAgICAvLyBGb3Igc29tZSByZWFzb24gY3Nbbl0uc2Nyb2xsSW50b1ZpZXcoKSBkb2Vzbid0IGJlaGF2ZSBhcyBJJ2QgZXhwZWN0OlxuICAgIC8vIGl0IG1vdmVzIHRoZSBlbnRpcmUgZGlhbG9nIGJveCBpbiB0aGUgdmlld3BvcnQuXG5cbiAgICAvLyBHZXQgdGhlIHRvcCBhbmQgYm90dG9tIG9mIHRoZSBhY3RpdmUgaXRlbSByZWxhdGl2ZSB0byBqdW1wQm9keS5cbiAgICBjb25zdCBhY3RpdmVUb3AgPSBjc1tuXS5vZmZzZXRUb3AgLSBjc1swXS5vZmZzZXRUb3A7XG4gICAgY29uc3QgYWN0aXZlQm90dG9tID0gYWN0aXZlVG9wICsgY3Nbbl0uY2xpZW50SGVpZ2h0O1xuICAgIGlmIChhY3RpdmVUb3AgPCBqdW1wQm9keS5zY3JvbGxUb3ApIHtcbiAgICAgIC8vIE9mZiB0aGUgdG9wOyBzY3JvbGwgdXAuXG4gICAgICBqdW1wQm9keS5zY3JvbGxUb3AgPSBhY3RpdmVUb3A7XG4gICAgfSBlbHNlIGlmIChhY3RpdmVCb3R0b20gPiBqdW1wQm9keS5zY3JvbGxUb3AgKyBqdW1wQm9keS5jbGllbnRIZWlnaHQpIHtcbiAgICAgIC8vIE9mZiB0aGUgYm90dG9tOyBzY3JvbGwgZG93bi5cbiAgICAgIGp1bXBCb2R5LnNjcm9sbFRvcCA9IGFjdGl2ZUJvdHRvbSAtIGp1bXBCb2R5LmNsaWVudEhlaWdodDtcbiAgICB9XG4gIH1cbiAgYWN0aXZlSnVtcEl0ZW0gPSBuO1xufVxuXG4vLyBJbmNyZW1lbnQgdGhlIGFjdGl2ZUp1bXBJdGVtIGJ5IGRlbHRhLlxuZnVuY3Rpb24gaW5jQWN0aXZlSnVtcEl0ZW0oZGVsdGE6IG51bWJlcikge1xuICBpZiAoYWN0aXZlSnVtcEl0ZW0gPCAwKSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGxldCBuID0gYWN0aXZlSnVtcEl0ZW0gKyBkZWx0YTtcbiAgaWYgKG4gPCAwKSB7XG4gICAgbiA9IDA7XG4gIH1cbiAgc2V0QWN0aXZlSnVtcEl0ZW0obik7XG59XG5cbi8vIFByZXNzaW5nIGEga2V5IGluIHRoZSBmaWx0ZXIgdXBkYXRlcyB0aGUgbGlzdCAoaWYgdGhlIGZpbHRlciBhY3R1YWxseSBjaGFuZ2VkKS5cbmp1bXBGaWx0ZXI/LmFkZEV2ZW50TGlzdGVuZXIoJ2tleXVwJywgZnVuY3Rpb24gKCkge1xuICBpZiAoanVtcEZpbHRlci52YWx1ZS50b1VwcGVyQ2FzZSgpICE9IGxhc3RGaWx0ZXJWYWx1ZS50b1VwcGVyQ2FzZSgpKSB7XG4gICAgdXBkYXRlSnVtcExpc3QoanVtcEZpbHRlci52YWx1ZSk7XG4gIH1cbn0pO1xuXG4vLyBQcmVzc2luZyBlbnRlciBpbiB0aGUgZmlsdGVyIHNlbGVjdHMgdGhlIGZpcnN0IGVsZW1lbnQgaW4gdGhlIGxpc3QuXG5qdW1wRmlsdGVyPy5hZGRFdmVudExpc3RlbmVyKCdrZXlkb3duJywgZnVuY3Rpb24gKGV2ZW50KSB7XG4gIGNvbnN0IHVwQXJyb3cgPSAzODtcbiAgY29uc3QgZG93bkFycm93ID0gNDA7XG4gIGNvbnN0IGVudGVyS2V5ID0gMTM7XG4gIHN3aXRjaCAoZXZlbnQud2hpY2gpIHtcbiAgICBjYXNlIHVwQXJyb3c6XG4gICAgICBpbmNBY3RpdmVKdW1wSXRlbSgtMSk7XG4gICAgICBldmVudC5wcmV2ZW50RGVmYXVsdCgpO1xuICAgICAgYnJlYWs7XG4gICAgY2FzZSBkb3duQXJyb3c6XG4gICAgICBpbmNBY3RpdmVKdW1wSXRlbSgxKTtcbiAgICAgIGV2ZW50LnByZXZlbnREZWZhdWx0KCk7XG4gICAgICBicmVhaztcbiAgICBjYXNlIGVudGVyS2V5OlxuICAgICAgaWYgKGFjdGl2ZUp1bXBJdGVtID49IDApIHtcbiAgICAgICAgaWYgKGp1bXBMaXN0KSB7XG4gICAgICAgICAgKGp1bXBMaXN0LmNoaWxkcmVuW2FjdGl2ZUp1bXBJdGVtXSBhcyBIVE1MRWxlbWVudCkuY2xpY2soKTtcbiAgICAgICAgICBldmVudC5wcmV2ZW50RGVmYXVsdCgpO1xuICAgICAgICB9XG4gICAgICB9XG4gICAgICBicmVhaztcbiAgfVxufSk7XG5cbmNvbnN0IHNob3J0Y3V0c0RpYWxvZyA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTERpYWxvZ0VsZW1lbnQ+KCcuU2hvcnRjdXRzRGlhbG9nJyk7XG5cbi8vIEtleWJvYXJkIHNob3J0Y3V0czpcbi8vIC0gUHJlc3NpbmcgJy8nIGZvY3VzZXMgdGhlIHNlYXJjaCBib3hcbi8vIC0gUHJlc3NpbmcgJ2YnIG9yICdGJyBvcGVucyB0aGUganVtcC10by1pZGVudGlmaWVyIGRpYWxvZy5cbi8vIC0gUHJlc3NpbmcgJz8nIG9wZW5zIHVwIHRoZSBzaG9ydGN1dCBkaWFsb2cuXG4vLyBJZ25vcmUgYSBrZXlwcmVzcyBpZiBhIGRpYWxvZyBpcyBhbHJlYWR5IG9wZW4sIG9yIGlmIGl0IGlzIHByZXNzZWQgb24gYVxuLy8gY29tcG9uZW50IHRoYXQgd2FudHMgdG8gY29uc3VtZSBpdC5cbmRvY3VtZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2tleXByZXNzJywgZnVuY3Rpb24gKGUpIHtcbiAgaWYgKGp1bXBEaWFsb2c/Lm9wZW4gfHwgc2hvcnRjdXRzRGlhbG9nPy5vcGVuKSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGNvbnN0IHRhcmdldCA9IGUudGFyZ2V0IGFzIEhUTUxFbGVtZW50IHwgbnVsbDtcbiAgY29uc3QgdCA9IHRhcmdldD8udGFnTmFtZTtcbiAgaWYgKHQgPT0gJ0lOUFVUJyB8fCB0ID09ICdTRUxFQ1QnIHx8IHQgPT0gJ1RFWFRBUkVBJykge1xuICAgIHJldHVybjtcbiAgfVxuICBpZiAodGFyZ2V0Py5jb250ZW50RWRpdGFibGUgPT0gJ3RydWUnKSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGlmIChlLm1ldGFLZXkgfHwgZS5jdHJsS2V5KSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGNvbnN0IGNoID0gU3RyaW5nLmZyb21DaGFyQ29kZShlLndoaWNoKTtcbiAgc3dpdGNoIChjaCkge1xuICAgIGNhc2UgJ2YnOlxuICAgIGNhc2UgJ0YnOlxuICAgICAgZS5wcmV2ZW50RGVmYXVsdCgpO1xuICAgICAgaWYgKGp1bXBGaWx0ZXIpIHtcbiAgICAgICAganVtcEZpbHRlci52YWx1ZSA9ICcnO1xuICAgICAgfVxuICAgICAganVtcERpYWxvZz8uc2hvd01vZGFsKCk7XG4gICAgICBqdW1wRmlsdGVyPy5mb2N1cygpO1xuICAgICAgdXBkYXRlSnVtcExpc3QoJycpO1xuICAgICAgYnJlYWs7XG4gICAgY2FzZSAnPyc6XG4gICAgICBzaG9ydGN1dHNEaWFsb2c/LnNob3dNb2RhbCgpO1xuICAgICAgYnJlYWs7XG4gIH1cbn0pO1xuXG5jb25zdCBqdW1wT3V0bGluZUlucHV0ID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLWp1bXBUb0lucHV0Jyk7XG5pZiAoanVtcE91dGxpbmVJbnB1dCkge1xuICBqdW1wT3V0bGluZUlucHV0LmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4ge1xuICAgIGlmIChqdW1wRmlsdGVyKSB7XG4gICAgICBqdW1wRmlsdGVyLnZhbHVlID0gJyc7XG4gICAgfVxuICAgIHVwZGF0ZUp1bXBMaXN0KCcnKTtcbiAgfSk7XG59XG4iLCAiLyohXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMjEgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8vIFRoaXMgZmlsZSBpbXBsZW1lbnRzIHRoZSBwbGF5Z3JvdW5kIGltcGxlbWVudGF0aW9uIG9mIHRoZSBkb2N1bWVudGF0aW9uXG4vLyBwYWdlLiBUaGUgcGxheWdyb3VuZCBpbnZvbHZlcyBhIFwicGxheVwiIGJ1dHRvbiB0aGF0IGFsbG93cyB5b3UgdG8gb3BlbiB1cFxuLy8gYSBuZXcgbGluayB0byBwbGF5LmdvbGFuZy5vcmcgdXNpbmcgdGhlIGV4YW1wbGUgY29kZS5cblxuLy8gVGhlIENTUyBpcyBpbiBzdGF0aWMvZnJvbnRlbmQvdW5pdC9tYWluL19kb2MuY3NzXG5cbi8qKlxuICogQ1NTIGNsYXNzZXMgdXNlZCBieSBQbGF5Z3JvdW5kRXhhbXBsZUNvbnRyb2xsZXJcbiAqL1xuY29uc3QgUGxheUV4YW1wbGVDbGFzc05hbWUgPSB7XG4gIFBMQVlfSFJFRjogJy5qcy1leGFtcGxlSHJlZicsXG4gIFBMQVlfQ09OVEFJTkVSOiAnLmpzLWV4YW1wbGVDb250YWluZXInLFxuICBFWEFNUExFX0lOUFVUOiAnLkRvY3VtZW50YXRpb24tZXhhbXBsZUNvZGUnLFxuICBFWEFNUExFX09VVFBVVDogJy5Eb2N1bWVudGF0aW9uLWV4YW1wbGVPdXRwdXQnLFxuICBFWEFNUExFX0VSUk9SOiAnLkRvY3VtZW50YXRpb24tZXhhbXBsZUVycm9yJyxcbiAgUExBWV9CVVRUT046ICcuRG9jdW1lbnRhdGlvbi1leGFtcGxlUGxheUJ1dHRvbicsXG4gIFNIQVJFX0JVVFRPTjogJy5Eb2N1bWVudGF0aW9uLWV4YW1wbGVTaGFyZUJ1dHRvbicsXG4gIEZPUk1BVF9CVVRUT046ICcuRG9jdW1lbnRhdGlvbi1leGFtcGxlRm9ybWF0QnV0dG9uJyxcbiAgUlVOX0JVVFRPTjogJy5Eb2N1bWVudGF0aW9uLWV4YW1wbGVSdW5CdXR0b24nLFxufTtcblxuLyoqXG4gKiBUaGlzIGNvbnRyb2xsZXIgZW5hYmxlcyBwbGF5Z3JvdW5kIGV4YW1wbGVzIHRvIGV4cGFuZCB0aGVpciBkcm9wZG93biBvclxuICogZ2VuZXJhdGUgc2hhcmVhYmxlIEdvIFBsYXlncm91bmQgVVJMcy5cbiAqL1xuZXhwb3J0IGNsYXNzIFBsYXlncm91bmRFeGFtcGxlQ29udHJvbGxlciB7XG4gIC8qKlxuICAgKiBUaGUgYW5jaG9yIHRhZyB1c2VkIHRvIGlkZW50aWZ5IHRoZSBjb250YWluZXIgd2l0aCBhbiBleGFtcGxlIGhyZWYuXG4gICAqIFRoZXJlIGlzIG9ubHkgb25lIGluIGFuIGV4YW1wbGUgY29udGFpbmVyIGRpdi5cbiAgICovXG4gIHByaXZhdGUgcmVhZG9ubHkgYW5jaG9yRWw6IEhUTUxBbmNob3JFbGVtZW50IHwgbnVsbDtcblxuICAvKipcbiAgICogVGhlIGVycm9yIGVsZW1lbnRcbiAgICovXG4gIHByaXZhdGUgcmVhZG9ubHkgZXJyb3JFbDogRWxlbWVudCB8IG51bGw7XG5cbiAgLyoqXG4gICAqIEJ1dHRvbnMgdGhhdCByZWRpcmVjdCB0byBhbiBleGFtcGxlJ3MgcGxheWdyb3VuZCwgdGhpcyBlbGVtZW50XG4gICAqIG9ubHkgZXhpc3RzIGluIGV4ZWN1dGFibGUgZXhhbXBsZXMuXG4gICAqL1xuICBwcml2YXRlIHJlYWRvbmx5IHBsYXlCdXR0b25FbDogRWxlbWVudCB8IG51bGw7XG4gIHByaXZhdGUgcmVhZG9ubHkgc2hhcmVCdXR0b25FbDogRWxlbWVudCB8IG51bGw7XG5cbiAgLyoqXG4gICAqIEJ1dHRvbiB0aGF0IGZvcm1hdHMgdGhlIGNvZGUgaW4gYW4gZXhhbXBsZSdzIHBsYXlncm91bmQuXG4gICAqL1xuICBwcml2YXRlIHJlYWRvbmx5IGZvcm1hdEJ1dHRvbkVsOiBFbGVtZW50IHwgbnVsbDtcblxuICAvKipcbiAgICogQnV0dG9uIHRoYXQgcnVucyB0aGUgY29kZSBpbiBhbiBleGFtcGxlJ3MgcGxheWdyb3VuZCwgdGhpcyBlbGVtZW50XG4gICAqIG9ubHkgZXhpc3RzIGluIGV4ZWN1dGFibGUgZXhhbXBsZXMuXG4gICAqL1xuICBwcml2YXRlIHJlYWRvbmx5IHJ1bkJ1dHRvbkVsOiBFbGVtZW50IHwgbnVsbDtcblxuICAvKipcbiAgICogVGhlIGV4ZWN1dGFibGUgY29kZSBvZiBhbiBleGFtcGxlLlxuICAgKi9cbiAgcHJpdmF0ZSByZWFkb25seSBpbnB1dEVsOiBIVE1MVGV4dEFyZWFFbGVtZW50IHwgbnVsbDtcblxuICAvKipcbiAgICogVGhlIG91dHB1dCBvZiB0aGUgZ2l2ZW4gZXhhbXBsZSBjb2RlLiBUaGlzIG9ubHkgZXhpc3RzIGlmIHRoZVxuICAgKiBhdXRob3Igb2YgdGhlIHBhY2thZ2UgcHJvdmlkZXMgYW4gb3V0cHV0IGZvciB0aGlzIGV4YW1wbGUuXG4gICAqL1xuICBwcml2YXRlIHJlYWRvbmx5IG91dHB1dEVsOiBFbGVtZW50IHwgbnVsbDtcblxuICAvKipcbiAgICogQHBhcmFtIGV4YW1wbGVFbCBUaGUgZGl2IHRoYXQgY29udGFpbnMgcGxheWdyb3VuZCBjb250ZW50IGZvciB0aGUgZ2l2ZW4gZXhhbXBsZS5cbiAgICovXG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgcmVhZG9ubHkgZXhhbXBsZUVsOiBIVE1MRGV0YWlsc0VsZW1lbnQpIHtcbiAgICB0aGlzLmV4YW1wbGVFbCA9IGV4YW1wbGVFbDtcbiAgICB0aGlzLmFuY2hvckVsID0gZXhhbXBsZUVsLnF1ZXJ5U2VsZWN0b3IoJ2EnKTtcbiAgICB0aGlzLmVycm9yRWwgPSBleGFtcGxlRWwucXVlcnlTZWxlY3RvcihQbGF5RXhhbXBsZUNsYXNzTmFtZS5FWEFNUExFX0VSUk9SKTtcbiAgICB0aGlzLnBsYXlCdXR0b25FbCA9IGV4YW1wbGVFbC5xdWVyeVNlbGVjdG9yKFBsYXlFeGFtcGxlQ2xhc3NOYW1lLlBMQVlfQlVUVE9OKTtcbiAgICB0aGlzLnNoYXJlQnV0dG9uRWwgPSBleGFtcGxlRWwucXVlcnlTZWxlY3RvcihQbGF5RXhhbXBsZUNsYXNzTmFtZS5TSEFSRV9CVVRUT04pO1xuICAgIHRoaXMuZm9ybWF0QnV0dG9uRWwgPSBleGFtcGxlRWwucXVlcnlTZWxlY3RvcihQbGF5RXhhbXBsZUNsYXNzTmFtZS5GT1JNQVRfQlVUVE9OKTtcbiAgICB0aGlzLnJ1bkJ1dHRvbkVsID0gZXhhbXBsZUVsLnF1ZXJ5U2VsZWN0b3IoUGxheUV4YW1wbGVDbGFzc05hbWUuUlVOX0JVVFRPTik7XG4gICAgdGhpcy5pbnB1dEVsID0gdGhpcy5tYWtlVGV4dEFyZWEoZXhhbXBsZUVsLnF1ZXJ5U2VsZWN0b3IoUGxheUV4YW1wbGVDbGFzc05hbWUuRVhBTVBMRV9JTlBVVCkpO1xuICAgIHRoaXMub3V0cHV0RWwgPSBleGFtcGxlRWwucXVlcnlTZWxlY3RvcihQbGF5RXhhbXBsZUNsYXNzTmFtZS5FWEFNUExFX09VVFBVVCk7XG5cbiAgICAvLyBUaGlzIGlzIGxlZ2FjeSBsaXN0ZW5lciB0byBiZSByZXBsYWNlZCB0aGUgbGlzdGVuZXIgZm9yIHNoYXJlQnV0dG9uRWwuXG4gICAgdGhpcy5wbGF5QnV0dG9uRWw/LmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4gdGhpcy5oYW5kbGVTaGFyZUJ1dHRvbkNsaWNrKCkpO1xuICAgIHRoaXMuc2hhcmVCdXR0b25FbD8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB0aGlzLmhhbmRsZVNoYXJlQnV0dG9uQ2xpY2soKSk7XG4gICAgdGhpcy5mb3JtYXRCdXR0b25FbD8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB0aGlzLmhhbmRsZUZvcm1hdEJ1dHRvbkNsaWNrKCkpO1xuICAgIHRoaXMucnVuQnV0dG9uRWw/LmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4gdGhpcy5oYW5kbGVSdW5CdXR0b25DbGljaygpKTtcblxuICAgIGlmICghdGhpcy5pbnB1dEVsKSByZXR1cm47XG5cbiAgICB0aGlzLnJlc2l6ZSgpO1xuICAgIHRoaXMuaW5wdXRFbC5hZGRFdmVudExpc3RlbmVyKCdrZXl1cCcsICgpID0+IHRoaXMucmVzaXplKCkpO1xuICAgIHRoaXMuaW5wdXRFbC5hZGRFdmVudExpc3RlbmVyKCdrZXlkb3duJywgZSA9PiB0aGlzLm9uS2V5ZG93bihlKSk7XG4gIH1cblxuICAvKipcbiAgICogUmVwbGFjZSB0aGUgcHJlIGVsZW1lbnQgd2l0aCBhIHRleHRhcmVhLiBUaGUgZXhhbXBsZXMgYXJlIGluaXRpYWxseSByZW5kZXJlZFxuICAgKiBhcyBwcmUgZWxlbWVudHMgc28gdGhleSdyZSBmdWxseSB2aXNpYmxlIHdoZW4gSlMgaXMgZGlzYWJsZWQuXG4gICAqL1xuICBtYWtlVGV4dEFyZWEoZWw6IEVsZW1lbnQgfCBudWxsKTogSFRNTFRleHRBcmVhRWxlbWVudCB7XG4gICAgY29uc3QgdCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ3RleHRhcmVhJyk7XG4gICAgdC5jbGFzc0xpc3QuYWRkKCdEb2N1bWVudGF0aW9uLWV4YW1wbGVDb2RlJywgJ2NvZGUnKTtcbiAgICB0LnNwZWxsY2hlY2sgPSBmYWxzZTtcbiAgICB0LnZhbHVlID0gZWw/LnRleHRDb250ZW50ID8/ICcnO1xuICAgIGVsPy5wYXJlbnRFbGVtZW50Py5yZXBsYWNlQ2hpbGQodCwgZWwpO1xuICAgIHJldHVybiB0O1xuICB9XG5cbiAgLyoqXG4gICAqIFJldHJpZXZlIHRoZSBoYXNoIHZhbHVlIG9mIHRoZSBhbmNob3IgZWxlbWVudC5cbiAgICovXG4gIGdldEFuY2hvckhhc2goKTogc3RyaW5nIHwgdW5kZWZpbmVkIHtcbiAgICByZXR1cm4gdGhpcy5hbmNob3JFbD8uaGFzaDtcbiAgfVxuXG4gIC8qKlxuICAgKiBFeHBhbmRzIHRoZSBjdXJyZW50IHBsYXlncm91bmQgZXhhbXBsZS5cbiAgICovXG4gIGV4cGFuZCgpOiB2b2lkIHtcbiAgICB0aGlzLmV4YW1wbGVFbC5vcGVuID0gdHJ1ZTtcbiAgfVxuXG4gIC8qKlxuICAgKiBSZXNpemVzIHRoZSBpbnB1dCBlbGVtZW50IHRvIGFjY29tb2RhdGUgdGhlIGFtb3VudCBvZiB0ZXh0IHByZXNlbnQuXG4gICAqL1xuICBwcml2YXRlIHJlc2l6ZSgpOiB2b2lkIHtcbiAgICBpZiAodGhpcy5pbnB1dEVsPy52YWx1ZSkge1xuICAgICAgY29uc3QgbnVtTGluZUJyZWFrcyA9ICh0aGlzLmlucHV0RWwudmFsdWUubWF0Y2goL1xcbi9nKSB8fCBbXSkubGVuZ3RoO1xuICAgICAgLy8gbWluLWhlaWdodCArIGxpbmVzIHggbGluZS1oZWlnaHQgKyBwYWRkaW5nICsgYm9yZGVyXG4gICAgICB0aGlzLmlucHV0RWwuc3R5bGUuaGVpZ2h0ID0gYCR7KDIwICsgbnVtTGluZUJyZWFrcyAqIDIwICsgMTIgKyAyKSAvIDE2fXJlbWA7XG4gICAgfVxuICB9XG5cbiAgLyoqXG4gICAqIEhhbmRsZXIgdG8gb3ZlcnJpZGUga2V5Ym9hcmQgYmVoYXZpb3IgaW4gdGhlIHBsYXlncm91bmQnc1xuICAgKiB0ZXh0YXJlYSBlbGVtZW50LlxuICAgKlxuICAgKiBUYWIga2V5IGluc2VydHMgdGFicyBpbnRvIHRoZSBleGFtcGxlIHBsYXlncm91bmQgaW5zdGVhZCBvZlxuICAgKiBzd2l0Y2hpbmcgdG8gdGhlIG5leHQgaW50ZXJhY3RpdmUgZWxlbWVudC5cbiAgICogQHBhcmFtIGUgaW5wdXQgZWxlbWVudCBrZXlib2FyZCBldmVudC5cbiAgICovXG4gIHByaXZhdGUgb25LZXlkb3duKGU6IEtleWJvYXJkRXZlbnQpIHtcbiAgICBpZiAoZS5rZXkgPT09ICdUYWInKSB7XG4gICAgICBkb2N1bWVudC5leGVjQ29tbWFuZCgnaW5zZXJ0VGV4dCcsIGZhbHNlLCAnXFx0Jyk7XG4gICAgICBlLnByZXZlbnREZWZhdWx0KCk7XG4gICAgfVxuICB9XG5cbiAgLyoqXG4gICAqIENoYW5nZXMgdGhlIHRleHQgb2YgdGhlIGV4YW1wbGUncyBpbnB1dCBib3guXG4gICAqL1xuICBwcml2YXRlIHNldElucHV0VGV4dChvdXRwdXQ6IHN0cmluZykge1xuICAgIGlmICh0aGlzLmlucHV0RWwpIHtcbiAgICAgIHRoaXMuaW5wdXRFbC52YWx1ZSA9IG91dHB1dDtcbiAgICB9XG4gIH1cblxuICAvKipcbiAgICogQ2hhbmdlcyB0aGUgdGV4dCBvZiB0aGUgZXhhbXBsZSdzIG91dHB1dCBib3guXG4gICAqL1xuICBwcml2YXRlIHNldE91dHB1dFRleHQob3V0cHV0OiBzdHJpbmcpIHtcbiAgICBpZiAodGhpcy5vdXRwdXRFbCkge1xuICAgICAgdGhpcy5vdXRwdXRFbC50ZXh0Q29udGVudCA9IG91dHB1dDtcbiAgICB9XG4gIH1cblxuICAvKipcbiAgICogU2V0cyB0aGUgZXJyb3IgbWVzc2FnZSB0ZXh0IGFuZCBvdmVyd3JpdGVzXG4gICAqIG91dHB1dCBib3ggdG8gaW5kaWNhdGUgYSBmYWlsZWQgcmVzcG9uc2UuXG4gICAqL1xuICBwcml2YXRlIHNldEVycm9yVGV4dChlcnI6IHN0cmluZykge1xuICAgIGlmICh0aGlzLmVycm9yRWwpIHtcbiAgICAgIHRoaXMuZXJyb3JFbC50ZXh0Q29udGVudCA9IGVycjtcbiAgICB9XG4gICAgdGhpcy5zZXRPdXRwdXRUZXh0KCdBbiBlcnJvciBoYXMgb2NjdXJyZWRcdTIwMjYnKTtcbiAgfVxuXG4gIC8qKlxuICAgKiBPcGVucyBhIG5ldyB3aW5kb3cgdG8gcGxheS5nb2xhbmcub3JnIHVzaW5nIHRoZVxuICAgKiBleGFtcGxlIHNuaXBwZXQncyBjb2RlIGluIHRoZSBwbGF5Z3JvdW5kLlxuICAgKi9cbiAgcHJpdmF0ZSBoYW5kbGVTaGFyZUJ1dHRvbkNsaWNrKCkge1xuICAgIGNvbnN0IFBMQVlHUk9VTkRfQkFTRV9VUkwgPSAnaHR0cHM6Ly9wbGF5LmdvbGFuZy5vcmcvcC8nO1xuXG4gICAgdGhpcy5zZXRPdXRwdXRUZXh0KCdXYWl0aW5nIGZvciByZW1vdGUgc2VydmVyXHUyMDI2Jyk7XG5cbiAgICBmZXRjaCgnL3BsYXkvc2hhcmUnLCB7XG4gICAgICBtZXRob2Q6ICdQT1NUJyxcbiAgICAgIGJvZHk6IHRoaXMuaW5wdXRFbD8udmFsdWUsXG4gICAgfSlcbiAgICAgIC50aGVuKHJlcyA9PiByZXMudGV4dCgpKVxuICAgICAgLnRoZW4oc2hhcmVJZCA9PiB7XG4gICAgICAgIGNvbnN0IGhyZWYgPSBQTEFZR1JPVU5EX0JBU0VfVVJMICsgc2hhcmVJZDtcbiAgICAgICAgdGhpcy5zZXRPdXRwdXRUZXh0KGA8YSBocmVmPVwiJHtocmVmfVwiPiR7aHJlZn08L2E+YCk7XG4gICAgICAgIHdpbmRvdy5vcGVuKGhyZWYpO1xuICAgICAgfSlcbiAgICAgIC5jYXRjaChlcnIgPT4ge1xuICAgICAgICB0aGlzLnNldEVycm9yVGV4dChlcnIpO1xuICAgICAgfSk7XG4gIH1cblxuICAvKipcbiAgICogUnVucyBnb2ZtdCBvbiB0aGUgZXhhbXBsZSBzbmlwcGV0IGluIHRoZSBwbGF5Z3JvdW5kLlxuICAgKi9cbiAgcHJpdmF0ZSBoYW5kbGVGb3JtYXRCdXR0b25DbGljaygpIHtcbiAgICB0aGlzLnNldE91dHB1dFRleHQoJ1dhaXRpbmcgZm9yIHJlbW90ZSBzZXJ2ZXJcdTIwMjYnKTtcbiAgICBjb25zdCBib2R5ID0gbmV3IEZvcm1EYXRhKCk7XG4gICAgYm9keS5hcHBlbmQoJ2JvZHknLCB0aGlzLmlucHV0RWw/LnZhbHVlID8/ICcnKTtcblxuICAgIGZldGNoKCcvcGxheS9mbXQnLCB7XG4gICAgICBtZXRob2Q6ICdQT1NUJyxcbiAgICAgIGJvZHk6IGJvZHksXG4gICAgfSlcbiAgICAgIC50aGVuKHJlcyA9PiByZXMuanNvbigpKVxuICAgICAgLnRoZW4oKHsgQm9keSwgRXJyb3IgfSkgPT4ge1xuICAgICAgICB0aGlzLnNldE91dHB1dFRleHQoRXJyb3IgfHwgJ0RvbmUuJyk7XG4gICAgICAgIGlmIChCb2R5KSB7XG4gICAgICAgICAgdGhpcy5zZXRJbnB1dFRleHQoQm9keSk7XG4gICAgICAgICAgdGhpcy5yZXNpemUoKTtcbiAgICAgICAgfVxuICAgICAgfSlcbiAgICAgIC5jYXRjaChlcnIgPT4ge1xuICAgICAgICB0aGlzLnNldEVycm9yVGV4dChlcnIpO1xuICAgICAgfSk7XG4gIH1cblxuICAvKipcbiAgICogUnVucyB0aGUgY29kZSBzbmlwcGV0IGluIHRoZSBleGFtcGxlIHBsYXlncm91bmQuXG4gICAqL1xuICBwcml2YXRlIGhhbmRsZVJ1bkJ1dHRvbkNsaWNrKCkge1xuICAgIHRoaXMuc2V0T3V0cHV0VGV4dCgnV2FpdGluZyBmb3IgcmVtb3RlIHNlcnZlclx1MjAyNicpO1xuXG4gICAgZmV0Y2goJy9wbGF5L2NvbXBpbGUnLCB7XG4gICAgICBtZXRob2Q6ICdQT1NUJyxcbiAgICAgIGJvZHk6IEpTT04uc3RyaW5naWZ5KHsgYm9keTogdGhpcy5pbnB1dEVsPy52YWx1ZSwgdmVyc2lvbjogMiB9KSxcbiAgICB9KVxuICAgICAgLnRoZW4ocmVzID0+IHJlcy5qc29uKCkpXG4gICAgICAudGhlbihhc3luYyAoeyBFdmVudHMsIEVycm9ycyB9KSA9PiB7XG4gICAgICAgIHRoaXMuc2V0T3V0cHV0VGV4dChFcnJvcnMgfHwgJycpO1xuICAgICAgICBmb3IgKGNvbnN0IGUgb2YgRXZlbnRzIHx8IFtdKSB7XG4gICAgICAgICAgdGhpcy5zZXRPdXRwdXRUZXh0KGUuTWVzc2FnZSk7XG4gICAgICAgICAgYXdhaXQgbmV3IFByb21pc2UocmVzb2x2ZSA9PiBzZXRUaW1lb3V0KHJlc29sdmUsIGUuRGVsYXkgLyAxMDAwMDAwKSk7XG4gICAgICAgIH1cbiAgICAgIH0pXG4gICAgICAuY2F0Y2goZXJyID0+IHtcbiAgICAgICAgdGhpcy5zZXRFcnJvclRleHQoZXJyKTtcbiAgICAgIH0pO1xuICB9XG59XG5cbmNvbnN0IGV4YW1wbGVIYXNoUmVnZXggPSBsb2NhdGlvbi5oYXNoLm1hdGNoKC9eIyhleGFtcGxlLS4qKSQvKTtcbmlmIChleGFtcGxlSGFzaFJlZ2V4KSB7XG4gIGNvbnN0IGV4YW1wbGVIYXNoRWwgPSBkb2N1bWVudC5nZXRFbGVtZW50QnlJZChleGFtcGxlSGFzaFJlZ2V4WzFdKSBhcyBIVE1MRGV0YWlsc0VsZW1lbnQ7XG4gIGlmIChleGFtcGxlSGFzaEVsKSB7XG4gICAgZXhhbXBsZUhhc2hFbC5vcGVuID0gdHJ1ZTtcbiAgfVxufVxuXG4vLyBXZSB1c2UgYSBzcHJlYWQgb3BlcmF0b3IgdG8gY29udmVydCBhIG5vZGVsaXN0IGludG8gYW4gYXJyYXkgb2YgZWxlbWVudHMuXG5jb25zdCBleGFtcGxlSHJlZnMgPSBbXG4gIC4uLmRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGw8SFRNTEFuY2hvckVsZW1lbnQ+KFBsYXlFeGFtcGxlQ2xhc3NOYW1lLlBMQVlfSFJFRiksXG5dO1xuXG4vKipcbiAqIFNvbWV0aW1lcyBleGFtcGxlSHJlZnMgYW5kIHBsYXlDb250YWluZXJzIGFyZSBpbiBkaWZmZXJlbnQgb3JkZXIsIHNvIHdlXG4gKiBmaW5kIGFuIGV4YW1wbGVIcmVmIGZyb20gYSBjb21tb24gaGFzaC5cbiAqIEBwYXJhbSBwbGF5Q29udGFpbmVyIC0gcGxheWdyb3VuZCBjb250YWluZXJcbiAqL1xuY29uc3QgZmluZEV4YW1wbGVIYXNoID0gKHBsYXlDb250YWluZXI6IFBsYXlncm91bmRFeGFtcGxlQ29udHJvbGxlcikgPT5cbiAgZXhhbXBsZUhyZWZzLmZpbmQoZXggPT4ge1xuICAgIHJldHVybiBleC5oYXNoID09PSBwbGF5Q29udGFpbmVyLmdldEFuY2hvckhhc2goKTtcbiAgfSk7XG5cbmZvciAoY29uc3QgZWwgb2YgZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbChQbGF5RXhhbXBsZUNsYXNzTmFtZS5QTEFZX0NPTlRBSU5FUikpIHtcbiAgLy8gVGhlcmUgc2hvdWxkIGJlIHRoZSBzYW1lIGFtb3VudCBvZiBocmVmcyByZWZlcmVuY2luZyBleGFtcGxlcyBhcyBleGFtcGxlIGNvbnRhaW5lcnMuXG4gIGNvbnN0IHBsYXlDb250YWluZXIgPSBuZXcgUGxheWdyb3VuZEV4YW1wbGVDb250cm9sbGVyKGVsIGFzIEhUTUxEZXRhaWxzRWxlbWVudCk7XG4gIGNvbnN0IGV4YW1wbGVIcmVmID0gZmluZEV4YW1wbGVIYXNoKHBsYXlDb250YWluZXIpO1xuICBpZiAoZXhhbXBsZUhyZWYpIHtcbiAgICBleGFtcGxlSHJlZi5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICAgIHBsYXlDb250YWluZXIuZXhwYW5kKCk7XG4gICAgfSk7XG4gIH0gZWxzZSB7XG4gICAgY29uc29sZS53YXJuKCdleGFtcGxlIGhyZWYgbm90IGZvdW5kJyk7XG4gIH1cbn1cbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuaW1wb3J0IHsgVHJlZU5hdkNvbnRyb2xsZXIgfSBmcm9tICcuL3RyZWUuanMnO1xuXG5leHBvcnQgY2xhc3MgU2VsZWN0TmF2Q29udHJvbGxlciB7XG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEVsZW1lbnQpIHtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2NoYW5nZScsIGUgPT4ge1xuICAgICAgY29uc3QgdGFyZ2V0ID0gZS50YXJnZXQgYXMgSFRNTFNlbGVjdEVsZW1lbnQ7XG4gICAgICBsZXQgaHJlZiA9IHRhcmdldC52YWx1ZTtcbiAgICAgIGlmICghdGFyZ2V0LnZhbHVlLnN0YXJ0c1dpdGgoJy8nKSkge1xuICAgICAgICBocmVmID0gJy8nICsgaHJlZjtcbiAgICAgIH1cbiAgICAgIHdpbmRvdy5sb2NhdGlvbi5ocmVmID0gaHJlZjtcbiAgICB9KTtcbiAgfVxufVxuXG5leHBvcnQgZnVuY3Rpb24gbWFrZVNlbGVjdE5hdih0cmVlOiBUcmVlTmF2Q29udHJvbGxlcik6IEhUTUxMYWJlbEVsZW1lbnQge1xuICBjb25zdCBsYWJlbCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ2xhYmVsJyk7XG4gIGxhYmVsLmNsYXNzTGlzdC5hZGQoJ2dvLUxhYmVsJyk7XG4gIGxhYmVsLnNldEF0dHJpYnV0ZSgnYXJpYS1sYWJlbCcsICdNZW51Jyk7XG4gIGNvbnN0IHNlbGVjdCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ3NlbGVjdCcpO1xuICBzZWxlY3QuY2xhc3NMaXN0LmFkZCgnZ28tU2VsZWN0JywgJ2pzLXNlbGVjdE5hdicpO1xuICBsYWJlbC5hcHBlbmRDaGlsZChzZWxlY3QpO1xuICBjb25zdCBvdXRsaW5lID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnb3B0Z3JvdXAnKTtcbiAgb3V0bGluZS5sYWJlbCA9ICdPdXRsaW5lJztcbiAgc2VsZWN0LmFwcGVuZENoaWxkKG91dGxpbmUpO1xuICBjb25zdCBncm91cE1hcDogUmVjb3JkPHN0cmluZywgSFRNTE9wdEdyb3VwRWxlbWVudD4gPSB7fTtcbiAgbGV0IGdyb3VwOiBIVE1MT3B0R3JvdXBFbGVtZW50O1xuICBmb3IgKGNvbnN0IHQgb2YgdHJlZS50cmVlaXRlbXMpIHtcbiAgICBpZiAoTnVtYmVyKHQuZGVwdGgpID4gNCkgY29udGludWU7XG4gICAgaWYgKHQuZ3JvdXBUcmVlaXRlbSkge1xuICAgICAgZ3JvdXAgPSBncm91cE1hcFt0Lmdyb3VwVHJlZWl0ZW0ubGFiZWxdO1xuICAgICAgaWYgKCFncm91cCkge1xuICAgICAgICBncm91cCA9IGdyb3VwTWFwW3QuZ3JvdXBUcmVlaXRlbS5sYWJlbF0gPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdvcHRncm91cCcpO1xuICAgICAgICBncm91cC5sYWJlbCA9IHQuZ3JvdXBUcmVlaXRlbS5sYWJlbDtcbiAgICAgICAgc2VsZWN0LmFwcGVuZENoaWxkKGdyb3VwKTtcbiAgICAgIH1cbiAgICB9IGVsc2Uge1xuICAgICAgZ3JvdXAgPSBvdXRsaW5lO1xuICAgIH1cbiAgICBjb25zdCBvID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnb3B0aW9uJyk7XG4gICAgby5sYWJlbCA9IHQubGFiZWw7XG4gICAgby50ZXh0Q29udGVudCA9IHQubGFiZWw7XG4gICAgby52YWx1ZSA9ICh0LmVsIGFzIEhUTUxBbmNob3JFbGVtZW50KS5ocmVmLnJlcGxhY2Uod2luZG93LmxvY2F0aW9uLm9yaWdpbiwgJycpLnJlcGxhY2UoJy8nLCAnJyk7XG4gICAgZ3JvdXAuYXBwZW5kQ2hpbGQobyk7XG4gIH1cbiAgdHJlZS5hZGRPYnNlcnZlcih0ID0+IHtcbiAgICBjb25zdCBoYXNoID0gKHQuZWwgYXMgSFRNTEFuY2hvckVsZW1lbnQpLmhhc2g7XG4gICAgY29uc3QgdmFsdWUgPSBzZWxlY3QucXVlcnlTZWxlY3RvcjxIVE1MT3B0aW9uRWxlbWVudD4oYFt2YWx1ZSQ9XCIke2hhc2h9XCJdYCk/LnZhbHVlO1xuICAgIGlmICh2YWx1ZSkge1xuICAgICAgc2VsZWN0LnZhbHVlID0gdmFsdWU7XG4gICAgfVxuICB9LCA1MCk7XG4gIHJldHVybiBsYWJlbDtcbn1cbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuLyoqXG4gKiBUcmVlTmF2Q29udHJvbGxlciBpcyB0aGUgbmF2aWdhdGlvbiB0cmVlIGNvbXBvbmVudCBvZiB0aGUgZG9jdW1lbnRhdGlvbiBwYWdlLlxuICogSXQgYWRkcyBhY2Nlc3NpYmxpdHkgYXR0cmlidXRlcyB0byBhIHRyZWUsIG9ic2VydmVzIHRoZSBoZWFkaW5nIGVsZW1lbnRzXG4gKiBmb2N1cyB0aGUgdG9wbW9zdCBsaW5rIGZvciBoZWFkaW5ncyB2aXNpYmxlIG9uIHRoZSBwYWdlLCBhbmQgaW1wbGVtZW50cyB0aGVcbiAqIFdBSS1BUklBIFRyZWV2aWV3IERlc2lnbiBQYXR0ZXJuIHdpdGggZnVsbFxuICogW2tleWJvYXJkIHN1cHBvcnRdKGh0dHBzOi8vd3d3LnczLm9yZy9UUi93YWktYXJpYS1wcmFjdGljZXMvZXhhbXBsZXMvdHJlZXZpZXcvdHJlZXZpZXctMi90cmVldmlldy0yYS5odG1sI2tiZF9sYWJlbCkuXG4gKi9cbmV4cG9ydCBjbGFzcyBUcmVlTmF2Q29udHJvbGxlciB7XG4gIHRyZWVpdGVtczogVHJlZUl0ZW1bXTtcblxuICAvKipcbiAgICogZmlyc3RDaGFycyBpcyB0aGUgZmlyc3QgY2hhcmFjdGVyIG9mIGVhY2ggdHJlZWl0ZW0gaW4gdGhlIHNhbWUgb3JkZXJcbiAgICogYXMgdGhpcy50cmVlaXRlbXMuIFdlIHVzZSB0aGlzIGFycmF5IHRvIHNldCBmb2N1cyBieSBjaGFyYWN0ZXIgd2hlblxuICAgKiBuYXZpZ2F0aW5nIHRoZSB0cmVlIHdpdGggYSBrZXlib2FyZC5cbiAgICovXG4gIHByaXZhdGUgZmlyc3RDaGFyczogc3RyaW5nW107XG4gIHByaXZhdGUgZmlyc3RUcmVlaXRlbTogVHJlZUl0ZW0gfCBudWxsO1xuICBwcml2YXRlIGxhc3RUcmVlaXRlbTogVHJlZUl0ZW0gfCBudWxsO1xuICBwcml2YXRlIG9ic2VydmVyQ2FsbGJhY2tzOiAoKHQ6IFRyZWVJdGVtKSA9PiB2b2lkKVtdO1xuXG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEhUTUxFbGVtZW50KSB7XG4gICAgdGhpcy50cmVlaXRlbXMgPSBbXTtcbiAgICB0aGlzLmZpcnN0Q2hhcnMgPSBbXTtcbiAgICB0aGlzLmZpcnN0VHJlZWl0ZW0gPSBudWxsO1xuICAgIHRoaXMubGFzdFRyZWVpdGVtID0gbnVsbDtcbiAgICB0aGlzLm9ic2VydmVyQ2FsbGJhY2tzID0gW107XG4gICAgdGhpcy5pbml0KCk7XG4gIH1cblxuICBwcml2YXRlIGluaXQoKTogdm9pZCB7XG4gICAgdGhpcy5lbC5zdHlsZS5zZXRQcm9wZXJ0eSgnLS1qcy10cmVlLWhlaWdodCcsIHRoaXMuZWwuY2xpZW50SGVpZ2h0ICsgJ3B4Jyk7XG4gICAgdGhpcy5maW5kVHJlZUl0ZW1zKCk7XG4gICAgdGhpcy51cGRhdGVWaXNpYmxlVHJlZWl0ZW1zKCk7XG4gICAgdGhpcy5vYnNlcnZlVGFyZ2V0cygpO1xuICAgIGlmICh0aGlzLmZpcnN0VHJlZWl0ZW0pIHtcbiAgICAgIHRoaXMuZmlyc3RUcmVlaXRlbS5lbC50YWJJbmRleCA9IDA7XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBvYnNlcnZlVGFyZ2V0cygpIHtcbiAgICB0aGlzLmFkZE9ic2VydmVyKHRyZWVpdGVtID0+IHtcbiAgICAgIHRoaXMuZXhwYW5kVHJlZWl0ZW0odHJlZWl0ZW0pO1xuICAgICAgdGhpcy5zZXRTZWxlY3RlZCh0cmVlaXRlbSk7XG4gICAgICAvLyBUT0RPOiBGaXggc2Nyb2xsIGlzc3VlIGluIGh0dHBzOi8vZ29sYW5nLm9yZy9pc3N1ZS80NzQ1MC5cbiAgICAgIC8vIHRyZWVpdGVtLmVsLnNjcm9sbEludG9WaWV3KHsgYmxvY2s6ICduZWFyZXN0JyB9KTtcbiAgICB9KTtcblxuICAgIGNvbnN0IHRhcmdldHMgPSBuZXcgTWFwPHN0cmluZywgYm9vbGVhbj4oKTtcbiAgICBjb25zdCBvYnNlcnZlciA9IG5ldyBJbnRlcnNlY3Rpb25PYnNlcnZlcihcbiAgICAgIGVudHJpZXMgPT4ge1xuICAgICAgICBmb3IgKGNvbnN0IGVudHJ5IG9mIGVudHJpZXMpIHtcbiAgICAgICAgICB0YXJnZXRzLnNldChlbnRyeS50YXJnZXQuaWQsIGVudHJ5LmlzSW50ZXJzZWN0aW5nIHx8IGVudHJ5LmludGVyc2VjdGlvblJhdGlvID09PSAxKTtcbiAgICAgICAgfVxuICAgICAgICBmb3IgKGNvbnN0IFtpZCwgaXNJbnRlcnNlY3RpbmddIG9mIHRhcmdldHMpIHtcbiAgICAgICAgICBpZiAoaXNJbnRlcnNlY3RpbmcpIHtcbiAgICAgICAgICAgIGNvbnN0IGFjdGl2ZSA9IHRoaXMudHJlZWl0ZW1zLmZpbmQodCA9PlxuICAgICAgICAgICAgICAodC5lbCBhcyBIVE1MQW5jaG9yRWxlbWVudCk/LmhyZWYuZW5kc1dpdGgoYCMke2lkfWApXG4gICAgICAgICAgICApO1xuICAgICAgICAgICAgaWYgKGFjdGl2ZSkge1xuICAgICAgICAgICAgICBmb3IgKGNvbnN0IGZuIG9mIHRoaXMub2JzZXJ2ZXJDYWxsYmFja3MpIHtcbiAgICAgICAgICAgICAgICBmbihhY3RpdmUpO1xuICAgICAgICAgICAgICB9XG4gICAgICAgICAgICB9XG4gICAgICAgICAgICBicmVhaztcbiAgICAgICAgICB9XG4gICAgICAgIH1cbiAgICAgIH0sXG4gICAgICB7XG4gICAgICAgIHRocmVzaG9sZDogMS4wLFxuICAgICAgICByb290TWFyZ2luOiAnLTYwcHggMHB4IDBweCAwcHgnLFxuICAgICAgfVxuICAgICk7XG5cbiAgICBmb3IgKGNvbnN0IGhyZWYgb2YgdGhpcy50cmVlaXRlbXMubWFwKHQgPT4gdC5lbC5nZXRBdHRyaWJ1dGUoJ2hyZWYnKSkpIHtcbiAgICAgIGlmIChocmVmKSB7XG4gICAgICAgIGNvbnN0IGlkID0gaHJlZi5yZXBsYWNlKHdpbmRvdy5sb2NhdGlvbi5vcmlnaW4sICcnKS5yZXBsYWNlKCcvJywgJycpLnJlcGxhY2UoJyMnLCAnJyk7XG4gICAgICAgIGNvbnN0IHRhcmdldCA9IGRvY3VtZW50LmdldEVsZW1lbnRCeUlkKGlkKTtcbiAgICAgICAgaWYgKHRhcmdldCkge1xuICAgICAgICAgIG9ic2VydmVyLm9ic2VydmUodGFyZ2V0KTtcbiAgICAgICAgfVxuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIGFkZE9ic2VydmVyKGZuOiAodDogVHJlZUl0ZW0pID0+IHZvaWQsIGRlbGF5ID0gMjAwKTogdm9pZCB7XG4gICAgdGhpcy5vYnNlcnZlckNhbGxiYWNrcy5wdXNoKGRlYm91bmNlKGZuLCBkZWxheSkpO1xuICB9XG5cbiAgc2V0Rm9jdXNUb05leHRJdGVtKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBuZXh0SXRlbSA9IG51bGw7XG4gICAgZm9yIChsZXQgaSA9IGN1cnJlbnRJdGVtLmluZGV4ICsgMTsgaSA8IHRoaXMudHJlZWl0ZW1zLmxlbmd0aDsgaSsrKSB7XG4gICAgICBjb25zdCB0aSA9IHRoaXMudHJlZWl0ZW1zW2ldO1xuICAgICAgaWYgKHRpLmlzVmlzaWJsZSkge1xuICAgICAgICBuZXh0SXRlbSA9IHRpO1xuICAgICAgICBicmVhaztcbiAgICAgIH1cbiAgICB9XG4gICAgaWYgKG5leHRJdGVtKSB7XG4gICAgICB0aGlzLnNldEZvY3VzVG9JdGVtKG5leHRJdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c1RvUHJldmlvdXNJdGVtKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGxldCBwcmV2SXRlbSA9IG51bGw7XG4gICAgZm9yIChsZXQgaSA9IGN1cnJlbnRJdGVtLmluZGV4IC0gMTsgaSA+IC0xOyBpLS0pIHtcbiAgICAgIGNvbnN0IHRpID0gdGhpcy50cmVlaXRlbXNbaV07XG4gICAgICBpZiAodGkuaXNWaXNpYmxlKSB7XG4gICAgICAgIHByZXZJdGVtID0gdGk7XG4gICAgICAgIGJyZWFrO1xuICAgICAgfVxuICAgIH1cbiAgICBpZiAocHJldkl0ZW0pIHtcbiAgICAgIHRoaXMuc2V0Rm9jdXNUb0l0ZW0ocHJldkl0ZW0pO1xuICAgIH1cbiAgfVxuXG4gIHNldEZvY3VzVG9QYXJlbnRJdGVtKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGlmIChjdXJyZW50SXRlbS5ncm91cFRyZWVpdGVtKSB7XG4gICAgICB0aGlzLnNldEZvY3VzVG9JdGVtKGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW0pO1xuICAgIH1cbiAgfVxuXG4gIHNldEZvY3VzVG9GaXJzdEl0ZW0oKTogdm9pZCB7XG4gICAgdGhpcy5maXJzdFRyZWVpdGVtICYmIHRoaXMuc2V0Rm9jdXNUb0l0ZW0odGhpcy5maXJzdFRyZWVpdGVtKTtcbiAgfVxuXG4gIHNldEZvY3VzVG9MYXN0SXRlbSgpOiB2b2lkIHtcbiAgICB0aGlzLmxhc3RUcmVlaXRlbSAmJiB0aGlzLnNldEZvY3VzVG9JdGVtKHRoaXMubGFzdFRyZWVpdGVtKTtcbiAgfVxuXG4gIHNldFNlbGVjdGVkKGN1cnJlbnRJdGVtOiBUcmVlSXRlbSk6IHZvaWQge1xuICAgIGZvciAoY29uc3QgbDEgb2YgdGhpcy5lbC5xdWVyeVNlbGVjdG9yQWxsKCdbYXJpYS1leHBhbmRlZD1cInRydWVcIl0nKSkge1xuICAgICAgaWYgKGwxID09PSBjdXJyZW50SXRlbS5lbCkgY29udGludWU7XG4gICAgICBpZiAoIWwxLm5leHRFbGVtZW50U2libGluZz8uY29udGFpbnMoY3VycmVudEl0ZW0uZWwpKSB7XG4gICAgICAgIGwxLnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICdmYWxzZScpO1xuICAgICAgfVxuICAgIH1cbiAgICBmb3IgKGNvbnN0IGwxIG9mIHRoaXMuZWwucXVlcnlTZWxlY3RvckFsbCgnW2FyaWEtc2VsZWN0ZWRdJykpIHtcbiAgICAgIGlmIChsMSAhPT0gY3VycmVudEl0ZW0uZWwpIHtcbiAgICAgICAgbDEuc2V0QXR0cmlidXRlKCdhcmlhLXNlbGVjdGVkJywgJ2ZhbHNlJyk7XG4gICAgICB9XG4gICAgfVxuICAgIGN1cnJlbnRJdGVtLmVsLnNldEF0dHJpYnV0ZSgnYXJpYS1zZWxlY3RlZCcsICd0cnVlJyk7XG4gICAgdGhpcy51cGRhdGVWaXNpYmxlVHJlZWl0ZW1zKCk7XG4gICAgdGhpcy5zZXRGb2N1c1RvSXRlbShjdXJyZW50SXRlbSwgZmFsc2UpO1xuICB9XG5cbiAgZXhwYW5kVHJlZWl0ZW0odHJlZWl0ZW06IFRyZWVJdGVtKTogdm9pZCB7XG4gICAgbGV0IGN1cnJlbnRJdGVtOiBUcmVlSXRlbSB8IG51bGwgPSB0cmVlaXRlbTtcbiAgICB3aGlsZSAoY3VycmVudEl0ZW0pIHtcbiAgICAgIGlmIChjdXJyZW50SXRlbS5pc0V4cGFuZGFibGUpIHtcbiAgICAgICAgY3VycmVudEl0ZW0uZWwuc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgJ3RydWUnKTtcbiAgICAgIH1cbiAgICAgIGN1cnJlbnRJdGVtID0gY3VycmVudEl0ZW0uZ3JvdXBUcmVlaXRlbTtcbiAgICB9XG4gICAgdGhpcy51cGRhdGVWaXNpYmxlVHJlZWl0ZW1zKCk7XG4gIH1cblxuICBleHBhbmRBbGxTaWJsaW5nSXRlbXMoY3VycmVudEl0ZW06IFRyZWVJdGVtKTogdm9pZCB7XG4gICAgZm9yIChjb25zdCB0aSBvZiB0aGlzLnRyZWVpdGVtcykge1xuICAgICAgaWYgKHRpLmdyb3VwVHJlZWl0ZW0gPT09IGN1cnJlbnRJdGVtLmdyb3VwVHJlZWl0ZW0gJiYgdGkuaXNFeHBhbmRhYmxlKSB7XG4gICAgICAgIHRoaXMuZXhwYW5kVHJlZWl0ZW0odGkpO1xuICAgICAgfVxuICAgIH1cbiAgfVxuXG4gIGNvbGxhcHNlVHJlZWl0ZW0oY3VycmVudEl0ZW06IFRyZWVJdGVtKTogdm9pZCB7XG4gICAgbGV0IGdyb3VwVHJlZWl0ZW0gPSBudWxsO1xuXG4gICAgaWYgKGN1cnJlbnRJdGVtLmlzRXhwYW5kZWQoKSkge1xuICAgICAgZ3JvdXBUcmVlaXRlbSA9IGN1cnJlbnRJdGVtO1xuICAgIH0gZWxzZSB7XG4gICAgICBncm91cFRyZWVpdGVtID0gY3VycmVudEl0ZW0uZ3JvdXBUcmVlaXRlbTtcbiAgICB9XG5cbiAgICBpZiAoZ3JvdXBUcmVlaXRlbSkge1xuICAgICAgZ3JvdXBUcmVlaXRlbS5lbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnLCAnZmFsc2UnKTtcbiAgICAgIHRoaXMudXBkYXRlVmlzaWJsZVRyZWVpdGVtcygpO1xuICAgICAgdGhpcy5zZXRGb2N1c1RvSXRlbShncm91cFRyZWVpdGVtKTtcbiAgICB9XG4gIH1cblxuICBzZXRGb2N1c0J5Rmlyc3RDaGFyYWN0ZXIoY3VycmVudEl0ZW06IFRyZWVJdGVtLCBjaGFyOiBzdHJpbmcpOiB2b2lkIHtcbiAgICBsZXQgc3RhcnQ6IG51bWJlciwgaW5kZXg6IG51bWJlcjtcbiAgICBjaGFyID0gY2hhci50b0xvd2VyQ2FzZSgpO1xuXG4gICAgLy8gR2V0IHN0YXJ0IGluZGV4IGZvciBzZWFyY2ggYmFzZWQgb24gcG9zaXRpb24gb2YgY3VycmVudEl0ZW1cbiAgICBzdGFydCA9IGN1cnJlbnRJdGVtLmluZGV4ICsgMTtcbiAgICBpZiAoc3RhcnQgPT09IHRoaXMudHJlZWl0ZW1zLmxlbmd0aCkge1xuICAgICAgc3RhcnQgPSAwO1xuICAgIH1cblxuICAgIC8vIENoZWNrIHJlbWFpbmluZyBzbG90cyBpbiB0aGUgbWVudVxuICAgIGluZGV4ID0gdGhpcy5nZXRJbmRleEZpcnN0Q2hhcnMoc3RhcnQsIGNoYXIpO1xuXG4gICAgLy8gSWYgbm90IGZvdW5kIGluIHJlbWFpbmluZyBzbG90cywgY2hlY2sgZnJvbSBiZWdpbm5pbmdcbiAgICBpZiAoaW5kZXggPT09IC0xKSB7XG4gICAgICBpbmRleCA9IHRoaXMuZ2V0SW5kZXhGaXJzdENoYXJzKDAsIGNoYXIpO1xuICAgIH1cblxuICAgIC8vIElmIG1hdGNoIHdhcyBmb3VuZC4uLlxuICAgIGlmIChpbmRleCA+IC0xKSB7XG4gICAgICB0aGlzLnNldEZvY3VzVG9JdGVtKHRoaXMudHJlZWl0ZW1zW2luZGV4XSk7XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBmaW5kVHJlZUl0ZW1zKCkge1xuICAgIGNvbnN0IGZpbmRJdGVtcyA9IChlbDogSFRNTEVsZW1lbnQsIGdyb3VwOiBUcmVlSXRlbSB8IG51bGwpID0+IHtcbiAgICAgIGxldCB0aSA9IGdyb3VwO1xuICAgICAgbGV0IGN1cnIgPSBlbC5maXJzdEVsZW1lbnRDaGlsZCBhcyBIVE1MRWxlbWVudDtcbiAgICAgIHdoaWxlIChjdXJyKSB7XG4gICAgICAgIGlmIChjdXJyLnRhZ05hbWUgPT09ICdBJyB8fCBjdXJyLnRhZ05hbWUgPT09ICdTUEFOJykge1xuICAgICAgICAgIHRpID0gbmV3IFRyZWVJdGVtKGN1cnIsIHRoaXMsIGdyb3VwKTtcbiAgICAgICAgICB0aGlzLnRyZWVpdGVtcy5wdXNoKHRpKTtcbiAgICAgICAgICB0aGlzLmZpcnN0Q2hhcnMucHVzaCh0aS5sYWJlbC5zdWJzdHJpbmcoMCwgMSkudG9Mb3dlckNhc2UoKSk7XG4gICAgICAgIH1cbiAgICAgICAgaWYgKGN1cnIuZmlyc3RFbGVtZW50Q2hpbGQpIHtcbiAgICAgICAgICBmaW5kSXRlbXMoY3VyciwgdGkpO1xuICAgICAgICB9XG4gICAgICAgIGN1cnIgPSBjdXJyLm5leHRFbGVtZW50U2libGluZyBhcyBIVE1MRWxlbWVudDtcbiAgICAgIH1cbiAgICB9O1xuICAgIGZpbmRJdGVtcyh0aGlzLmVsIGFzIEhUTUxFbGVtZW50LCBudWxsKTtcbiAgICB0aGlzLnRyZWVpdGVtcy5tYXAoKHRpLCBpZHgpID0+ICh0aS5pbmRleCA9IGlkeCkpO1xuICB9XG5cbiAgcHJpdmF0ZSB1cGRhdGVWaXNpYmxlVHJlZWl0ZW1zKCk6IHZvaWQge1xuICAgIHRoaXMuZmlyc3RUcmVlaXRlbSA9IHRoaXMudHJlZWl0ZW1zWzBdO1xuXG4gICAgZm9yIChjb25zdCB0aSBvZiB0aGlzLnRyZWVpdGVtcykge1xuICAgICAgbGV0IHBhcmVudCA9IHRpLmdyb3VwVHJlZWl0ZW07XG4gICAgICB0aS5pc1Zpc2libGUgPSB0cnVlO1xuICAgICAgd2hpbGUgKHBhcmVudCAmJiBwYXJlbnQuZWwgIT09IHRoaXMuZWwpIHtcbiAgICAgICAgaWYgKCFwYXJlbnQuaXNFeHBhbmRlZCgpKSB7XG4gICAgICAgICAgdGkuaXNWaXNpYmxlID0gZmFsc2U7XG4gICAgICAgIH1cbiAgICAgICAgcGFyZW50ID0gcGFyZW50Lmdyb3VwVHJlZWl0ZW07XG4gICAgICB9XG4gICAgICBpZiAodGkuaXNWaXNpYmxlKSB7XG4gICAgICAgIHRoaXMubGFzdFRyZWVpdGVtID0gdGk7XG4gICAgICB9XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBzZXRGb2N1c1RvSXRlbSh0cmVlaXRlbTogVHJlZUl0ZW0sIGZvY3VzRWwgPSB0cnVlKSB7XG4gICAgdHJlZWl0ZW0uZWwudGFiSW5kZXggPSAwO1xuICAgIGlmIChmb2N1c0VsKSB7XG4gICAgICB0cmVlaXRlbS5lbC5mb2N1cygpO1xuICAgIH1cbiAgICBmb3IgKGNvbnN0IHRpIG9mIHRoaXMudHJlZWl0ZW1zKSB7XG4gICAgICBpZiAodGkgIT09IHRyZWVpdGVtKSB7XG4gICAgICAgIHRpLmVsLnRhYkluZGV4ID0gLTE7XG4gICAgICB9XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBnZXRJbmRleEZpcnN0Q2hhcnMoc3RhcnRJbmRleDogbnVtYmVyLCBjaGFyOiBzdHJpbmcpOiBudW1iZXIge1xuICAgIGZvciAobGV0IGkgPSBzdGFydEluZGV4OyBpIDwgdGhpcy5maXJzdENoYXJzLmxlbmd0aDsgaSsrKSB7XG4gICAgICBpZiAodGhpcy50cmVlaXRlbXNbaV0uaXNWaXNpYmxlICYmIGNoYXIgPT09IHRoaXMuZmlyc3RDaGFyc1tpXSkge1xuICAgICAgICByZXR1cm4gaTtcbiAgICAgIH1cbiAgICB9XG4gICAgcmV0dXJuIC0xO1xuICB9XG59XG5cbmNsYXNzIFRyZWVJdGVtIHtcbiAgZWw6IEhUTUxFbGVtZW50O1xuICBncm91cFRyZWVpdGVtOiBUcmVlSXRlbSB8IG51bGw7XG4gIGxhYmVsOiBzdHJpbmc7XG4gIGlzRXhwYW5kYWJsZTogYm9vbGVhbjtcbiAgaXNWaXNpYmxlOiBib29sZWFuO1xuICBkZXB0aDogbnVtYmVyO1xuICBpbmRleDogbnVtYmVyO1xuXG4gIHByaXZhdGUgdHJlZTogVHJlZU5hdkNvbnRyb2xsZXI7XG4gIHByaXZhdGUgaXNJbkdyb3VwOiBib29sZWFuO1xuXG4gIGNvbnN0cnVjdG9yKGVsOiBIVE1MRWxlbWVudCwgdHJlZU9iajogVHJlZU5hdkNvbnRyb2xsZXIsIGdyb3VwOiBUcmVlSXRlbSB8IG51bGwpIHtcbiAgICBlbC50YWJJbmRleCA9IC0xO1xuICAgIHRoaXMuZWwgPSBlbDtcbiAgICB0aGlzLmdyb3VwVHJlZWl0ZW0gPSBncm91cDtcbiAgICB0aGlzLmxhYmVsID0gZWwudGV4dENvbnRlbnQ/LnRyaW0oKSA/PyAnJztcbiAgICB0aGlzLnRyZWUgPSB0cmVlT2JqO1xuICAgIHRoaXMuZGVwdGggPSAoZ3JvdXA/LmRlcHRoIHx8IDApICsgMTtcbiAgICB0aGlzLmluZGV4ID0gMDtcblxuICAgIGNvbnN0IHBhcmVudCA9IGVsLnBhcmVudEVsZW1lbnQ7XG4gICAgaWYgKHBhcmVudD8udGFnTmFtZS50b0xvd2VyQ2FzZSgpID09PSAnbGknKSB7XG4gICAgICBwYXJlbnQ/LnNldEF0dHJpYnV0ZSgncm9sZScsICdub25lJyk7XG4gICAgfVxuICAgIGVsLnNldEF0dHJpYnV0ZSgnYXJpYS1sZXZlbCcsIHRoaXMuZGVwdGggKyAnJyk7XG4gICAgaWYgKGVsLmdldEF0dHJpYnV0ZSgnYXJpYS1sYWJlbCcpKSB7XG4gICAgICB0aGlzLmxhYmVsID0gZWw/LmdldEF0dHJpYnV0ZSgnYXJpYS1sYWJlbCcpPy50cmltKCkgPz8gJyc7XG4gICAgfVxuXG4gICAgdGhpcy5pc0V4cGFuZGFibGUgPSBmYWxzZTtcbiAgICB0aGlzLmlzVmlzaWJsZSA9IGZhbHNlO1xuICAgIHRoaXMuaXNJbkdyb3VwID0gISFncm91cDtcblxuICAgIGxldCBjdXJyID0gZWwubmV4dEVsZW1lbnRTaWJsaW5nO1xuICAgIHdoaWxlIChjdXJyKSB7XG4gICAgICBpZiAoY3Vyci50YWdOYW1lLnRvTG93ZXJDYXNlKCkgPT0gJ3VsJykge1xuICAgICAgICBjb25zdCBncm91cElkID0gYCR7Z3JvdXA/LmxhYmVsID8/ICcnfSBuYXYgZ3JvdXAgJHt0aGlzLmxhYmVsfWAucmVwbGFjZSgvW1xcV19dKy9nLCAnXycpO1xuICAgICAgICBlbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtb3ducycsIGdyb3VwSWQpO1xuICAgICAgICBlbC5zZXRBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnLCAnZmFsc2UnKTtcbiAgICAgICAgY3Vyci5zZXRBdHRyaWJ1dGUoJ3JvbGUnLCAnZ3JvdXAnKTtcbiAgICAgICAgY3Vyci5zZXRBdHRyaWJ1dGUoJ2lkJywgZ3JvdXBJZCk7XG4gICAgICAgIHRoaXMuaXNFeHBhbmRhYmxlID0gdHJ1ZTtcbiAgICAgICAgYnJlYWs7XG4gICAgICB9XG5cbiAgICAgIGN1cnIgPSBjdXJyLm5leHRFbGVtZW50U2libGluZztcbiAgICB9XG4gICAgdGhpcy5pbml0KCk7XG4gIH1cblxuICBwcml2YXRlIGluaXQoKSB7XG4gICAgdGhpcy5lbC50YWJJbmRleCA9IC0xO1xuICAgIGlmICghdGhpcy5lbC5nZXRBdHRyaWJ1dGUoJ3JvbGUnKSkge1xuICAgICAgdGhpcy5lbC5zZXRBdHRyaWJ1dGUoJ3JvbGUnLCAndHJlZWl0ZW0nKTtcbiAgICB9XG4gICAgdGhpcy5lbC5hZGRFdmVudExpc3RlbmVyKCdrZXlkb3duJywgdGhpcy5oYW5kbGVLZXlkb3duLmJpbmQodGhpcykpO1xuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCB0aGlzLmhhbmRsZUNsaWNrLmJpbmQodGhpcykpO1xuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcignZm9jdXMnLCB0aGlzLmhhbmRsZUZvY3VzLmJpbmQodGhpcykpO1xuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcignYmx1cicsIHRoaXMuaGFuZGxlQmx1ci5iaW5kKHRoaXMpKTtcbiAgfVxuXG4gIGlzRXhwYW5kZWQoKSB7XG4gICAgaWYgKHRoaXMuaXNFeHBhbmRhYmxlKSB7XG4gICAgICByZXR1cm4gdGhpcy5lbC5nZXRBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnKSA9PT0gJ3RydWUnO1xuICAgIH1cblxuICAgIHJldHVybiBmYWxzZTtcbiAgfVxuXG4gIGlzU2VsZWN0ZWQoKSB7XG4gICAgcmV0dXJuIHRoaXMuZWwuZ2V0QXR0cmlidXRlKCdhcmlhLXNlbGVjdGVkJykgPT09ICd0cnVlJztcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlQ2xpY2soZXZlbnQ6IE1vdXNlRXZlbnQpIHtcbiAgICAvLyBvbmx5IHByb2Nlc3MgY2xpY2sgZXZlbnRzIHRoYXQgZGlyZWN0bHkgaGFwcGVuZWQgb24gdGhpcyB0cmVlaXRlbVxuICAgIGlmIChldmVudC50YXJnZXQgIT09IHRoaXMuZWwgJiYgZXZlbnQudGFyZ2V0ICE9PSB0aGlzLmVsLmZpcnN0RWxlbWVudENoaWxkKSB7XG4gICAgICByZXR1cm47XG4gICAgfVxuICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgaWYgKHRoaXMuaXNFeHBhbmRlZCgpICYmIHRoaXMuaXNTZWxlY3RlZCgpKSB7XG4gICAgICAgIHRoaXMudHJlZS5jb2xsYXBzZVRyZWVpdGVtKHRoaXMpO1xuICAgICAgfSBlbHNlIHtcbiAgICAgICAgdGhpcy50cmVlLmV4cGFuZFRyZWVpdGVtKHRoaXMpO1xuICAgICAgfVxuICAgICAgZXZlbnQuc3RvcFByb3BhZ2F0aW9uKCk7XG4gICAgfVxuICAgIHRoaXMudHJlZS5zZXRTZWxlY3RlZCh0aGlzKTtcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlRm9jdXMoKSB7XG4gICAgbGV0IGVsID0gdGhpcy5lbDtcbiAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUpIHtcbiAgICAgIGVsID0gKGVsLmZpcnN0RWxlbWVudENoaWxkIGFzIEhUTUxFbGVtZW50KSA/PyBlbDtcbiAgICB9XG4gICAgZWwuY2xhc3NMaXN0LmFkZCgnZm9jdXMnKTtcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlQmx1cigpIHtcbiAgICBsZXQgZWwgPSB0aGlzLmVsO1xuICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgZWwgPSAoZWwuZmlyc3RFbGVtZW50Q2hpbGQgYXMgSFRNTEVsZW1lbnQpID8/IGVsO1xuICAgIH1cbiAgICBlbC5jbGFzc0xpc3QucmVtb3ZlKCdmb2N1cycpO1xuICB9XG5cbiAgcHJpdmF0ZSBoYW5kbGVLZXlkb3duKGV2ZW50OiBLZXlib2FyZEV2ZW50KSB7XG4gICAgaWYgKGV2ZW50LmFsdEtleSB8fCBldmVudC5jdHJsS2V5IHx8IGV2ZW50Lm1ldGFLZXkpIHtcbiAgICAgIHJldHVybjtcbiAgICB9XG5cbiAgICBsZXQgY2FwdHVyZWQgPSBmYWxzZTtcbiAgICBzd2l0Y2ggKGV2ZW50LmtleSkge1xuICAgICAgY2FzZSAnICc6XG4gICAgICBjYXNlICdFbnRlcic6XG4gICAgICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgICAgIGlmICh0aGlzLmlzRXhwYW5kZWQoKSAmJiB0aGlzLmlzU2VsZWN0ZWQoKSkge1xuICAgICAgICAgICAgdGhpcy50cmVlLmNvbGxhcHNlVHJlZWl0ZW0odGhpcyk7XG4gICAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5leHBhbmRUcmVlaXRlbSh0aGlzKTtcbiAgICAgICAgICB9XG4gICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgIGV2ZW50LnN0b3BQcm9wYWdhdGlvbigpO1xuICAgICAgICB9XG4gICAgICAgIHRoaXMudHJlZS5zZXRTZWxlY3RlZCh0aGlzKTtcbiAgICAgICAgYnJlYWs7XG5cbiAgICAgIGNhc2UgJ0Fycm93VXAnOlxuICAgICAgICB0aGlzLnRyZWUuc2V0Rm9jdXNUb1ByZXZpb3VzSXRlbSh0aGlzKTtcbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnQXJyb3dEb3duJzpcbiAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9OZXh0SXRlbSh0aGlzKTtcbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnQXJyb3dSaWdodCc6XG4gICAgICAgIGlmICh0aGlzLmlzRXhwYW5kYWJsZSkge1xuICAgICAgICAgIGlmICh0aGlzLmlzRXhwYW5kZWQoKSkge1xuICAgICAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9OZXh0SXRlbSh0aGlzKTtcbiAgICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgICAgdGhpcy50cmVlLmV4cGFuZFRyZWVpdGVtKHRoaXMpO1xuICAgICAgICAgIH1cbiAgICAgICAgfVxuICAgICAgICBjYXB0dXJlZCA9IHRydWU7XG4gICAgICAgIGJyZWFrO1xuXG4gICAgICBjYXNlICdBcnJvd0xlZnQnOlxuICAgICAgICBpZiAodGhpcy5pc0V4cGFuZGFibGUgJiYgdGhpcy5pc0V4cGFuZGVkKCkpIHtcbiAgICAgICAgICB0aGlzLnRyZWUuY29sbGFwc2VUcmVlaXRlbSh0aGlzKTtcbiAgICAgICAgICBjYXB0dXJlZCA9IHRydWU7XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgaWYgKHRoaXMuaXNJbkdyb3VwKSB7XG4gICAgICAgICAgICB0aGlzLnRyZWUuc2V0Rm9jdXNUb1BhcmVudEl0ZW0odGhpcyk7XG4gICAgICAgICAgICBjYXB0dXJlZCA9IHRydWU7XG4gICAgICAgICAgfVxuICAgICAgICB9XG4gICAgICAgIGJyZWFrO1xuXG4gICAgICBjYXNlICdIb21lJzpcbiAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9GaXJzdEl0ZW0oKTtcbiAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICBicmVhaztcblxuICAgICAgY2FzZSAnRW5kJzpcbiAgICAgICAgdGhpcy50cmVlLnNldEZvY3VzVG9MYXN0SXRlbSgpO1xuICAgICAgICBjYXB0dXJlZCA9IHRydWU7XG4gICAgICAgIGJyZWFrO1xuXG4gICAgICBkZWZhdWx0OlxuICAgICAgICBpZiAoZXZlbnQua2V5Lmxlbmd0aCA9PT0gMSAmJiBldmVudC5rZXkubWF0Y2goL1xcUy8pKSB7XG4gICAgICAgICAgaWYgKGV2ZW50LmtleSA9PSAnKicpIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5leHBhbmRBbGxTaWJsaW5nSXRlbXModGhpcyk7XG4gICAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICAgIHRoaXMudHJlZS5zZXRGb2N1c0J5Rmlyc3RDaGFyYWN0ZXIodGhpcywgZXZlbnQua2V5KTtcbiAgICAgICAgICB9XG4gICAgICAgICAgY2FwdHVyZWQgPSB0cnVlO1xuICAgICAgICB9XG4gICAgICAgIGJyZWFrO1xuICAgIH1cblxuICAgIGlmIChjYXB0dXJlZCkge1xuICAgICAgZXZlbnQuc3RvcFByb3BhZ2F0aW9uKCk7XG4gICAgICBldmVudC5wcmV2ZW50RGVmYXVsdCgpO1xuICAgIH1cbiAgfVxufVxuXG4vLyBlc2xpbnQtZGlzYWJsZS1uZXh0LWxpbmUgQHR5cGVzY3JpcHQtZXNsaW50L25vLWV4cGxpY2l0LWFueVxuZnVuY3Rpb24gZGVib3VuY2U8VCBleHRlbmRzICguLi5hcmdzOiBhbnlbXSkgPT4gYW55PihmdW5jOiBULCB3YWl0OiBudW1iZXIpIHtcbiAgbGV0IHRpbWVvdXQ6IFJldHVyblR5cGU8dHlwZW9mIHNldFRpbWVvdXQ+IHwgbnVsbDtcbiAgcmV0dXJuICguLi5hcmdzOiBQYXJhbWV0ZXJzPFQ+KSA9PiB7XG4gICAgY29uc3QgbGF0ZXIgPSAoKSA9PiB7XG4gICAgICB0aW1lb3V0ID0gbnVsbDtcbiAgICAgIGZ1bmMoLi4uYXJncyk7XG4gICAgfTtcbiAgICBpZiAodGltZW91dCkge1xuICAgICAgY2xlYXJUaW1lb3V0KHRpbWVvdXQpO1xuICAgIH1cbiAgICB0aW1lb3V0ID0gc2V0VGltZW91dChsYXRlciwgd2FpdCk7XG4gIH07XG59XG4iLCAiaW1wb3J0ICcuLi8uLi8uLi9zaGFyZWQvanVtcC9qdW1wJztcbmltcG9ydCAnLi4vLi4vLi4vc2hhcmVkL3BsYXlncm91bmQvcGxheWdyb3VuZCc7XG5cbmltcG9ydCB7IFNlbGVjdE5hdkNvbnRyb2xsZXIsIG1ha2VTZWxlY3ROYXYgfSBmcm9tICcuLi8uLi8uLi9zaGFyZWQvb3V0bGluZS9zZWxlY3QnO1xuaW1wb3J0IHsgVHJlZU5hdkNvbnRyb2xsZXIgfSBmcm9tICcuLi8uLi8uLi9zaGFyZWQvb3V0bGluZS90cmVlJztcblxuY29uc3QgdHJlZUVsID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MRWxlbWVudD4oJy5qcy10cmVlJyk7XG5pZiAodHJlZUVsKSB7XG4gIGNvbnN0IHRyZWVDdHJsID0gbmV3IFRyZWVOYXZDb250cm9sbGVyKHRyZWVFbCk7XG4gIGNvbnN0IHNlbGVjdCA9IG1ha2VTZWxlY3ROYXYodHJlZUN0cmwpO1xuICBjb25zdCBtb2JpbGVOYXYgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuanMtbWFpbk5hdk1vYmlsZScpO1xuICBpZiAobW9iaWxlTmF2ICYmIG1vYmlsZU5hdi5maXJzdEVsZW1lbnRDaGlsZCkge1xuICAgIG1vYmlsZU5hdj8ucmVwbGFjZUNoaWxkKHNlbGVjdCwgbW9iaWxlTmF2LmZpcnN0RWxlbWVudENoaWxkKTtcbiAgfVxuICBpZiAoc2VsZWN0LmZpcnN0RWxlbWVudENoaWxkKSB7XG4gICAgbmV3IFNlbGVjdE5hdkNvbnRyb2xsZXIoc2VsZWN0LmZpcnN0RWxlbWVudENoaWxkKTtcbiAgfVxufVxuXG4vKipcbiAqIEV2ZW50IGhhbmRsZXJzIGZvciBleHBhbmRpbmcgYW5kIGNvbGxhcHNpbmcgdGhlIHJlYWRtZSBzZWN0aW9uLlxuICovXG5jb25zdCByZWFkbWUgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuanMtcmVhZG1lJyk7XG5jb25zdCByZWFkbWVDb250ZW50ID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLXJlYWRtZUNvbnRlbnQnKTtcbmNvbnN0IHJlYWRtZU91dGxpbmUgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuanMtcmVhZG1lT3V0bGluZScpO1xuY29uc3QgcmVhZG1lRXhwYW5kID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbCgnLmpzLXJlYWRtZUV4cGFuZCcpO1xuY29uc3QgcmVhZG1lQ29sbGFwc2UgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuanMtcmVhZG1lQ29sbGFwc2UnKTtcbmNvbnN0IG1vYmlsZU5hdlNlbGVjdCA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTFNlbGVjdEVsZW1lbnQ+KCcuRG9jTmF2TW9iaWxlLXNlbGVjdCcpO1xuaWYgKHJlYWRtZSAmJiByZWFkbWVDb250ZW50ICYmIHJlYWRtZU91dGxpbmUgJiYgcmVhZG1lRXhwYW5kLmxlbmd0aCAmJiByZWFkbWVDb2xsYXBzZSkge1xuICBpZiAod2luZG93LmxvY2F0aW9uLmhhc2guaW5jbHVkZXMoJ3JlYWRtZScpKSB7XG4gICAgcmVhZG1lLmNsYXNzTGlzdC5hZGQoJ1VuaXRSZWFkbWUtLWV4cGFuZGVkJyk7XG4gIH1cbiAgbW9iaWxlTmF2U2VsZWN0Py5hZGRFdmVudExpc3RlbmVyKCdjaGFuZ2UnLCBlID0+IHtcbiAgICBpZiAoKGUudGFyZ2V0IGFzIEhUTUxTZWxlY3RFbGVtZW50KS52YWx1ZS5zdGFydHNXaXRoKCdyZWFkbWUtJykpIHtcbiAgICAgIHJlYWRtZS5jbGFzc0xpc3QuYWRkKCdVbml0UmVhZG1lLS1leHBhbmRlZCcpO1xuICAgIH1cbiAgfSk7XG4gIHJlYWRtZUV4cGFuZC5mb3JFYWNoKGVsID0+XG4gICAgZWwuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgIGUucHJldmVudERlZmF1bHQoKTtcbiAgICAgIHJlYWRtZS5jbGFzc0xpc3QuYWRkKCdVbml0UmVhZG1lLS1leHBhbmRlZCcpO1xuICAgICAgcmVhZG1lLnNjcm9sbEludG9WaWV3KCk7XG4gICAgfSlcbiAgKTtcbiAgcmVhZG1lQ29sbGFwc2UuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICBlLnByZXZlbnREZWZhdWx0KCk7XG4gICAgcmVhZG1lLmNsYXNzTGlzdC5yZW1vdmUoJ1VuaXRSZWFkbWUtLWV4cGFuZGVkJyk7XG4gICAgaWYgKHJlYWRtZUV4cGFuZFsxXSkge1xuICAgICAgcmVhZG1lRXhwYW5kWzFdLnNjcm9sbEludG9WaWV3KHsgYmxvY2s6ICdjZW50ZXInIH0pO1xuICAgIH1cbiAgfSk7XG4gIHJlYWRtZUNvbnRlbnQuYWRkRXZlbnRMaXN0ZW5lcigna2V5dXAnLCAoKSA9PiB7XG4gICAgcmVhZG1lLmNsYXNzTGlzdC5hZGQoJ1VuaXRSZWFkbWUtLWV4cGFuZGVkJyk7XG4gIH0pO1xuICByZWFkbWVDb250ZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4ge1xuICAgIHJlYWRtZS5jbGFzc0xpc3QuYWRkKCdVbml0UmVhZG1lLS1leHBhbmRlZCcpO1xuICB9KTtcbiAgcmVhZG1lT3V0bGluZS5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICByZWFkbWUuY2xhc3NMaXN0LmFkZCgnVW5pdFJlYWRtZS0tZXhwYW5kZWQnKTtcbiAgfSk7XG4gIGRvY3VtZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2tleWRvd24nLCBlID0+IHtcbiAgICBpZiAoKGUuY3RybEtleSB8fCBlLm1ldGFLZXkpICYmIGUua2V5ID09PSAnZicpIHtcbiAgICAgIHJlYWRtZS5jbGFzc0xpc3QuYWRkKCdVbml0UmVhZG1lLS1leHBhbmRlZCcpO1xuICAgIH1cbiAgfSk7XG59XG5cbi8qKlxuICogRXhwYW5kIGRldGFpbHMgaXRlbXMgdGhhdCBhcmUgZm9jdXNlZC4gVGhpcyB3aWxsIGV4cGFuZFxuICogZGVwcmVjYXRlZCBzeW1ib2xzIHdoZW4gdGhleSBhcmUgbmF2aWdhdGVkIHRvIGZyb20gdGhlIGluZGV4XG4gKiBvciBhIGRpcmVjdCBsaW5rLlxuICovXG5mdW5jdGlvbiBvcGVuRGVwcmVjYXRlZFN5bWJvbCgpIHtcbiAgaWYgKCFsb2NhdGlvbi5oYXNoKSByZXR1cm47XG4gIGNvbnN0IGhlYWRpbmcgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKGxvY2F0aW9uLmhhc2gpO1xuICBjb25zdCBncmFuZFBhcmVudCA9IGhlYWRpbmc/LnBhcmVudEVsZW1lbnQ/LnBhcmVudEVsZW1lbnQgYXMgSFRNTERldGFpbHNFbGVtZW50IHwgbnVsbDtcbiAgaWYgKGdyYW5kUGFyZW50Py5ub2RlTmFtZSA9PT0gJ0RFVEFJTFMnKSB7XG4gICAgZ3JhbmRQYXJlbnQub3BlbiA9IHRydWU7XG4gIH1cbn1cbm9wZW5EZXByZWNhdGVkU3ltYm9sKCk7XG53aW5kb3cuYWRkRXZlbnRMaXN0ZW5lcignaGFzaGNoYW5nZScsICgpID0+IG9wZW5EZXByZWNhdGVkU3ltYm9sKCkpO1xuXG4vKipcbiAqIExpc3RlbiBmb3IgY2hhbmdlcyBpbiB0aGUgYnVpbGQgY29udGV4dCBkcm9wZG93bi5cbiAqL1xuZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbCgnLmpzLWJ1aWxkQ29udGV4dFNlbGVjdCcpLmZvckVhY2goZWwgPT4ge1xuICBlbC5hZGRFdmVudExpc3RlbmVyKCdjaGFuZ2UnLCBlID0+IHtcbiAgICB3aW5kb3cubG9jYXRpb24uc2VhcmNoID0gYD9HT09TPSR7KGUudGFyZ2V0IGFzIEhUTUxTZWxlY3RFbGVtZW50KS52YWx1ZX1gO1xuICB9KTtcbn0pO1xuIl0sCiAgIm1hcHBpbmdzIjogIjs7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUF3QkEsTUFBTSxhQUFhLFNBQVMsY0FBaUM7QUFDN0QsTUFBTSxXQUFXLFlBQVksY0FBOEI7QUFDM0QsTUFBTSxXQUFXLFlBQVksY0FBOEI7QUFDM0QsTUFBTSxhQUFhLFlBQVksY0FBZ0M7QUFDL0QsTUFBTSxNQUFNLFNBQVMsY0FBOEI7QUFTbkQsTUFBSTtBQVVKLGtDQUFnQztBQUM5QixVQUFNLFFBQVE7QUFDZCxRQUFJLENBQUM7QUFBSztBQUNWLGVBQVcsTUFBTSxJQUFJLGlCQUFpQixnQkFBZ0I7QUFDcEQsWUFBTSxLQUFLLGdCQUFnQjtBQUFBO0FBSTdCLGVBQVcs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
