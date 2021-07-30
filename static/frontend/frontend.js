(() => {
  var __defProp = Object.defineProperty;
  var __markAsModule = (target) => __defProp(target, "__esModule", {value: true});
  var __commonJS = (callback, module) => () => {
    if (!module) {
      module = {exports: {}};
      callback(module.exports, module);
    }
    return module.exports;
  };
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, {get: all[name], enumerable: true});
  };

  // third_party/dialog-polyfill/dialog-polyfill.esm.js
  var require_dialog_polyfill_esm = __commonJS((exports) => {
    __markAsModule(exports);
    __export(exports, {
      default: () => dialog_polyfill_esm_default
    });
    var supportCustomEvent = window.CustomEvent;
    if (!supportCustomEvent || typeof supportCustomEvent === "object") {
      supportCustomEvent = function CustomEvent(event, x) {
        x = x || {};
        var ev = document.createEvent("CustomEvent");
        ev.initCustomEvent(event, !!x.bubbles, !!x.cancelable, x.detail || null);
        return ev;
      };
      supportCustomEvent.prototype = window.Event.prototype;
    }
    function createsStackingContext(el) {
      while (el && el !== document.body) {
        var s = window.getComputedStyle(el);
        var invalid = function(k, ok) {
          return !(s[k] === void 0 || s[k] === ok);
        };
        if (s.opacity < 1 || invalid("zIndex", "auto") || invalid("transform", "none") || invalid("mixBlendMode", "normal") || invalid("filter", "none") || invalid("perspective", "none") || s["isolation"] === "isolate" || s.position === "fixed" || s.webkitOverflowScrolling === "touch") {
          return true;
        }
        el = el.parentElement;
      }
      return false;
    }
    function findNearestDialog(el) {
      while (el) {
        if (el.localName === "dialog") {
          return el;
        }
        el = el.parentElement;
      }
      return null;
    }
    function safeBlur(el) {
      if (el && el.blur && el !== document.body) {
        el.blur();
      }
    }
    function inNodeList(nodeList, node) {
      for (var i = 0; i < nodeList.length; ++i) {
        if (nodeList[i] === node) {
          return true;
        }
      }
      return false;
    }
    function isFormMethodDialog(el) {
      if (!el || !el.hasAttribute("method")) {
        return false;
      }
      return el.getAttribute("method").toLowerCase() === "dialog";
    }
    function dialogPolyfillInfo(dialog) {
      this.dialog_ = dialog;
      this.replacedStyleTop_ = false;
      this.openAsModal_ = false;
      if (!dialog.hasAttribute("role")) {
        dialog.setAttribute("role", "dialog");
      }
      dialog.show = this.show.bind(this);
      dialog.showModal = this.showModal.bind(this);
      dialog.close = this.close.bind(this);
      if (!("returnValue" in dialog)) {
        dialog.returnValue = "";
      }
      if ("MutationObserver" in window) {
        var mo = new MutationObserver(this.maybeHideModal.bind(this));
        mo.observe(dialog, {attributes: true, attributeFilter: ["open"]});
      } else {
        var removed = false;
        var cb = function() {
          removed ? this.downgradeModal() : this.maybeHideModal();
          removed = false;
        }.bind(this);
        var timeout;
        var delayModel = function(ev) {
          if (ev.target !== dialog) {
            return;
          }
          var cand = "DOMNodeRemoved";
          removed |= ev.type.substr(0, cand.length) === cand;
          window.clearTimeout(timeout);
          timeout = window.setTimeout(cb, 0);
        };
        ["DOMAttrModified", "DOMNodeRemoved", "DOMNodeRemovedFromDocument"].forEach(function(name) {
          dialog.addEventListener(name, delayModel);
        });
      }
      Object.defineProperty(dialog, "open", {
        set: this.setOpen.bind(this),
        get: dialog.hasAttribute.bind(dialog, "open")
      });
      this.backdrop_ = document.createElement("div");
      this.backdrop_.className = "backdrop";
      this.backdrop_.addEventListener("click", this.backdropClick_.bind(this));
    }
    dialogPolyfillInfo.prototype = {
      get dialog() {
        return this.dialog_;
      },
      maybeHideModal: function() {
        if (this.dialog_.hasAttribute("open") && document.body.contains(this.dialog_)) {
          return;
        }
        this.downgradeModal();
      },
      downgradeModal: function() {
        if (!this.openAsModal_) {
          return;
        }
        this.openAsModal_ = false;
        this.dialog_.style.zIndex = "";
        if (this.replacedStyleTop_) {
          this.dialog_.style.top = "";
          this.replacedStyleTop_ = false;
        }
        this.backdrop_.parentNode && this.backdrop_.parentNode.removeChild(this.backdrop_);
        dialogPolyfill.dm.removeDialog(this);
      },
      setOpen: function(value) {
        if (value) {
          this.dialog_.hasAttribute("open") || this.dialog_.setAttribute("open", "");
        } else {
          this.dialog_.removeAttribute("open");
          this.maybeHideModal();
        }
      },
      backdropClick_: function(e) {
        if (!this.dialog_.hasAttribute("tabindex")) {
          var fake = document.createElement("div");
          this.dialog_.insertBefore(fake, this.dialog_.firstChild);
          fake.tabIndex = -1;
          fake.focus();
          this.dialog_.removeChild(fake);
        } else {
          this.dialog_.focus();
        }
        var redirectedEvent = document.createEvent("MouseEvents");
        redirectedEvent.initMouseEvent(e.type, e.bubbles, e.cancelable, window, e.detail, e.screenX, e.screenY, e.clientX, e.clientY, e.ctrlKey, e.altKey, e.shiftKey, e.metaKey, e.button, e.relatedTarget);
        this.dialog_.dispatchEvent(redirectedEvent);
        e.stopPropagation();
      },
      focus_: function() {
        var target = this.dialog_.querySelector("[autofocus]:not([disabled])");
        if (!target && this.dialog_.tabIndex >= 0) {
          target = this.dialog_;
        }
        if (!target) {
          var opts = ["button", "input", "keygen", "select", "textarea"];
          var query = opts.map(function(el) {
            return el + ":not([disabled])";
          });
          query.push('[tabindex]:not([disabled]):not([tabindex=""])');
          target = this.dialog_.querySelector(query.join(", "));
        }
        safeBlur(document.activeElement);
        target && target.focus();
      },
      updateZIndex: function(dialogZ, backdropZ) {
        if (dialogZ < backdropZ) {
          throw new Error("dialogZ should never be < backdropZ");
        }
        this.dialog_.style.zIndex = dialogZ;
        this.backdrop_.style.zIndex = backdropZ;
      },
      show: function() {
        if (!this.dialog_.open) {
          this.setOpen(true);
          this.focus_();
        }
      },
      showModal: function() {
        if (this.dialog_.hasAttribute("open")) {
          throw new Error("Failed to execute 'showModal' on dialog: The element is already open, and therefore cannot be opened modally.");
        }
        if (!document.body.contains(this.dialog_)) {
          throw new Error("Failed to execute 'showModal' on dialog: The element is not in a Document.");
        }
        if (!dialogPolyfill.dm.pushDialog(this)) {
          throw new Error("Failed to execute 'showModal' on dialog: There are too many open modal dialogs.");
        }
        if (createsStackingContext(this.dialog_.parentElement)) {
          console.warn("A dialog is being shown inside a stacking context. This may cause it to be unusable. For more information, see this link: https://github.com/GoogleChrome/dialog-polyfill/#stacking-context");
        }
        this.setOpen(true);
        this.openAsModal_ = true;
        if (dialogPolyfill.needsCentering(this.dialog_)) {
          dialogPolyfill.reposition(this.dialog_);
          this.replacedStyleTop_ = true;
        } else {
          this.replacedStyleTop_ = false;
        }
        this.dialog_.parentNode.insertBefore(this.backdrop_, this.dialog_.nextSibling);
        this.focus_();
      },
      close: function(opt_returnValue) {
        if (!this.dialog_.hasAttribute("open")) {
          throw new Error("Failed to execute 'close' on dialog: The element does not have an 'open' attribute, and therefore cannot be closed.");
        }
        this.setOpen(false);
        if (opt_returnValue !== void 0) {
          this.dialog_.returnValue = opt_returnValue;
        }
        var closeEvent = new supportCustomEvent("close", {
          bubbles: false,
          cancelable: false
        });
        this.dialog_.dispatchEvent(closeEvent);
      }
    };
    var dialogPolyfill = {};
    dialogPolyfill.reposition = function(element) {
      var scrollTop = document.body.scrollTop || document.documentElement.scrollTop;
      var topValue = scrollTop + (window.innerHeight - element.offsetHeight) / 2;
      element.style.top = Math.max(scrollTop, topValue) + "px";
    };
    dialogPolyfill.isInlinePositionSetByStylesheet = function(element) {
      for (var i = 0; i < document.styleSheets.length; ++i) {
        var styleSheet = document.styleSheets[i];
        var cssRules = null;
        try {
          cssRules = styleSheet.cssRules;
        } catch (e) {
        }
        if (!cssRules) {
          continue;
        }
        for (var j = 0; j < cssRules.length; ++j) {
          var rule = cssRules[j];
          var selectedNodes = null;
          try {
            selectedNodes = document.querySelectorAll(rule.selectorText);
          } catch (e) {
          }
          if (!selectedNodes || !inNodeList(selectedNodes, element)) {
            continue;
          }
          var cssTop = rule.style.getPropertyValue("top");
          var cssBottom = rule.style.getPropertyValue("bottom");
          if (cssTop && cssTop !== "auto" || cssBottom && cssBottom !== "auto") {
            return true;
          }
        }
      }
      return false;
    };
    dialogPolyfill.needsCentering = function(dialog) {
      var computedStyle = window.getComputedStyle(dialog);
      if (computedStyle.position !== "absolute") {
        return false;
      }
      if (dialog.style.top !== "auto" && dialog.style.top !== "" || dialog.style.bottom !== "auto" && dialog.style.bottom !== "") {
        return false;
      }
      return !dialogPolyfill.isInlinePositionSetByStylesheet(dialog);
    };
    dialogPolyfill.forceRegisterDialog = function(element) {
      if (window.HTMLDialogElement || element.showModal) {
        console.warn("This browser already supports <dialog>, the polyfill may not work correctly", element);
      }
      if (element.localName !== "dialog") {
        throw new Error("Failed to register dialog: The element is not a dialog.");
      }
      new dialogPolyfillInfo(element);
    };
    dialogPolyfill.registerDialog = function(element) {
      if (!element.showModal) {
        dialogPolyfill.forceRegisterDialog(element);
      }
    };
    dialogPolyfill.DialogManager = function() {
      this.pendingDialogStack = [];
      var checkDOM = this.checkDOM_.bind(this);
      this.overlay = document.createElement("div");
      this.overlay.className = "_dialog_overlay";
      this.overlay.addEventListener("click", function(e) {
        this.forwardTab_ = void 0;
        e.stopPropagation();
        checkDOM([]);
      }.bind(this));
      this.handleKey_ = this.handleKey_.bind(this);
      this.handleFocus_ = this.handleFocus_.bind(this);
      this.zIndexLow_ = 1e5;
      this.zIndexHigh_ = 1e5 + 150;
      this.forwardTab_ = void 0;
      if ("MutationObserver" in window) {
        this.mo_ = new MutationObserver(function(records) {
          var removed = [];
          records.forEach(function(rec) {
            for (var i = 0, c; c = rec.removedNodes[i]; ++i) {
              if (!(c instanceof Element)) {
                continue;
              } else if (c.localName === "dialog") {
                removed.push(c);
              }
              removed = removed.concat(c.querySelectorAll("dialog"));
            }
          });
          removed.length && checkDOM(removed);
        });
      }
    };
    dialogPolyfill.DialogManager.prototype.blockDocument = function() {
      document.documentElement.addEventListener("focus", this.handleFocus_, true);
      document.addEventListener("keydown", this.handleKey_);
      this.mo_ && this.mo_.observe(document, {childList: true, subtree: true});
    };
    dialogPolyfill.DialogManager.prototype.unblockDocument = function() {
      document.documentElement.removeEventListener("focus", this.handleFocus_, true);
      document.removeEventListener("keydown", this.handleKey_);
      this.mo_ && this.mo_.disconnect();
    };
    dialogPolyfill.DialogManager.prototype.updateStacking = function() {
      var zIndex = this.zIndexHigh_;
      for (var i = 0, dpi; dpi = this.pendingDialogStack[i]; ++i) {
        dpi.updateZIndex(--zIndex, --zIndex);
        if (i === 0) {
          this.overlay.style.zIndex = --zIndex;
        }
      }
      var last = this.pendingDialogStack[0];
      if (last) {
        var p = last.dialog.parentNode || document.body;
        p.appendChild(this.overlay);
      } else if (this.overlay.parentNode) {
        this.overlay.parentNode.removeChild(this.overlay);
      }
    };
    dialogPolyfill.DialogManager.prototype.containedByTopDialog_ = function(candidate) {
      while (candidate = findNearestDialog(candidate)) {
        for (var i = 0, dpi; dpi = this.pendingDialogStack[i]; ++i) {
          if (dpi.dialog === candidate) {
            return i === 0;
          }
        }
        candidate = candidate.parentElement;
      }
      return false;
    };
    dialogPolyfill.DialogManager.prototype.handleFocus_ = function(event) {
      if (this.containedByTopDialog_(event.target)) {
        return;
      }
      if (document.activeElement === document.documentElement) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      safeBlur(event.target);
      if (this.forwardTab_ === void 0) {
        return;
      }
      var dpi = this.pendingDialogStack[0];
      var dialog = dpi.dialog;
      var position = dialog.compareDocumentPosition(event.target);
      if (position & Node.DOCUMENT_POSITION_PRECEDING) {
        if (this.forwardTab_) {
          dpi.focus_();
        } else if (event.target !== document.documentElement) {
          document.documentElement.focus();
        }
      }
      return false;
    };
    dialogPolyfill.DialogManager.prototype.handleKey_ = function(event) {
      this.forwardTab_ = void 0;
      if (event.keyCode === 27) {
        event.preventDefault();
        event.stopPropagation();
        var cancelEvent = new supportCustomEvent("cancel", {
          bubbles: false,
          cancelable: true
        });
        var dpi = this.pendingDialogStack[0];
        if (dpi && dpi.dialog.dispatchEvent(cancelEvent)) {
          dpi.dialog.close();
        }
      } else if (event.keyCode === 9) {
        this.forwardTab_ = !event.shiftKey;
      }
    };
    dialogPolyfill.DialogManager.prototype.checkDOM_ = function(removed) {
      var clone = this.pendingDialogStack.slice();
      clone.forEach(function(dpi) {
        if (removed.indexOf(dpi.dialog) !== -1) {
          dpi.downgradeModal();
        } else {
          dpi.maybeHideModal();
        }
      });
    };
    dialogPolyfill.DialogManager.prototype.pushDialog = function(dpi) {
      var allowed = (this.zIndexHigh_ - this.zIndexLow_) / 2 - 1;
      if (this.pendingDialogStack.length >= allowed) {
        return false;
      }
      if (this.pendingDialogStack.unshift(dpi) === 1) {
        this.blockDocument();
      }
      this.updateStacking();
      return true;
    };
    dialogPolyfill.DialogManager.prototype.removeDialog = function(dpi) {
      var index = this.pendingDialogStack.indexOf(dpi);
      if (index === -1) {
        return;
      }
      this.pendingDialogStack.splice(index, 1);
      if (this.pendingDialogStack.length === 0) {
        this.unblockDocument();
      }
      this.updateStacking();
    };
    dialogPolyfill.dm = new dialogPolyfill.DialogManager();
    dialogPolyfill.formSubmitter = null;
    dialogPolyfill.useValue = null;
    if (window.HTMLDialogElement === void 0) {
      testForm = document.createElement("form");
      testForm.setAttribute("method", "dialog");
      if (testForm.method !== "dialog") {
        methodDescriptor = Object.getOwnPropertyDescriptor(HTMLFormElement.prototype, "method");
        if (methodDescriptor) {
          realGet = methodDescriptor.get;
          methodDescriptor.get = function() {
            if (isFormMethodDialog(this)) {
              return "dialog";
            }
            return realGet.call(this);
          };
          realSet = methodDescriptor.set;
          methodDescriptor.set = function(v) {
            if (typeof v === "string" && v.toLowerCase() === "dialog") {
              return this.setAttribute("method", v);
            }
            return realSet.call(this, v);
          };
          Object.defineProperty(HTMLFormElement.prototype, "method", methodDescriptor);
        }
      }
      document.addEventListener("click", function(ev) {
        dialogPolyfill.formSubmitter = null;
        dialogPolyfill.useValue = null;
        if (ev.defaultPrevented) {
          return;
        }
        var target = ev.target;
        if (!target || !isFormMethodDialog(target.form)) {
          return;
        }
        var valid = target.type === "submit" && ["button", "input"].indexOf(target.localName) > -1;
        if (!valid) {
          if (!(target.localName === "input" && target.type === "image")) {
            return;
          }
          dialogPolyfill.useValue = ev.offsetX + "," + ev.offsetY;
        }
        var dialog = findNearestDialog(target);
        if (!dialog) {
          return;
        }
        dialogPolyfill.formSubmitter = target;
      }, false);
      nativeFormSubmit = HTMLFormElement.prototype.submit;
      replacementFormSubmit = function() {
        if (!isFormMethodDialog(this)) {
          return nativeFormSubmit.call(this);
        }
        var dialog = findNearestDialog(this);
        dialog && dialog.close();
      };
      HTMLFormElement.prototype.submit = replacementFormSubmit;
      document.addEventListener("submit", function(ev) {
        if (ev.defaultPrevented) {
          return;
        }
        var form = ev.target;
        if (!isFormMethodDialog(form)) {
          return;
        }
        ev.preventDefault();
        var dialog = findNearestDialog(form);
        if (!dialog) {
          return;
        }
        var s = dialogPolyfill.formSubmitter;
        if (s && s.form === form) {
          dialog.close(dialogPolyfill.useValue || s.value);
        } else {
          dialog.close();
        }
        dialogPolyfill.formSubmitter = null;
      }, false);
    }
    var testForm;
    var methodDescriptor;
    var realGet;
    var realSet;
    var nativeFormSubmit;
    var replacementFormSubmit;
    var dialog_polyfill_esm_default = dialogPolyfill;
  });

  // static/shared/header/header.ts
  function registerHeaderListeners() {
    const header = document.querySelector(".js-header");
    const menuButtons = document.querySelectorAll(".js-headerMenuButton");
    menuButtons.forEach((button) => {
      button.addEventListener("click", (e) => {
        e.preventDefault();
        header?.classList.toggle("is-active");
        button.setAttribute("aria-expanded", String(header?.classList.contains("is-active")));
      });
    });
    const scrim = document.querySelector(".js-scrim");
    scrim?.addEventListener("click", (e) => {
      e.preventDefault();
      header?.classList.remove("is-active");
      menuButtons.forEach((button) => {
        button.setAttribute("aria-expanded", String(header?.classList.contains("is-active")));
      });
    });
  }
  function registerSearchFormListeners() {
    const searchForm = document.querySelector(".js-searchForm");
    const expandSearch = document.querySelector(".js-expandSearch");
    const input = searchForm?.querySelector("input");
    const headerLogo = document.querySelector(".js-headerLogo");
    const menuButton = document.querySelector(".js-headerMenuButton");
    expandSearch?.addEventListener("click", () => {
      searchForm?.classList.add("go-SearchForm--expanded");
      headerLogo?.classList.add("go-Header-logo--hidden");
      menuButton?.classList.add("go-Header-navOpen--hidden");
      input?.focus();
    });
    document?.addEventListener("click", (e) => {
      if (!searchForm?.contains(e.target)) {
        searchForm?.classList.remove("go-SearchForm--expanded");
        headerLogo?.classList.remove("go-Header-logo--hidden");
        menuButton?.classList.remove("go-Header-navOpen--hidden");
      }
    });
  }
  document.querySelectorAll(".js-searchModeSelect").forEach((el) => {
    el.addEventListener("change", (e) => {
      const urlSearchParams = new URLSearchParams(window.location.search);
      const params = Object.fromEntries(urlSearchParams.entries());
      const query = params["q"];
      if (query) {
        window.location.search = `q=${query}&m=${e.target.value}`;
      }
    });
  });
  registerHeaderListeners();
  registerSearchFormListeners();

  // static/shared/clipboard/clipboard.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var ClipboardController = class {
    constructor(el) {
      this.el = el;
      this.data = el.dataset["toCopy"] ?? el.innerText;
      if (!this.data && el.parentElement?.classList.contains("go-InputGroup")) {
        this.data = (this.data || el.parentElement?.querySelector("input")?.value) ?? "";
      }
      el.addEventListener("click", (e) => this.handleCopyClick(e));
    }
    handleCopyClick(e) {
      e.preventDefault();
      const TOOLTIP_SHOW_DURATION_MS = 1e3;
      if (!navigator.clipboard) {
        this.showTooltipText("Unable to copy", TOOLTIP_SHOW_DURATION_MS);
        return;
      }
      navigator.clipboard.writeText(this.data).then(() => {
        this.showTooltipText("Copied!", TOOLTIP_SHOW_DURATION_MS);
      }).catch(() => {
        this.showTooltipText("Unable to copy", TOOLTIP_SHOW_DURATION_MS);
      });
    }
    showTooltipText(text, durationMs) {
      this.el.setAttribute("data-tooltip", text);
      setTimeout(() => this.el.setAttribute("data-tooltip", ""), durationMs);
    }
  };

  // static/shared/tooltip/tooltip.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var ToolTipController = class {
    constructor(el) {
      this.el = el;
      document.addEventListener("click", (e) => {
        const insideTooltip = this.el.contains(e.target);
        if (!insideTooltip) {
          this.el.removeAttribute("open");
        }
      });
    }
  };

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

  // static/shared/modal/modal.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var ModalController = class {
    constructor(el) {
      this.el = el;
      if (!window.HTMLDialogElement && !el.showModal) {
        Promise.resolve().then(() => require_dialog_polyfill_esm()).then(({default: polyfill}) => {
          polyfill.registerDialog(el);
        });
      }
      const id = el.id;
      const button = document.querySelector(`[aria-controls="${id}"]`);
      if (button) {
        button.addEventListener("click", () => {
          if (this.el.showModal) {
            this.el.showModal();
          } else {
            this.el.open = true;
          }
          el.querySelector("input")?.focus();
        });
      }
      for (const close of this.el.querySelectorAll("[data-modal-close]")) {
        close.addEventListener("click", () => {
          if (this.el.close) {
            this.el.close();
          } else {
            this.el.open = false;
          }
        });
      }
    }
  };

  // static/shared/analytics/analytics.ts
  var analytics_exports = {};
  __export(analytics_exports, {
    func: () => func,
    track: () => track
  });
  function track(event, category, action, label) {
    window.dataLayer ??= [];
    if (typeof event === "string") {
      window.dataLayer.push({
        event,
        event_category: category,
        event_action: action,
        event_label: label
      });
    } else {
      window.dataLayer.push(event);
    }
  }
  function func(fn) {
    window.dataLayer ??= [];
    window.dataLayer.push(fn);
  }

  // static/shared/keyboard/keyboard.ts
  /*!
   * @license
   * Copyright 2019-2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var KeyboardController = class {
    constructor() {
      this.handlers = {};
      document.addEventListener("keydown", (e) => this.handleKeyPress(e));
    }
    on(key, description, callback, options) {
      this.handlers[key] ??= new Set();
      this.handlers[key].add({description, callback, ...options});
      return this;
    }
    handleKeyPress(e) {
      for (const handler of this.handlers[e.key.toLowerCase()] ?? new Set()) {
        if (handler.target && handler.target !== e.target) {
          return;
        }
        const t = e.target;
        if (!handler.target && (t?.tagName === "INPUT" || t?.tagName === "SELECT" || t?.tagName === "TEXTAREA")) {
          return;
        }
        if (t?.isContentEditable) {
          return;
        }
        if (handler.withMeta && !(e.ctrlKey || e.metaKey) || !handler.withMeta && (e.ctrlKey || e.metaKey)) {
          return;
        }
        track("keypress", "hotkeys", `${e.key} pressed`, handler.description);
        handler.callback(e);
      }
    }
  };
  var keyboard = new KeyboardController();

  // static/shared/shared.ts
  /**
   * @license
   * Copyright 2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  for (const el of document.querySelectorAll(".js-clipboard")) {
    new ClipboardController(el);
  }
  for (const el of document.querySelectorAll(".js-modal")) {
    new ModalController(el);
  }
  for (const t of document.querySelectorAll(".js-tooltip")) {
    new ToolTipController(t);
  }
  for (const el of document.querySelectorAll(".js-selectNav")) {
    new SelectNavController(el);
  }

  // static/frontend/frontend.ts
  /**
   * @license
   * Copyright 2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  keyboard.on("t", "toggle theme", () => {
    let nextTheme = "dark";
    const theme = document.documentElement.getAttribute("data-theme");
    if (theme === "dark") {
      nextTheme = "light";
    } else if (theme === "light") {
      nextTheme = "auto";
    }
    document.documentElement.setAttribute("data-theme", nextTheme);
    document.cookie = `prefers-color-scheme=${nextTheme};path=/;max-age=31536000;`;
  });
  keyboard.on("/", "focus search", (e) => {
    const searchInput = Array.from(document.querySelectorAll(".js-searchFocus")).pop();
    if (searchInput && !window.navigator.userAgent.includes("Firefox")) {
      e.preventDefault();
      searchInput.focus();
    }
  });
  keyboard.on("y", "set canonical url", () => {
    const canonicalURLPath = document.querySelector(".js-canonicalURLPath")?.dataset["canonicalUrlPath"];
    if (canonicalURLPath && canonicalURLPath !== "") {
      window.history.replaceState(null, "", canonicalURLPath);
    }
  });
  (function setupGoogleTagManager() {
    analytics_exports.track({
      "gtm.start": new Date().getTime(),
      event: "gtm.js"
    });
  })();
  function removeUTMSource() {
    const urlParams = new URLSearchParams(window.location.search);
    const utmSource = urlParams.get("utm_source");
    if (utmSource !== "gopls" && utmSource !== "godoc" && utmSource !== "pkggodev") {
      return;
    }
    const newURL = new URL(window.location.href);
    urlParams.delete("utm_source");
    newURL.search = urlParams.toString();
    window.history.replaceState(null, "", newURL.toString());
  }
  if (document.querySelector(".js-gtmID")?.dataset.gtmid && window.dataLayer) {
    analytics_exports.func(function() {
      removeUTMSource();
    });
  } else {
    removeUTMSource();
  }
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiLi4vLi4vdGhpcmRfcGFydHkvZGlhbG9nLXBvbHlmaWxsL2RpYWxvZy1wb2x5ZmlsbC5lc20uanMiLCAiLi4vc2hhcmVkL2hlYWRlci9oZWFkZXIudHMiLCAiLi4vc2hhcmVkL2NsaXBib2FyZC9jbGlwYm9hcmQudHMiLCAiLi4vc2hhcmVkL3Rvb2x0aXAvdG9vbHRpcC50cyIsICIuLi9zaGFyZWQvb3V0bGluZS9zZWxlY3QudHMiLCAiLi4vc2hhcmVkL21vZGFsL21vZGFsLnRzIiwgIi4uL3NoYXJlZC9hbmFseXRpY3MvYW5hbHl0aWNzLnRzIiwgIi4uL3NoYXJlZC9rZXlib2FyZC9rZXlib2FyZC50cyIsICIuLi9zaGFyZWQvc2hhcmVkLnRzIiwgImZyb250ZW5kLnRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyIvLyBuYi4gVGhpcyBpcyBmb3IgSUUxMCBhbmQgbG93ZXIgX29ubHlfLlxudmFyIHN1cHBvcnRDdXN0b21FdmVudCA9IHdpbmRvdy5DdXN0b21FdmVudDtcbmlmICghc3VwcG9ydEN1c3RvbUV2ZW50IHx8IHR5cGVvZiBzdXBwb3J0Q3VzdG9tRXZlbnQgPT09ICdvYmplY3QnKSB7XG4gIHN1cHBvcnRDdXN0b21FdmVudCA9IGZ1bmN0aW9uIEN1c3RvbUV2ZW50KGV2ZW50LCB4KSB7XG4gICAgeCA9IHggfHwge307XG4gICAgdmFyIGV2ID0gZG9jdW1lbnQuY3JlYXRlRXZlbnQoJ0N1c3RvbUV2ZW50Jyk7XG4gICAgZXYuaW5pdEN1c3RvbUV2ZW50KGV2ZW50LCAhIXguYnViYmxlcywgISF4LmNhbmNlbGFibGUsIHguZGV0YWlsIHx8IG51bGwpO1xuICAgIHJldHVybiBldjtcbiAgfTtcbiAgc3VwcG9ydEN1c3RvbUV2ZW50LnByb3RvdHlwZSA9IHdpbmRvdy5FdmVudC5wcm90b3R5cGU7XG59XG5cbi8qKlxuICogQHBhcmFtIHtFbGVtZW50fSBlbCB0byBjaGVjayBmb3Igc3RhY2tpbmcgY29udGV4dFxuICogQHJldHVybiB7Ym9vbGVhbn0gd2hldGhlciB0aGlzIGVsIG9yIGl0cyBwYXJlbnRzIGNyZWF0ZXMgYSBzdGFja2luZyBjb250ZXh0XG4gKi9cbmZ1bmN0aW9uIGNyZWF0ZXNTdGFja2luZ0NvbnRleHQoZWwpIHtcbiAgd2hpbGUgKGVsICYmIGVsICE9PSBkb2N1bWVudC5ib2R5KSB7XG4gICAgdmFyIHMgPSB3aW5kb3cuZ2V0Q29tcHV0ZWRTdHlsZShlbCk7XG4gICAgdmFyIGludmFsaWQgPSBmdW5jdGlvbihrLCBvaykge1xuICAgICAgcmV0dXJuICEoc1trXSA9PT0gdW5kZWZpbmVkIHx8IHNba10gPT09IG9rKTtcbiAgICB9O1xuXG4gICAgaWYgKHMub3BhY2l0eSA8IDEgfHxcbiAgICAgICAgaW52YWxpZCgnekluZGV4JywgJ2F1dG8nKSB8fFxuICAgICAgICBpbnZhbGlkKCd0cmFuc2Zvcm0nLCAnbm9uZScpIHx8XG4gICAgICAgIGludmFsaWQoJ21peEJsZW5kTW9kZScsICdub3JtYWwnKSB8fFxuICAgICAgICBpbnZhbGlkKCdmaWx0ZXInLCAnbm9uZScpIHx8XG4gICAgICAgIGludmFsaWQoJ3BlcnNwZWN0aXZlJywgJ25vbmUnKSB8fFxuICAgICAgICBzWydpc29sYXRpb24nXSA9PT0gJ2lzb2xhdGUnIHx8XG4gICAgICAgIHMucG9zaXRpb24gPT09ICdmaXhlZCcgfHxcbiAgICAgICAgcy53ZWJraXRPdmVyZmxvd1Njcm9sbGluZyA9PT0gJ3RvdWNoJykge1xuICAgICAgcmV0dXJuIHRydWU7XG4gICAgfVxuICAgIGVsID0gZWwucGFyZW50RWxlbWVudDtcbiAgfVxuICByZXR1cm4gZmFsc2U7XG59XG5cbi8qKlxuICogRmluZHMgdGhlIG5lYXJlc3QgPGRpYWxvZz4gZnJvbSB0aGUgcGFzc2VkIGVsZW1lbnQuXG4gKlxuICogQHBhcmFtIHtFbGVtZW50fSBlbCB0byBzZWFyY2ggZnJvbVxuICogQHJldHVybiB7SFRNTERpYWxvZ0VsZW1lbnR9IGRpYWxvZyBmb3VuZFxuICovXG5mdW5jdGlvbiBmaW5kTmVhcmVzdERpYWxvZyhlbCkge1xuICB3aGlsZSAoZWwpIHtcbiAgICBpZiAoZWwubG9jYWxOYW1lID09PSAnZGlhbG9nJykge1xuICAgICAgcmV0dXJuIC8qKiBAdHlwZSB7SFRNTERpYWxvZ0VsZW1lbnR9ICovIChlbCk7XG4gICAgfVxuICAgIGVsID0gZWwucGFyZW50RWxlbWVudDtcbiAgfVxuICByZXR1cm4gbnVsbDtcbn1cblxuLyoqXG4gKiBCbHVyIHRoZSBzcGVjaWZpZWQgZWxlbWVudCwgYXMgbG9uZyBhcyBpdCdzIG5vdCB0aGUgSFRNTCBib2R5IGVsZW1lbnQuXG4gKiBUaGlzIHdvcmtzIGFyb3VuZCBhbiBJRTkvMTAgYnVnIC0gYmx1cnJpbmcgdGhlIGJvZHkgY2F1c2VzIFdpbmRvd3MgdG9cbiAqIGJsdXIgdGhlIHdob2xlIGFwcGxpY2F0aW9uLlxuICpcbiAqIEBwYXJhbSB7RWxlbWVudH0gZWwgdG8gYmx1clxuICovXG5mdW5jdGlvbiBzYWZlQmx1cihlbCkge1xuICBpZiAoZWwgJiYgZWwuYmx1ciAmJiBlbCAhPT0gZG9jdW1lbnQuYm9keSkge1xuICAgIGVsLmJsdXIoKTtcbiAgfVxufVxuXG4vKipcbiAqIEBwYXJhbSB7IU5vZGVMaXN0fSBub2RlTGlzdCB0byBzZWFyY2hcbiAqIEBwYXJhbSB7Tm9kZX0gbm9kZSB0byBmaW5kXG4gKiBAcmV0dXJuIHtib29sZWFufSB3aGV0aGVyIG5vZGUgaXMgaW5zaWRlIG5vZGVMaXN0XG4gKi9cbmZ1bmN0aW9uIGluTm9kZUxpc3Qobm9kZUxpc3QsIG5vZGUpIHtcbiAgZm9yICh2YXIgaSA9IDA7IGkgPCBub2RlTGlzdC5sZW5ndGg7ICsraSkge1xuICAgIGlmIChub2RlTGlzdFtpXSA9PT0gbm9kZSkge1xuICAgICAgcmV0dXJuIHRydWU7XG4gICAgfVxuICB9XG4gIHJldHVybiBmYWxzZTtcbn1cblxuLyoqXG4gKiBAcGFyYW0ge0hUTUxGb3JtRWxlbWVudH0gZWwgdG8gY2hlY2tcbiAqIEByZXR1cm4ge2Jvb2xlYW59IHdoZXRoZXIgdGhpcyBmb3JtIGhhcyBtZXRob2Q9XCJkaWFsb2dcIlxuICovXG5mdW5jdGlvbiBpc0Zvcm1NZXRob2REaWFsb2coZWwpIHtcbiAgaWYgKCFlbCB8fCAhZWwuaGFzQXR0cmlidXRlKCdtZXRob2QnKSkge1xuICAgIHJldHVybiBmYWxzZTtcbiAgfVxuICByZXR1cm4gZWwuZ2V0QXR0cmlidXRlKCdtZXRob2QnKS50b0xvd2VyQ2FzZSgpID09PSAnZGlhbG9nJztcbn1cblxuLyoqXG4gKiBAcGFyYW0geyFIVE1MRGlhbG9nRWxlbWVudH0gZGlhbG9nIHRvIHVwZ3JhZGVcbiAqIEBjb25zdHJ1Y3RvclxuICovXG5mdW5jdGlvbiBkaWFsb2dQb2x5ZmlsbEluZm8oZGlhbG9nKSB7XG4gIHRoaXMuZGlhbG9nXyA9IGRpYWxvZztcbiAgdGhpcy5yZXBsYWNlZFN0eWxlVG9wXyA9IGZhbHNlO1xuICB0aGlzLm9wZW5Bc01vZGFsXyA9IGZhbHNlO1xuXG4gIC8vIFNldCBhMTF5IHJvbGUuIEJyb3dzZXJzIHRoYXQgc3VwcG9ydCBkaWFsb2cgaW1wbGljaXRseSBrbm93IHRoaXMgYWxyZWFkeS5cbiAgaWYgKCFkaWFsb2cuaGFzQXR0cmlidXRlKCdyb2xlJykpIHtcbiAgICBkaWFsb2cuc2V0QXR0cmlidXRlKCdyb2xlJywgJ2RpYWxvZycpO1xuICB9XG5cbiAgZGlhbG9nLnNob3cgPSB0aGlzLnNob3cuYmluZCh0aGlzKTtcbiAgZGlhbG9nLnNob3dNb2RhbCA9IHRoaXMuc2hvd01vZGFsLmJpbmQodGhpcyk7XG4gIGRpYWxvZy5jbG9zZSA9IHRoaXMuY2xvc2UuYmluZCh0aGlzKTtcblxuICBpZiAoISgncmV0dXJuVmFsdWUnIGluIGRpYWxvZykpIHtcbiAgICBkaWFsb2cucmV0dXJuVmFsdWUgPSAnJztcbiAgfVxuXG4gIGlmICgnTXV0YXRpb25PYnNlcnZlcicgaW4gd2luZG93KSB7XG4gICAgdmFyIG1vID0gbmV3IE11dGF0aW9uT2JzZXJ2ZXIodGhpcy5tYXliZUhpZGVNb2RhbC5iaW5kKHRoaXMpKTtcbiAgICBtby5vYnNlcnZlKGRpYWxvZywge2F0dHJpYnV0ZXM6IHRydWUsIGF0dHJpYnV0ZUZpbHRlcjogWydvcGVuJ119KTtcbiAgfSBlbHNlIHtcbiAgICAvLyBJRTEwIGFuZCBiZWxvdyBzdXBwb3J0LiBOb3RlIHRoYXQgRE9NTm9kZVJlbW92ZWQgZXRjIGZpcmUgX2JlZm9yZV8gcmVtb3ZhbC4gVGhleSBhbHNvXG4gICAgLy8gc2VlbSB0byBmaXJlIGV2ZW4gaWYgdGhlIGVsZW1lbnQgd2FzIHJlbW92ZWQgYXMgcGFydCBvZiBhIHBhcmVudCByZW1vdmFsLiBVc2UgdGhlIHJlbW92ZWRcbiAgICAvLyBldmVudHMgdG8gZm9yY2UgZG93bmdyYWRlICh1c2VmdWwgaWYgcmVtb3ZlZC9pbW1lZGlhdGVseSBhZGRlZCkuXG4gICAgdmFyIHJlbW92ZWQgPSBmYWxzZTtcbiAgICB2YXIgY2IgPSBmdW5jdGlvbigpIHtcbiAgICAgIHJlbW92ZWQgPyB0aGlzLmRvd25ncmFkZU1vZGFsKCkgOiB0aGlzLm1heWJlSGlkZU1vZGFsKCk7XG4gICAgICByZW1vdmVkID0gZmFsc2U7XG4gICAgfS5iaW5kKHRoaXMpO1xuICAgIHZhciB0aW1lb3V0O1xuICAgIHZhciBkZWxheU1vZGVsID0gZnVuY3Rpb24oZXYpIHtcbiAgICAgIGlmIChldi50YXJnZXQgIT09IGRpYWxvZykgeyByZXR1cm47IH0gIC8vIG5vdCBmb3IgYSBjaGlsZCBlbGVtZW50XG4gICAgICB2YXIgY2FuZCA9ICdET01Ob2RlUmVtb3ZlZCc7XG4gICAgICByZW1vdmVkIHw9IChldi50eXBlLnN1YnN0cigwLCBjYW5kLmxlbmd0aCkgPT09IGNhbmQpO1xuICAgICAgd2luZG93LmNsZWFyVGltZW91dCh0aW1lb3V0KTtcbiAgICAgIHRpbWVvdXQgPSB3aW5kb3cuc2V0VGltZW91dChjYiwgMCk7XG4gICAgfTtcbiAgICBbJ0RPTUF0dHJNb2RpZmllZCcsICdET01Ob2RlUmVtb3ZlZCcsICdET01Ob2RlUmVtb3ZlZEZyb21Eb2N1bWVudCddLmZvckVhY2goZnVuY3Rpb24obmFtZSkge1xuICAgICAgZGlhbG9nLmFkZEV2ZW50TGlzdGVuZXIobmFtZSwgZGVsYXlNb2RlbCk7XG4gICAgfSk7XG4gIH1cbiAgLy8gTm90ZSB0aGF0IHRoZSBET00gaXMgb2JzZXJ2ZWQgaW5zaWRlIERpYWxvZ01hbmFnZXIgd2hpbGUgYW55IGRpYWxvZ1xuICAvLyBpcyBiZWluZyBkaXNwbGF5ZWQgYXMgYSBtb2RhbCwgdG8gY2F0Y2ggbW9kYWwgcmVtb3ZhbCBmcm9tIHRoZSBET00uXG5cbiAgT2JqZWN0LmRlZmluZVByb3BlcnR5KGRpYWxvZywgJ29wZW4nLCB7XG4gICAgc2V0OiB0aGlzLnNldE9wZW4uYmluZCh0aGlzKSxcbiAgICBnZXQ6IGRpYWxvZy5oYXNBdHRyaWJ1dGUuYmluZChkaWFsb2csICdvcGVuJylcbiAgfSk7XG5cbiAgdGhpcy5iYWNrZHJvcF8gPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdkaXYnKTtcbiAgdGhpcy5iYWNrZHJvcF8uY2xhc3NOYW1lID0gJ2JhY2tkcm9wJztcbiAgdGhpcy5iYWNrZHJvcF8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCB0aGlzLmJhY2tkcm9wQ2xpY2tfLmJpbmQodGhpcykpO1xufVxuXG5kaWFsb2dQb2x5ZmlsbEluZm8ucHJvdG90eXBlID0ge1xuXG4gIGdldCBkaWFsb2coKSB7XG4gICAgcmV0dXJuIHRoaXMuZGlhbG9nXztcbiAgfSxcblxuICAvKipcbiAgICogTWF5YmUgcmVtb3ZlIHRoaXMgZGlhbG9nIGZyb20gdGhlIG1vZGFsIHRvcCBsYXllci4gVGhpcyBpcyBjYWxsZWQgd2hlblxuICAgKiBhIG1vZGFsIGRpYWxvZyBtYXkgbm8gbG9uZ2VyIGJlIHRlbmFibGUsIGUuZy4sIHdoZW4gdGhlIGRpYWxvZyBpcyBub1xuICAgKiBsb25nZXIgb3BlbiBvciBpcyBubyBsb25nZXIgcGFydCBvZiB0aGUgRE9NLlxuICAgKi9cbiAgbWF5YmVIaWRlTW9kYWw6IGZ1bmN0aW9uKCkge1xuICAgIGlmICh0aGlzLmRpYWxvZ18uaGFzQXR0cmlidXRlKCdvcGVuJykgJiYgZG9jdW1lbnQuYm9keS5jb250YWlucyh0aGlzLmRpYWxvZ18pKSB7IHJldHVybjsgfVxuICAgIHRoaXMuZG93bmdyYWRlTW9kYWwoKTtcbiAgfSxcblxuICAvKipcbiAgICogUmVtb3ZlIHRoaXMgZGlhbG9nIGZyb20gdGhlIG1vZGFsIHRvcCBsYXllciwgbGVhdmluZyBpdCBhcyBhIG5vbi1tb2RhbC5cbiAgICovXG4gIGRvd25ncmFkZU1vZGFsOiBmdW5jdGlvbigpIHtcbiAgICBpZiAoIXRoaXMub3BlbkFzTW9kYWxfKSB7IHJldHVybjsgfVxuICAgIHRoaXMub3BlbkFzTW9kYWxfID0gZmFsc2U7XG4gICAgdGhpcy5kaWFsb2dfLnN0eWxlLnpJbmRleCA9ICcnO1xuXG4gICAgLy8gVGhpcyB3b24ndCBtYXRjaCB0aGUgbmF0aXZlIDxkaWFsb2c+IGV4YWN0bHkgYmVjYXVzZSBpZiB0aGUgdXNlciBzZXQgdG9wIG9uIGEgY2VudGVyZWRcbiAgICAvLyBwb2x5ZmlsbCBkaWFsb2csIHRoYXQgdG9wIGdldHMgdGhyb3duIGF3YXkgd2hlbiB0aGUgZGlhbG9nIGlzIGNsb3NlZC4gTm90IHN1cmUgaXQnc1xuICAgIC8vIHBvc3NpYmxlIHRvIHBvbHlmaWxsIHRoaXMgcGVyZmVjdGx5LlxuICAgIGlmICh0aGlzLnJlcGxhY2VkU3R5bGVUb3BfKSB7XG4gICAgICB0aGlzLmRpYWxvZ18uc3R5bGUudG9wID0gJyc7XG4gICAgICB0aGlzLnJlcGxhY2VkU3R5bGVUb3BfID0gZmFsc2U7XG4gICAgfVxuXG4gICAgLy8gQ2xlYXIgdGhlIGJhY2tkcm9wIGFuZCByZW1vdmUgZnJvbSB0aGUgbWFuYWdlci5cbiAgICB0aGlzLmJhY2tkcm9wXy5wYXJlbnROb2RlICYmIHRoaXMuYmFja2Ryb3BfLnBhcmVudE5vZGUucmVtb3ZlQ2hpbGQodGhpcy5iYWNrZHJvcF8pO1xuICAgIGRpYWxvZ1BvbHlmaWxsLmRtLnJlbW92ZURpYWxvZyh0aGlzKTtcbiAgfSxcblxuICAvKipcbiAgICogQHBhcmFtIHtib29sZWFufSB2YWx1ZSB3aGV0aGVyIHRvIG9wZW4gb3IgY2xvc2UgdGhpcyBkaWFsb2dcbiAgICovXG4gIHNldE9wZW46IGZ1bmN0aW9uKHZhbHVlKSB7XG4gICAgaWYgKHZhbHVlKSB7XG4gICAgICB0aGlzLmRpYWxvZ18uaGFzQXR0cmlidXRlKCdvcGVuJykgfHwgdGhpcy5kaWFsb2dfLnNldEF0dHJpYnV0ZSgnb3BlbicsICcnKTtcbiAgICB9IGVsc2Uge1xuICAgICAgdGhpcy5kaWFsb2dfLnJlbW92ZUF0dHJpYnV0ZSgnb3BlbicpO1xuICAgICAgdGhpcy5tYXliZUhpZGVNb2RhbCgpOyAgLy8gbmIuIHJlZHVuZGFudCB3aXRoIE11dGF0aW9uT2JzZXJ2ZXJcbiAgICB9XG4gIH0sXG5cbiAgLyoqXG4gICAqIEhhbmRsZXMgY2xpY2tzIG9uIHRoZSBmYWtlIC5iYWNrZHJvcCBlbGVtZW50LCByZWRpcmVjdGluZyB0aGVtIGFzIGlmXG4gICAqIHRoZXkgd2VyZSBvbiB0aGUgZGlhbG9nIGl0c2VsZi5cbiAgICpcbiAgICogQHBhcmFtIHshRXZlbnR9IGUgdG8gcmVkaXJlY3RcbiAgICovXG4gIGJhY2tkcm9wQ2xpY2tfOiBmdW5jdGlvbihlKSB7XG4gICAgaWYgKCF0aGlzLmRpYWxvZ18uaGFzQXR0cmlidXRlKCd0YWJpbmRleCcpKSB7XG4gICAgICAvLyBDbGlja2luZyBvbiB0aGUgYmFja2Ryb3Agc2hvdWxkIG1vdmUgdGhlIGltcGxpY2l0IGN1cnNvciwgZXZlbiBpZiBkaWFsb2cgY2Fubm90IGJlXG4gICAgICAvLyBmb2N1c2VkLiBDcmVhdGUgYSBmYWtlIHRoaW5nIHRvIGZvY3VzIG9uLiBJZiB0aGUgYmFja2Ryb3Agd2FzIF9iZWZvcmVfIHRoZSBkaWFsb2csIHRoaXNcbiAgICAgIC8vIHdvdWxkIG5vdCBiZSBuZWVkZWQgLSBjbGlja3Mgd291bGQgbW92ZSB0aGUgaW1wbGljaXQgY3Vyc29yIHRoZXJlLlxuICAgICAgdmFyIGZha2UgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdkaXYnKTtcbiAgICAgIHRoaXMuZGlhbG9nXy5pbnNlcnRCZWZvcmUoZmFrZSwgdGhpcy5kaWFsb2dfLmZpcnN0Q2hpbGQpO1xuICAgICAgZmFrZS50YWJJbmRleCA9IC0xO1xuICAgICAgZmFrZS5mb2N1cygpO1xuICAgICAgdGhpcy5kaWFsb2dfLnJlbW92ZUNoaWxkKGZha2UpO1xuICAgIH0gZWxzZSB7XG4gICAgICB0aGlzLmRpYWxvZ18uZm9jdXMoKTtcbiAgICB9XG5cbiAgICB2YXIgcmVkaXJlY3RlZEV2ZW50ID0gZG9jdW1lbnQuY3JlYXRlRXZlbnQoJ01vdXNlRXZlbnRzJyk7XG4gICAgcmVkaXJlY3RlZEV2ZW50LmluaXRNb3VzZUV2ZW50KGUudHlwZSwgZS5idWJibGVzLCBlLmNhbmNlbGFibGUsIHdpbmRvdyxcbiAgICAgICAgZS5kZXRhaWwsIGUuc2NyZWVuWCwgZS5zY3JlZW5ZLCBlLmNsaWVudFgsIGUuY2xpZW50WSwgZS5jdHJsS2V5LFxuICAgICAgICBlLmFsdEtleSwgZS5zaGlmdEtleSwgZS5tZXRhS2V5LCBlLmJ1dHRvbiwgZS5yZWxhdGVkVGFyZ2V0KTtcbiAgICB0aGlzLmRpYWxvZ18uZGlzcGF0Y2hFdmVudChyZWRpcmVjdGVkRXZlbnQpO1xuICAgIGUuc3RvcFByb3BhZ2F0aW9uKCk7XG4gIH0sXG5cbiAgLyoqXG4gICAqIEZvY3VzZXMgb24gdGhlIGZpcnN0IGZvY3VzYWJsZSBlbGVtZW50IHdpdGhpbiB0aGUgZGlhbG9nLiBUaGlzIHdpbGwgYWx3YXlzIGJsdXIgdGhlIGN1cnJlbnRcbiAgICogZm9jdXMsIGV2ZW4gaWYgbm90aGluZyB3aXRoaW4gdGhlIGRpYWxvZyBpcyBmb3VuZC5cbiAgICovXG4gIGZvY3VzXzogZnVuY3Rpb24oKSB7XG4gICAgLy8gRmluZCBlbGVtZW50IHdpdGggYGF1dG9mb2N1c2AgYXR0cmlidXRlLCBvciBmYWxsIGJhY2sgdG8gdGhlIGZpcnN0IGZvcm0vdGFiaW5kZXggY29udHJvbC5cbiAgICB2YXIgdGFyZ2V0ID0gdGhpcy5kaWFsb2dfLnF1ZXJ5U2VsZWN0b3IoJ1thdXRvZm9jdXNdOm5vdChbZGlzYWJsZWRdKScpO1xuICAgIGlmICghdGFyZ2V0ICYmIHRoaXMuZGlhbG9nXy50YWJJbmRleCA+PSAwKSB7XG4gICAgICB0YXJnZXQgPSB0aGlzLmRpYWxvZ187XG4gICAgfVxuICAgIGlmICghdGFyZ2V0KSB7XG4gICAgICAvLyBOb3RlIHRoYXQgdGhpcyBpcyAnYW55IGZvY3VzYWJsZSBhcmVhJy4gVGhpcyBsaXN0IGlzIHByb2JhYmx5IG5vdCBleGhhdXN0aXZlLCBidXQgdGhlXG4gICAgICAvLyBhbHRlcm5hdGl2ZSBpbnZvbHZlcyBzdGVwcGluZyB0aHJvdWdoIGFuZCB0cnlpbmcgdG8gZm9jdXMgZXZlcnl0aGluZy5cbiAgICAgIHZhciBvcHRzID0gWydidXR0b24nLCAnaW5wdXQnLCAna2V5Z2VuJywgJ3NlbGVjdCcsICd0ZXh0YXJlYSddO1xuICAgICAgdmFyIHF1ZXJ5ID0gb3B0cy5tYXAoZnVuY3Rpb24oZWwpIHtcbiAgICAgICAgcmV0dXJuIGVsICsgJzpub3QoW2Rpc2FibGVkXSknO1xuICAgICAgfSk7XG4gICAgICAvLyBUT0RPKHNhbXRob3IpOiB0YWJpbmRleCB2YWx1ZXMgdGhhdCBhcmUgbm90IG51bWVyaWMgYXJlIG5vdCBmb2N1c2FibGUuXG4gICAgICBxdWVyeS5wdXNoKCdbdGFiaW5kZXhdOm5vdChbZGlzYWJsZWRdKTpub3QoW3RhYmluZGV4PVwiXCJdKScpOyAgLy8gdGFiaW5kZXggIT0gXCJcIiwgbm90IGRpc2FibGVkXG4gICAgICB0YXJnZXQgPSB0aGlzLmRpYWxvZ18ucXVlcnlTZWxlY3RvcihxdWVyeS5qb2luKCcsICcpKTtcbiAgICB9XG4gICAgc2FmZUJsdXIoZG9jdW1lbnQuYWN0aXZlRWxlbWVudCk7XG4gICAgdGFyZ2V0ICYmIHRhcmdldC5mb2N1cygpO1xuICB9LFxuXG4gIC8qKlxuICAgKiBTZXRzIHRoZSB6SW5kZXggZm9yIHRoZSBiYWNrZHJvcCBhbmQgZGlhbG9nLlxuICAgKlxuICAgKiBAcGFyYW0ge251bWJlcn0gZGlhbG9nWlxuICAgKiBAcGFyYW0ge251bWJlcn0gYmFja2Ryb3BaXG4gICAqL1xuICB1cGRhdGVaSW5kZXg6IGZ1bmN0aW9uKGRpYWxvZ1osIGJhY2tkcm9wWikge1xuICAgIGlmIChkaWFsb2daIDwgYmFja2Ryb3BaKSB7XG4gICAgICB0aHJvdyBuZXcgRXJyb3IoJ2RpYWxvZ1ogc2hvdWxkIG5ldmVyIGJlIDwgYmFja2Ryb3BaJyk7XG4gICAgfVxuICAgIHRoaXMuZGlhbG9nXy5zdHlsZS56SW5kZXggPSBkaWFsb2daO1xuICAgIHRoaXMuYmFja2Ryb3BfLnN0eWxlLnpJbmRleCA9IGJhY2tkcm9wWjtcbiAgfSxcblxuICAvKipcbiAgICogU2hvd3MgdGhlIGRpYWxvZy4gSWYgdGhlIGRpYWxvZyBpcyBhbHJlYWR5IG9wZW4sIHRoaXMgZG9lcyBub3RoaW5nLlxuICAgKi9cbiAgc2hvdzogZnVuY3Rpb24oKSB7XG4gICAgaWYgKCF0aGlzLmRpYWxvZ18ub3Blbikge1xuICAgICAgdGhpcy5zZXRPcGVuKHRydWUpO1xuICAgICAgdGhpcy5mb2N1c18oKTtcbiAgICB9XG4gIH0sXG5cbiAgLyoqXG4gICAqIFNob3cgdGhpcyBkaWFsb2cgbW9kYWxseS5cbiAgICovXG4gIHNob3dNb2RhbDogZnVuY3Rpb24oKSB7XG4gICAgaWYgKHRoaXMuZGlhbG9nXy5oYXNBdHRyaWJ1dGUoJ29wZW4nKSkge1xuICAgICAgdGhyb3cgbmV3IEVycm9yKCdGYWlsZWQgdG8gZXhlY3V0ZSBcXCdzaG93TW9kYWxcXCcgb24gZGlhbG9nOiBUaGUgZWxlbWVudCBpcyBhbHJlYWR5IG9wZW4sIGFuZCB0aGVyZWZvcmUgY2Fubm90IGJlIG9wZW5lZCBtb2RhbGx5LicpO1xuICAgIH1cbiAgICBpZiAoIWRvY3VtZW50LmJvZHkuY29udGFpbnModGhpcy5kaWFsb2dfKSkge1xuICAgICAgdGhyb3cgbmV3IEVycm9yKCdGYWlsZWQgdG8gZXhlY3V0ZSBcXCdzaG93TW9kYWxcXCcgb24gZGlhbG9nOiBUaGUgZWxlbWVudCBpcyBub3QgaW4gYSBEb2N1bWVudC4nKTtcbiAgICB9XG4gICAgaWYgKCFkaWFsb2dQb2x5ZmlsbC5kbS5wdXNoRGlhbG9nKHRoaXMpKSB7XG4gICAgICB0aHJvdyBuZXcgRXJyb3IoJ0ZhaWxlZCB0byBleGVjdXRlIFxcJ3Nob3dNb2RhbFxcJyBvbiBkaWFsb2c6IFRoZXJlIGFyZSB0b28gbWFueSBvcGVuIG1vZGFsIGRpYWxvZ3MuJyk7XG4gICAgfVxuXG4gICAgaWYgKGNyZWF0ZXNTdGFja2luZ0NvbnRleHQodGhpcy5kaWFsb2dfLnBhcmVudEVsZW1lbnQpKSB7XG4gICAgICBjb25zb2xlLndhcm4oJ0EgZGlhbG9nIGlzIGJlaW5nIHNob3duIGluc2lkZSBhIHN0YWNraW5nIGNvbnRleHQuICcgK1xuICAgICAgICAgICdUaGlzIG1heSBjYXVzZSBpdCB0byBiZSB1bnVzYWJsZS4gRm9yIG1vcmUgaW5mb3JtYXRpb24sIHNlZSB0aGlzIGxpbms6ICcgK1xuICAgICAgICAgICdodHRwczovL2dpdGh1Yi5jb20vR29vZ2xlQ2hyb21lL2RpYWxvZy1wb2x5ZmlsbC8jc3RhY2tpbmctY29udGV4dCcpO1xuICAgIH1cblxuICAgIHRoaXMuc2V0T3Blbih0cnVlKTtcbiAgICB0aGlzLm9wZW5Bc01vZGFsXyA9IHRydWU7XG5cbiAgICAvLyBPcHRpb25hbGx5IGNlbnRlciB2ZXJ0aWNhbGx5LCByZWxhdGl2ZSB0byB0aGUgY3VycmVudCB2aWV3cG9ydC5cbiAgICBpZiAoZGlhbG9nUG9seWZpbGwubmVlZHNDZW50ZXJpbmcodGhpcy5kaWFsb2dfKSkge1xuICAgICAgZGlhbG9nUG9seWZpbGwucmVwb3NpdGlvbih0aGlzLmRpYWxvZ18pO1xuICAgICAgdGhpcy5yZXBsYWNlZFN0eWxlVG9wXyA9IHRydWU7XG4gICAgfSBlbHNlIHtcbiAgICAgIHRoaXMucmVwbGFjZWRTdHlsZVRvcF8gPSBmYWxzZTtcbiAgICB9XG5cbiAgICAvLyBJbnNlcnQgYmFja2Ryb3AuXG4gICAgdGhpcy5kaWFsb2dfLnBhcmVudE5vZGUuaW5zZXJ0QmVmb3JlKHRoaXMuYmFja2Ryb3BfLCB0aGlzLmRpYWxvZ18ubmV4dFNpYmxpbmcpO1xuXG4gICAgLy8gRm9jdXMgb24gd2hhdGV2ZXIgaW5zaWRlIHRoZSBkaWFsb2cuXG4gICAgdGhpcy5mb2N1c18oKTtcbiAgfSxcblxuICAvKipcbiAgICogQ2xvc2VzIHRoaXMgSFRNTERpYWxvZ0VsZW1lbnQuIFRoaXMgaXMgb3B0aW9uYWwgdnMgY2xlYXJpbmcgdGhlIG9wZW5cbiAgICogYXR0cmlidXRlLCBob3dldmVyIHRoaXMgZmlyZXMgYSAnY2xvc2UnIGV2ZW50LlxuICAgKlxuICAgKiBAcGFyYW0ge3N0cmluZz19IG9wdF9yZXR1cm5WYWx1ZSB0byB1c2UgYXMgdGhlIHJldHVyblZhbHVlXG4gICAqL1xuICBjbG9zZTogZnVuY3Rpb24ob3B0X3JldHVyblZhbHVlKSB7XG4gICAgaWYgKCF0aGlzLmRpYWxvZ18uaGFzQXR0cmlidXRlKCdvcGVuJykpIHtcbiAgICAgIHRocm93IG5ldyBFcnJvcignRmFpbGVkIHRvIGV4ZWN1dGUgXFwnY2xvc2VcXCcgb24gZGlhbG9nOiBUaGUgZWxlbWVudCBkb2VzIG5vdCBoYXZlIGFuIFxcJ29wZW5cXCcgYXR0cmlidXRlLCBhbmQgdGhlcmVmb3JlIGNhbm5vdCBiZSBjbG9zZWQuJyk7XG4gICAgfVxuICAgIHRoaXMuc2V0T3BlbihmYWxzZSk7XG5cbiAgICAvLyBMZWF2ZSByZXR1cm5WYWx1ZSB1bnRvdWNoZWQgaW4gY2FzZSBpdCB3YXMgc2V0IGRpcmVjdGx5IG9uIHRoZSBlbGVtZW50XG4gICAgaWYgKG9wdF9yZXR1cm5WYWx1ZSAhPT0gdW5kZWZpbmVkKSB7XG4gICAgICB0aGlzLmRpYWxvZ18ucmV0dXJuVmFsdWUgPSBvcHRfcmV0dXJuVmFsdWU7XG4gICAgfVxuXG4gICAgLy8gVHJpZ2dlcmluZyBcImNsb3NlXCIgZXZlbnQgZm9yIGFueSBhdHRhY2hlZCBsaXN0ZW5lcnMgb24gdGhlIDxkaWFsb2c+LlxuICAgIHZhciBjbG9zZUV2ZW50ID0gbmV3IHN1cHBvcnRDdXN0b21FdmVudCgnY2xvc2UnLCB7XG4gICAgICBidWJibGVzOiBmYWxzZSxcbiAgICAgIGNhbmNlbGFibGU6IGZhbHNlXG4gICAgfSk7XG4gICAgdGhpcy5kaWFsb2dfLmRpc3BhdGNoRXZlbnQoY2xvc2VFdmVudCk7XG4gIH1cblxufTtcblxudmFyIGRpYWxvZ1BvbHlmaWxsID0ge307XG5cbmRpYWxvZ1BvbHlmaWxsLnJlcG9zaXRpb24gPSBmdW5jdGlvbihlbGVtZW50KSB7XG4gIHZhciBzY3JvbGxUb3AgPSBkb2N1bWVudC5ib2R5LnNjcm9sbFRvcCB8fCBkb2N1bWVudC5kb2N1bWVudEVsZW1lbnQuc2Nyb2xsVG9wO1xuICB2YXIgdG9wVmFsdWUgPSBzY3JvbGxUb3AgKyAod2luZG93LmlubmVySGVpZ2h0IC0gZWxlbWVudC5vZmZzZXRIZWlnaHQpIC8gMjtcbiAgZWxlbWVudC5zdHlsZS50b3AgPSBNYXRoLm1heChzY3JvbGxUb3AsIHRvcFZhbHVlKSArICdweCc7XG59O1xuXG5kaWFsb2dQb2x5ZmlsbC5pc0lubGluZVBvc2l0aW9uU2V0QnlTdHlsZXNoZWV0ID0gZnVuY3Rpb24oZWxlbWVudCkge1xuICBmb3IgKHZhciBpID0gMDsgaSA8IGRvY3VtZW50LnN0eWxlU2hlZXRzLmxlbmd0aDsgKytpKSB7XG4gICAgdmFyIHN0eWxlU2hlZXQgPSBkb2N1bWVudC5zdHlsZVNoZWV0c1tpXTtcbiAgICB2YXIgY3NzUnVsZXMgPSBudWxsO1xuICAgIC8vIFNvbWUgYnJvd3NlcnMgdGhyb3cgb24gY3NzUnVsZXMuXG4gICAgdHJ5IHtcbiAgICAgIGNzc1J1bGVzID0gc3R5bGVTaGVldC5jc3NSdWxlcztcbiAgICB9IGNhdGNoIChlKSB7fVxuICAgIGlmICghY3NzUnVsZXMpIHsgY29udGludWU7IH1cbiAgICBmb3IgKHZhciBqID0gMDsgaiA8IGNzc1J1bGVzLmxlbmd0aDsgKytqKSB7XG4gICAgICB2YXIgcnVsZSA9IGNzc1J1bGVzW2pdO1xuICAgICAgdmFyIHNlbGVjdGVkTm9kZXMgPSBudWxsO1xuICAgICAgLy8gSWdub3JlIGVycm9ycyBvbiBpbnZhbGlkIHNlbGVjdG9yIHRleHRzLlxuICAgICAgdHJ5IHtcbiAgICAgICAgc2VsZWN0ZWROb2RlcyA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwocnVsZS5zZWxlY3RvclRleHQpO1xuICAgICAgfSBjYXRjaChlKSB7fVxuICAgICAgaWYgKCFzZWxlY3RlZE5vZGVzIHx8ICFpbk5vZGVMaXN0KHNlbGVjdGVkTm9kZXMsIGVsZW1lbnQpKSB7XG4gICAgICAgIGNvbnRpbnVlO1xuICAgICAgfVxuICAgICAgdmFyIGNzc1RvcCA9IHJ1bGUuc3R5bGUuZ2V0UHJvcGVydHlWYWx1ZSgndG9wJyk7XG4gICAgICB2YXIgY3NzQm90dG9tID0gcnVsZS5zdHlsZS5nZXRQcm9wZXJ0eVZhbHVlKCdib3R0b20nKTtcbiAgICAgIGlmICgoY3NzVG9wICYmIGNzc1RvcCAhPT0gJ2F1dG8nKSB8fCAoY3NzQm90dG9tICYmIGNzc0JvdHRvbSAhPT0gJ2F1dG8nKSkge1xuICAgICAgICByZXR1cm4gdHJ1ZTtcbiAgICAgIH1cbiAgICB9XG4gIH1cbiAgcmV0dXJuIGZhbHNlO1xufTtcblxuZGlhbG9nUG9seWZpbGwubmVlZHNDZW50ZXJpbmcgPSBmdW5jdGlvbihkaWFsb2cpIHtcbiAgdmFyIGNvbXB1dGVkU3R5bGUgPSB3aW5kb3cuZ2V0Q29tcHV0ZWRTdHlsZShkaWFsb2cpO1xuICBpZiAoY29tcHV0ZWRTdHlsZS5wb3NpdGlvbiAhPT0gJ2Fic29sdXRlJykge1xuICAgIHJldHVybiBmYWxzZTtcbiAgfVxuXG4gIC8vIFdlIG11c3QgZGV0ZXJtaW5lIHdoZXRoZXIgdGhlIHRvcC9ib3R0b20gc3BlY2lmaWVkIHZhbHVlIGlzIG5vbi1hdXRvLiAgSW5cbiAgLy8gV2ViS2l0L0JsaW5rLCBjaGVja2luZyBjb21wdXRlZFN0eWxlLnRvcCA9PSAnYXV0bycgaXMgc3VmZmljaWVudCwgYnV0XG4gIC8vIEZpcmVmb3ggcmV0dXJucyB0aGUgdXNlZCB2YWx1ZS4gU28gd2UgZG8gdGhpcyBjcmF6eSB0aGluZyBpbnN0ZWFkOiBjaGVja1xuICAvLyB0aGUgaW5saW5lIHN0eWxlIGFuZCB0aGVuIGdvIHRocm91Z2ggQ1NTIHJ1bGVzLlxuICBpZiAoKGRpYWxvZy5zdHlsZS50b3AgIT09ICdhdXRvJyAmJiBkaWFsb2cuc3R5bGUudG9wICE9PSAnJykgfHxcbiAgICAgIChkaWFsb2cuc3R5bGUuYm90dG9tICE9PSAnYXV0bycgJiYgZGlhbG9nLnN0eWxlLmJvdHRvbSAhPT0gJycpKSB7XG4gICAgcmV0dXJuIGZhbHNlO1xuICB9XG4gIHJldHVybiAhZGlhbG9nUG9seWZpbGwuaXNJbmxpbmVQb3NpdGlvblNldEJ5U3R5bGVzaGVldChkaWFsb2cpO1xufTtcblxuLyoqXG4gKiBAcGFyYW0geyFFbGVtZW50fSBlbGVtZW50IHRvIGZvcmNlIHVwZ3JhZGVcbiAqL1xuZGlhbG9nUG9seWZpbGwuZm9yY2VSZWdpc3RlckRpYWxvZyA9IGZ1bmN0aW9uKGVsZW1lbnQpIHtcbiAgaWYgKHdpbmRvdy5IVE1MRGlhbG9nRWxlbWVudCB8fCBlbGVtZW50LnNob3dNb2RhbCkge1xuICAgIGNvbnNvbGUud2FybignVGhpcyBicm93c2VyIGFscmVhZHkgc3VwcG9ydHMgPGRpYWxvZz4sIHRoZSBwb2x5ZmlsbCAnICtcbiAgICAgICAgJ21heSBub3Qgd29yayBjb3JyZWN0bHknLCBlbGVtZW50KTtcbiAgfVxuICBpZiAoZWxlbWVudC5sb2NhbE5hbWUgIT09ICdkaWFsb2cnKSB7XG4gICAgdGhyb3cgbmV3IEVycm9yKCdGYWlsZWQgdG8gcmVnaXN0ZXIgZGlhbG9nOiBUaGUgZWxlbWVudCBpcyBub3QgYSBkaWFsb2cuJyk7XG4gIH1cbiAgbmV3IGRpYWxvZ1BvbHlmaWxsSW5mbygvKiogQHR5cGUgeyFIVE1MRGlhbG9nRWxlbWVudH0gKi8gKGVsZW1lbnQpKTtcbn07XG5cbi8qKlxuICogQHBhcmFtIHshRWxlbWVudH0gZWxlbWVudCB0byB1cGdyYWRlLCBpZiBuZWNlc3NhcnlcbiAqL1xuZGlhbG9nUG9seWZpbGwucmVnaXN0ZXJEaWFsb2cgPSBmdW5jdGlvbihlbGVtZW50KSB7XG4gIGlmICghZWxlbWVudC5zaG93TW9kYWwpIHtcbiAgICBkaWFsb2dQb2x5ZmlsbC5mb3JjZVJlZ2lzdGVyRGlhbG9nKGVsZW1lbnQpO1xuICB9XG59O1xuXG4vKipcbiAqIEBjb25zdHJ1Y3RvclxuICovXG5kaWFsb2dQb2x5ZmlsbC5EaWFsb2dNYW5hZ2VyID0gZnVuY3Rpb24oKSB7XG4gIC8qKiBAdHlwZSB7IUFycmF5PCFkaWFsb2dQb2x5ZmlsbEluZm8+fSAqL1xuICB0aGlzLnBlbmRpbmdEaWFsb2dTdGFjayA9IFtdO1xuXG4gIHZhciBjaGVja0RPTSA9IHRoaXMuY2hlY2tET01fLmJpbmQodGhpcyk7XG5cbiAgLy8gVGhlIG92ZXJsYXkgaXMgdXNlZCB0byBzaW11bGF0ZSBob3cgYSBtb2RhbCBkaWFsb2cgYmxvY2tzIHRoZSBkb2N1bWVudC5cbiAgLy8gVGhlIGJsb2NraW5nIGRpYWxvZyBpcyBwb3NpdGlvbmVkIG9uIHRvcCBvZiB0aGUgb3ZlcmxheSwgYW5kIHRoZSByZXN0IG9mXG4gIC8vIHRoZSBkaWFsb2dzIG9uIHRoZSBwZW5kaW5nIGRpYWxvZyBzdGFjayBhcmUgcG9zaXRpb25lZCBiZWxvdyBpdC4gSW4gdGhlXG4gIC8vIGFjdHVhbCBpbXBsZW1lbnRhdGlvbiwgdGhlIG1vZGFsIGRpYWxvZyBzdGFja2luZyBpcyBjb250cm9sbGVkIGJ5IHRoZVxuICAvLyB0b3AgbGF5ZXIsIHdoZXJlIHotaW5kZXggaGFzIG5vIGVmZmVjdC5cbiAgdGhpcy5vdmVybGF5ID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnZGl2Jyk7XG4gIHRoaXMub3ZlcmxheS5jbGFzc05hbWUgPSAnX2RpYWxvZ19vdmVybGF5JztcbiAgdGhpcy5vdmVybGF5LmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgZnVuY3Rpb24oZSkge1xuICAgIHRoaXMuZm9yd2FyZFRhYl8gPSB1bmRlZmluZWQ7XG4gICAgZS5zdG9wUHJvcGFnYXRpb24oKTtcbiAgICBjaGVja0RPTShbXSk7ICAvLyBzYW5pdHktY2hlY2sgRE9NXG4gIH0uYmluZCh0aGlzKSk7XG5cbiAgdGhpcy5oYW5kbGVLZXlfID0gdGhpcy5oYW5kbGVLZXlfLmJpbmQodGhpcyk7XG4gIHRoaXMuaGFuZGxlRm9jdXNfID0gdGhpcy5oYW5kbGVGb2N1c18uYmluZCh0aGlzKTtcblxuICB0aGlzLnpJbmRleExvd18gPSAxMDAwMDA7XG4gIHRoaXMuekluZGV4SGlnaF8gPSAxMDAwMDAgKyAxNTA7XG5cbiAgdGhpcy5mb3J3YXJkVGFiXyA9IHVuZGVmaW5lZDtcblxuICBpZiAoJ011dGF0aW9uT2JzZXJ2ZXInIGluIHdpbmRvdykge1xuICAgIHRoaXMubW9fID0gbmV3IE11dGF0aW9uT2JzZXJ2ZXIoZnVuY3Rpb24ocmVjb3Jkcykge1xuICAgICAgdmFyIHJlbW92ZWQgPSBbXTtcbiAgICAgIHJlY29yZHMuZm9yRWFjaChmdW5jdGlvbihyZWMpIHtcbiAgICAgICAgZm9yICh2YXIgaSA9IDAsIGM7IGMgPSByZWMucmVtb3ZlZE5vZGVzW2ldOyArK2kpIHtcbiAgICAgICAgICBpZiAoIShjIGluc3RhbmNlb2YgRWxlbWVudCkpIHtcbiAgICAgICAgICAgIGNvbnRpbnVlO1xuICAgICAgICAgIH0gZWxzZSBpZiAoYy5sb2NhbE5hbWUgPT09ICdkaWFsb2cnKSB7XG4gICAgICAgICAgICByZW1vdmVkLnB1c2goYyk7XG4gICAgICAgICAgfVxuICAgICAgICAgIHJlbW92ZWQgPSByZW1vdmVkLmNvbmNhdChjLnF1ZXJ5U2VsZWN0b3JBbGwoJ2RpYWxvZycpKTtcbiAgICAgICAgfVxuICAgICAgfSk7XG4gICAgICByZW1vdmVkLmxlbmd0aCAmJiBjaGVja0RPTShyZW1vdmVkKTtcbiAgICB9KTtcbiAgfVxufTtcblxuLyoqXG4gKiBDYWxsZWQgb24gdGhlIGZpcnN0IG1vZGFsIGRpYWxvZyBiZWluZyBzaG93bi4gQWRkcyB0aGUgb3ZlcmxheSBhbmQgcmVsYXRlZFxuICogaGFuZGxlcnMuXG4gKi9cbmRpYWxvZ1BvbHlmaWxsLkRpYWxvZ01hbmFnZXIucHJvdG90eXBlLmJsb2NrRG9jdW1lbnQgPSBmdW5jdGlvbigpIHtcbiAgZG9jdW1lbnQuZG9jdW1lbnRFbGVtZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2ZvY3VzJywgdGhpcy5oYW5kbGVGb2N1c18sIHRydWUpO1xuICBkb2N1bWVudC5hZGRFdmVudExpc3RlbmVyKCdrZXlkb3duJywgdGhpcy5oYW5kbGVLZXlfKTtcbiAgdGhpcy5tb18gJiYgdGhpcy5tb18ub2JzZXJ2ZShkb2N1bWVudCwge2NoaWxkTGlzdDogdHJ1ZSwgc3VidHJlZTogdHJ1ZX0pO1xufTtcblxuLyoqXG4gKiBDYWxsZWQgb24gdGhlIGZpcnN0IG1vZGFsIGRpYWxvZyBiZWluZyByZW1vdmVkLCBpLmUuLCB3aGVuIG5vIG1vcmUgbW9kYWxcbiAqIGRpYWxvZ3MgYXJlIHZpc2libGUuXG4gKi9cbmRpYWxvZ1BvbHlmaWxsLkRpYWxvZ01hbmFnZXIucHJvdG90eXBlLnVuYmxvY2tEb2N1bWVudCA9IGZ1bmN0aW9uKCkge1xuICBkb2N1bWVudC5kb2N1bWVudEVsZW1lbnQucmVtb3ZlRXZlbnRMaXN0ZW5lcignZm9jdXMnLCB0aGlzLmhhbmRsZUZvY3VzXywgdHJ1ZSk7XG4gIGRvY3VtZW50LnJlbW92ZUV2ZW50TGlzdGVuZXIoJ2tleWRvd24nLCB0aGlzLmhhbmRsZUtleV8pO1xuICB0aGlzLm1vXyAmJiB0aGlzLm1vXy5kaXNjb25uZWN0KCk7XG59O1xuXG4vKipcbiAqIFVwZGF0ZXMgdGhlIHN0YWNraW5nIG9mIGFsbCBrbm93biBkaWFsb2dzLlxuICovXG5kaWFsb2dQb2x5ZmlsbC5EaWFsb2dNYW5hZ2VyLnByb3RvdHlwZS51cGRhdGVTdGFja2luZyA9IGZ1bmN0aW9uKCkge1xuICB2YXIgekluZGV4ID0gdGhpcy56SW5kZXhIaWdoXztcblxuICBmb3IgKHZhciBpID0gMCwgZHBpOyBkcGkgPSB0aGlzLnBlbmRpbmdEaWFsb2dTdGFja1tpXTsgKytpKSB7XG4gICAgZHBpLnVwZGF0ZVpJbmRleCgtLXpJbmRleCwgLS16SW5kZXgpO1xuICAgIGlmIChpID09PSAwKSB7XG4gICAgICB0aGlzLm92ZXJsYXkuc3R5bGUuekluZGV4ID0gLS16SW5kZXg7XG4gICAgfVxuICB9XG5cbiAgLy8gTWFrZSB0aGUgb3ZlcmxheSBhIHNpYmxpbmcgb2YgdGhlIGRpYWxvZyBpdHNlbGYuXG4gIHZhciBsYXN0ID0gdGhpcy5wZW5kaW5nRGlhbG9nU3RhY2tbMF07XG4gIGlmIChsYXN0KSB7XG4gICAgdmFyIHAgPSBsYXN0LmRpYWxvZy5wYXJlbnROb2RlIHx8IGRvY3VtZW50LmJvZHk7XG4gICAgcC5hcHBlbmRDaGlsZCh0aGlzLm92ZXJsYXkpO1xuICB9IGVsc2UgaWYgKHRoaXMub3ZlcmxheS5wYXJlbnROb2RlKSB7XG4gICAgdGhpcy5vdmVybGF5LnBhcmVudE5vZGUucmVtb3ZlQ2hpbGQodGhpcy5vdmVybGF5KTtcbiAgfVxufTtcblxuLyoqXG4gKiBAcGFyYW0ge0VsZW1lbnR9IGNhbmRpZGF0ZSB0byBjaGVjayBpZiBjb250YWluZWQgb3IgaXMgdGhlIHRvcC1tb3N0IG1vZGFsIGRpYWxvZ1xuICogQHJldHVybiB7Ym9vbGVhbn0gd2hldGhlciBjYW5kaWRhdGUgaXMgY29udGFpbmVkIGluIHRvcCBkaWFsb2dcbiAqL1xuZGlhbG9nUG9seWZpbGwuRGlhbG9nTWFuYWdlci5wcm90b3R5cGUuY29udGFpbmVkQnlUb3BEaWFsb2dfID0gZnVuY3Rpb24oY2FuZGlkYXRlKSB7XG4gIHdoaWxlIChjYW5kaWRhdGUgPSBmaW5kTmVhcmVzdERpYWxvZyhjYW5kaWRhdGUpKSB7XG4gICAgZm9yICh2YXIgaSA9IDAsIGRwaTsgZHBpID0gdGhpcy5wZW5kaW5nRGlhbG9nU3RhY2tbaV07ICsraSkge1xuICAgICAgaWYgKGRwaS5kaWFsb2cgPT09IGNhbmRpZGF0ZSkge1xuICAgICAgICByZXR1cm4gaSA9PT0gMDsgIC8vIG9ubHkgdmFsaWQgaWYgdG9wLW1vc3RcbiAgICAgIH1cbiAgICB9XG4gICAgY2FuZGlkYXRlID0gY2FuZGlkYXRlLnBhcmVudEVsZW1lbnQ7XG4gIH1cbiAgcmV0dXJuIGZhbHNlO1xufTtcblxuZGlhbG9nUG9seWZpbGwuRGlhbG9nTWFuYWdlci5wcm90b3R5cGUuaGFuZGxlRm9jdXNfID0gZnVuY3Rpb24oZXZlbnQpIHtcbiAgaWYgKHRoaXMuY29udGFpbmVkQnlUb3BEaWFsb2dfKGV2ZW50LnRhcmdldCkpIHsgcmV0dXJuOyB9XG5cbiAgaWYgKGRvY3VtZW50LmFjdGl2ZUVsZW1lbnQgPT09IGRvY3VtZW50LmRvY3VtZW50RWxlbWVudCkgeyByZXR1cm47IH1cblxuICBldmVudC5wcmV2ZW50RGVmYXVsdCgpO1xuICBldmVudC5zdG9wUHJvcGFnYXRpb24oKTtcbiAgc2FmZUJsdXIoLyoqIEB0eXBlIHtFbGVtZW50fSAqLyAoZXZlbnQudGFyZ2V0KSk7XG5cbiAgaWYgKHRoaXMuZm9yd2FyZFRhYl8gPT09IHVuZGVmaW5lZCkgeyByZXR1cm47IH0gIC8vIG1vdmUgZm9jdXMgb25seSBmcm9tIGEgdGFiIGtleVxuXG4gIHZhciBkcGkgPSB0aGlzLnBlbmRpbmdEaWFsb2dTdGFja1swXTtcbiAgdmFyIGRpYWxvZyA9IGRwaS5kaWFsb2c7XG4gIHZhciBwb3NpdGlvbiA9IGRpYWxvZy5jb21wYXJlRG9jdW1lbnRQb3NpdGlvbihldmVudC50YXJnZXQpO1xuICBpZiAocG9zaXRpb24gJiBOb2RlLkRPQ1VNRU5UX1BPU0lUSU9OX1BSRUNFRElORykge1xuICAgIGlmICh0aGlzLmZvcndhcmRUYWJfKSB7XG4gICAgICAvLyBmb3J3YXJkXG4gICAgICBkcGkuZm9jdXNfKCk7XG4gICAgfSBlbHNlIGlmIChldmVudC50YXJnZXQgIT09IGRvY3VtZW50LmRvY3VtZW50RWxlbWVudCkge1xuICAgICAgLy8gYmFja3dhcmRzIGlmIHdlJ3JlIG5vdCBhbHJlYWR5IGZvY3VzZWQgb24gPGh0bWw+XG4gICAgICBkb2N1bWVudC5kb2N1bWVudEVsZW1lbnQuZm9jdXMoKTtcbiAgICB9XG4gIH1cblxuICByZXR1cm4gZmFsc2U7XG59O1xuXG5kaWFsb2dQb2x5ZmlsbC5EaWFsb2dNYW5hZ2VyLnByb3RvdHlwZS5oYW5kbGVLZXlfID0gZnVuY3Rpb24oZXZlbnQpIHtcbiAgdGhpcy5mb3J3YXJkVGFiXyA9IHVuZGVmaW5lZDtcbiAgaWYgKGV2ZW50LmtleUNvZGUgPT09IDI3KSB7XG4gICAgZXZlbnQucHJldmVudERlZmF1bHQoKTtcbiAgICBldmVudC5zdG9wUHJvcGFnYXRpb24oKTtcbiAgICB2YXIgY2FuY2VsRXZlbnQgPSBuZXcgc3VwcG9ydEN1c3RvbUV2ZW50KCdjYW5jZWwnLCB7XG4gICAgICBidWJibGVzOiBmYWxzZSxcbiAgICAgIGNhbmNlbGFibGU6IHRydWVcbiAgICB9KTtcbiAgICB2YXIgZHBpID0gdGhpcy5wZW5kaW5nRGlhbG9nU3RhY2tbMF07XG4gICAgaWYgKGRwaSAmJiBkcGkuZGlhbG9nLmRpc3BhdGNoRXZlbnQoY2FuY2VsRXZlbnQpKSB7XG4gICAgICBkcGkuZGlhbG9nLmNsb3NlKCk7XG4gICAgfVxuICB9IGVsc2UgaWYgKGV2ZW50LmtleUNvZGUgPT09IDkpIHtcbiAgICB0aGlzLmZvcndhcmRUYWJfID0gIWV2ZW50LnNoaWZ0S2V5O1xuICB9XG59O1xuXG4vKipcbiAqIEZpbmRzIGFuZCBkb3duZ3JhZGVzIGFueSBrbm93biBtb2RhbCBkaWFsb2dzIHRoYXQgYXJlIG5vIGxvbmdlciBkaXNwbGF5ZWQuIERpYWxvZ3MgdGhhdCBhcmVcbiAqIHJlbW92ZWQgYW5kIGltbWVkaWF0ZWx5IHJlYWRkZWQgZG9uJ3Qgc3RheSBtb2RhbCwgdGhleSBiZWNvbWUgbm9ybWFsLlxuICpcbiAqIEBwYXJhbSB7IUFycmF5PCFIVE1MRGlhbG9nRWxlbWVudD59IHJlbW92ZWQgdGhhdCBoYXZlIGRlZmluaXRlbHkgYmVlbiByZW1vdmVkXG4gKi9cbmRpYWxvZ1BvbHlmaWxsLkRpYWxvZ01hbmFnZXIucHJvdG90eXBlLmNoZWNrRE9NXyA9IGZ1bmN0aW9uKHJlbW92ZWQpIHtcbiAgLy8gVGhpcyBvcGVyYXRlcyBvbiBhIGNsb25lIGJlY2F1c2UgaXQgbWF5IGNhdXNlIGl0IHRvIGNoYW5nZS4gRWFjaCBjaGFuZ2UgYWxzbyBjYWxsc1xuICAvLyB1cGRhdGVTdGFja2luZywgd2hpY2ggb25seSBhY3R1YWxseSBuZWVkcyB0byBoYXBwZW4gb25jZS4gQnV0IHdobyByZW1vdmVzIG1hbnkgbW9kYWwgZGlhbG9nc1xuICAvLyBhdCBhIHRpbWU/IVxuICB2YXIgY2xvbmUgPSB0aGlzLnBlbmRpbmdEaWFsb2dTdGFjay5zbGljZSgpO1xuICBjbG9uZS5mb3JFYWNoKGZ1bmN0aW9uKGRwaSkge1xuICAgIGlmIChyZW1vdmVkLmluZGV4T2YoZHBpLmRpYWxvZykgIT09IC0xKSB7XG4gICAgICBkcGkuZG93bmdyYWRlTW9kYWwoKTtcbiAgICB9IGVsc2Uge1xuICAgICAgZHBpLm1heWJlSGlkZU1vZGFsKCk7XG4gICAgfVxuICB9KTtcbn07XG5cbi8qKlxuICogQHBhcmFtIHshZGlhbG9nUG9seWZpbGxJbmZvfSBkcGlcbiAqIEByZXR1cm4ge2Jvb2xlYW59IHdoZXRoZXIgdGhlIGRpYWxvZyB3YXMgYWxsb3dlZFxuICovXG5kaWFsb2dQb2x5ZmlsbC5EaWFsb2dNYW5hZ2VyLnByb3RvdHlwZS5wdXNoRGlhbG9nID0gZnVuY3Rpb24oZHBpKSB7XG4gIHZhciBhbGxvd2VkID0gKHRoaXMuekluZGV4SGlnaF8gLSB0aGlzLnpJbmRleExvd18pIC8gMiAtIDE7XG4gIGlmICh0aGlzLnBlbmRpbmdEaWFsb2dTdGFjay5sZW5ndGggPj0gYWxsb3dlZCkge1xuICAgIHJldHVybiBmYWxzZTtcbiAgfVxuICBpZiAodGhpcy5wZW5kaW5nRGlhbG9nU3RhY2sudW5zaGlmdChkcGkpID09PSAxKSB7XG4gICAgdGhpcy5ibG9ja0RvY3VtZW50KCk7XG4gIH1cbiAgdGhpcy51cGRhdGVTdGFja2luZygpO1xuICByZXR1cm4gdHJ1ZTtcbn07XG5cbi8qKlxuICogQHBhcmFtIHshZGlhbG9nUG9seWZpbGxJbmZvfSBkcGlcbiAqL1xuZGlhbG9nUG9seWZpbGwuRGlhbG9nTWFuYWdlci5wcm90b3R5cGUucmVtb3ZlRGlhbG9nID0gZnVuY3Rpb24oZHBpKSB7XG4gIHZhciBpbmRleCA9IHRoaXMucGVuZGluZ0RpYWxvZ1N0YWNrLmluZGV4T2YoZHBpKTtcbiAgaWYgKGluZGV4ID09PSAtMSkgeyByZXR1cm47IH1cblxuICB0aGlzLnBlbmRpbmdEaWFsb2dTdGFjay5zcGxpY2UoaW5kZXgsIDEpO1xuICBpZiAodGhpcy5wZW5kaW5nRGlhbG9nU3RhY2subGVuZ3RoID09PSAwKSB7XG4gICAgdGhpcy51bmJsb2NrRG9jdW1lbnQoKTtcbiAgfVxuICB0aGlzLnVwZGF0ZVN0YWNraW5nKCk7XG59O1xuXG5kaWFsb2dQb2x5ZmlsbC5kbSA9IG5ldyBkaWFsb2dQb2x5ZmlsbC5EaWFsb2dNYW5hZ2VyKCk7XG5kaWFsb2dQb2x5ZmlsbC5mb3JtU3VibWl0dGVyID0gbnVsbDtcbmRpYWxvZ1BvbHlmaWxsLnVzZVZhbHVlID0gbnVsbDtcblxuLyoqXG4gKiBJbnN0YWxscyBnbG9iYWwgaGFuZGxlcnMsIHN1Y2ggYXMgY2xpY2sgbGlzdGVycyBhbmQgbmF0aXZlIG1ldGhvZCBvdmVycmlkZXMuIFRoZXNlIGFyZSBuZWVkZWRcbiAqIGV2ZW4gaWYgYSBubyBkaWFsb2cgaXMgcmVnaXN0ZXJlZCwgYXMgdGhleSBkZWFsIHdpdGggPGZvcm0gbWV0aG9kPVwiZGlhbG9nXCI+LlxuICovXG5pZiAod2luZG93LkhUTUxEaWFsb2dFbGVtZW50ID09PSB1bmRlZmluZWQpIHtcblxuICAvKipcbiAgICogSWYgSFRNTEZvcm1FbGVtZW50IHRyYW5zbGF0ZXMgbWV0aG9kPVwiRElBTE9HXCIgaW50byAnZ2V0JywgdGhlbiByZXBsYWNlIHRoZSBkZXNjcmlwdG9yIHdpdGhcbiAgICogb25lIHRoYXQgcmV0dXJucyB0aGUgY29ycmVjdCB2YWx1ZS5cbiAgICovXG4gIHZhciB0ZXN0Rm9ybSA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ2Zvcm0nKTtcbiAgdGVzdEZvcm0uc2V0QXR0cmlidXRlKCdtZXRob2QnLCAnZGlhbG9nJyk7XG4gIGlmICh0ZXN0Rm9ybS5tZXRob2QgIT09ICdkaWFsb2cnKSB7XG4gICAgdmFyIG1ldGhvZERlc2NyaXB0b3IgPSBPYmplY3QuZ2V0T3duUHJvcGVydHlEZXNjcmlwdG9yKEhUTUxGb3JtRWxlbWVudC5wcm90b3R5cGUsICdtZXRob2QnKTtcbiAgICBpZiAobWV0aG9kRGVzY3JpcHRvcikge1xuICAgICAgLy8gbmIuIFNvbWUgb2xkZXIgaU9TIGFuZCBvbGRlciBQaGFudG9tSlMgZmFpbCB0byByZXR1cm4gdGhlIGRlc2NyaXB0b3IuIERvbid0IGRvIGFueXRoaW5nXG4gICAgICAvLyBhbmQgZG9uJ3QgYm90aGVyIHRvIHVwZGF0ZSB0aGUgZWxlbWVudC5cbiAgICAgIHZhciByZWFsR2V0ID0gbWV0aG9kRGVzY3JpcHRvci5nZXQ7XG4gICAgICBtZXRob2REZXNjcmlwdG9yLmdldCA9IGZ1bmN0aW9uKCkge1xuICAgICAgICBpZiAoaXNGb3JtTWV0aG9kRGlhbG9nKHRoaXMpKSB7XG4gICAgICAgICAgcmV0dXJuICdkaWFsb2cnO1xuICAgICAgICB9XG4gICAgICAgIHJldHVybiByZWFsR2V0LmNhbGwodGhpcyk7XG4gICAgICB9O1xuICAgICAgdmFyIHJlYWxTZXQgPSBtZXRob2REZXNjcmlwdG9yLnNldDtcbiAgICAgIG1ldGhvZERlc2NyaXB0b3Iuc2V0ID0gZnVuY3Rpb24odikge1xuICAgICAgICBpZiAodHlwZW9mIHYgPT09ICdzdHJpbmcnICYmIHYudG9Mb3dlckNhc2UoKSA9PT0gJ2RpYWxvZycpIHtcbiAgICAgICAgICByZXR1cm4gdGhpcy5zZXRBdHRyaWJ1dGUoJ21ldGhvZCcsIHYpO1xuICAgICAgICB9XG4gICAgICAgIHJldHVybiByZWFsU2V0LmNhbGwodGhpcywgdik7XG4gICAgICB9O1xuICAgICAgT2JqZWN0LmRlZmluZVByb3BlcnR5KEhUTUxGb3JtRWxlbWVudC5wcm90b3R5cGUsICdtZXRob2QnLCBtZXRob2REZXNjcmlwdG9yKTtcbiAgICB9XG4gIH1cblxuICAvKipcbiAgICogR2xvYmFsICdjbGljaycgaGFuZGxlciwgdG8gY2FwdHVyZSB0aGUgPGlucHV0IHR5cGU9XCJzdWJtaXRcIj4gb3IgPGJ1dHRvbj4gZWxlbWVudCB3aGljaCBoYXNcbiAgICogc3VibWl0dGVkIGEgPGZvcm0gbWV0aG9kPVwiZGlhbG9nXCI+LiBOZWVkZWQgYXMgU2FmYXJpIGFuZCBvdGhlcnMgZG9uJ3QgcmVwb3J0IHRoaXMgaW5zaWRlXG4gICAqIGRvY3VtZW50LmFjdGl2ZUVsZW1lbnQuXG4gICAqL1xuICBkb2N1bWVudC5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsIGZ1bmN0aW9uKGV2KSB7XG4gICAgZGlhbG9nUG9seWZpbGwuZm9ybVN1Ym1pdHRlciA9IG51bGw7XG4gICAgZGlhbG9nUG9seWZpbGwudXNlVmFsdWUgPSBudWxsO1xuICAgIGlmIChldi5kZWZhdWx0UHJldmVudGVkKSB7IHJldHVybjsgfSAgLy8gZS5nLiBhIHN1Ym1pdCB3aGljaCBwcmV2ZW50cyBkZWZhdWx0IHN1Ym1pc3Npb25cblxuICAgIHZhciB0YXJnZXQgPSAvKiogQHR5cGUge0VsZW1lbnR9ICovIChldi50YXJnZXQpO1xuICAgIGlmICghdGFyZ2V0IHx8ICFpc0Zvcm1NZXRob2REaWFsb2codGFyZ2V0LmZvcm0pKSB7IHJldHVybjsgfVxuXG4gICAgdmFyIHZhbGlkID0gKHRhcmdldC50eXBlID09PSAnc3VibWl0JyAmJiBbJ2J1dHRvbicsICdpbnB1dCddLmluZGV4T2YodGFyZ2V0LmxvY2FsTmFtZSkgPiAtMSk7XG4gICAgaWYgKCF2YWxpZCkge1xuICAgICAgaWYgKCEodGFyZ2V0LmxvY2FsTmFtZSA9PT0gJ2lucHV0JyAmJiB0YXJnZXQudHlwZSA9PT0gJ2ltYWdlJykpIHsgcmV0dXJuOyB9XG4gICAgICAvLyB0aGlzIGlzIGEgPGlucHV0IHR5cGU9XCJpbWFnZVwiPiwgd2hpY2ggY2FuIHN1Ym1pdCBmb3Jtc1xuICAgICAgZGlhbG9nUG9seWZpbGwudXNlVmFsdWUgPSBldi5vZmZzZXRYICsgJywnICsgZXYub2Zmc2V0WTtcbiAgICB9XG5cbiAgICB2YXIgZGlhbG9nID0gZmluZE5lYXJlc3REaWFsb2codGFyZ2V0KTtcbiAgICBpZiAoIWRpYWxvZykgeyByZXR1cm47IH1cblxuICAgIGRpYWxvZ1BvbHlmaWxsLmZvcm1TdWJtaXR0ZXIgPSB0YXJnZXQ7XG5cbiAgfSwgZmFsc2UpO1xuXG4gIC8qKlxuICAgKiBSZXBsYWNlIHRoZSBuYXRpdmUgSFRNTEZvcm1FbGVtZW50LnN1Ym1pdCgpIG1ldGhvZCwgYXMgaXQgd29uJ3QgZmlyZSB0aGVcbiAgICogc3VibWl0IGV2ZW50IGFuZCBnaXZlIHVzIGEgY2hhbmNlIHRvIHJlc3BvbmQuXG4gICAqL1xuICB2YXIgbmF0aXZlRm9ybVN1Ym1pdCA9IEhUTUxGb3JtRWxlbWVudC5wcm90b3R5cGUuc3VibWl0O1xuICB2YXIgcmVwbGFjZW1lbnRGb3JtU3VibWl0ID0gZnVuY3Rpb24gKCkge1xuICAgIGlmICghaXNGb3JtTWV0aG9kRGlhbG9nKHRoaXMpKSB7XG4gICAgICByZXR1cm4gbmF0aXZlRm9ybVN1Ym1pdC5jYWxsKHRoaXMpO1xuICAgIH1cbiAgICB2YXIgZGlhbG9nID0gZmluZE5lYXJlc3REaWFsb2codGhpcyk7XG4gICAgZGlhbG9nICYmIGRpYWxvZy5jbG9zZSgpO1xuICB9O1xuICBIVE1MRm9ybUVsZW1lbnQucHJvdG90eXBlLnN1Ym1pdCA9IHJlcGxhY2VtZW50Rm9ybVN1Ym1pdDtcblxuICAvKipcbiAgICogR2xvYmFsIGZvcm0gJ2RpYWxvZycgbWV0aG9kIGhhbmRsZXIuIENsb3NlcyBhIGRpYWxvZyBjb3JyZWN0bHkgb24gc3VibWl0XG4gICAqIGFuZCBwb3NzaWJseSBzZXRzIGl0cyByZXR1cm4gdmFsdWUuXG4gICAqL1xuICBkb2N1bWVudC5hZGRFdmVudExpc3RlbmVyKCdzdWJtaXQnLCBmdW5jdGlvbihldikge1xuICAgIGlmIChldi5kZWZhdWx0UHJldmVudGVkKSB7IHJldHVybjsgfSAgLy8gZS5nLiBhIHN1Ym1pdCB3aGljaCBwcmV2ZW50cyBkZWZhdWx0IHN1Ym1pc3Npb25cblxuICAgIHZhciBmb3JtID0gLyoqIEB0eXBlIHtIVE1MRm9ybUVsZW1lbnR9ICovIChldi50YXJnZXQpO1xuICAgIGlmICghaXNGb3JtTWV0aG9kRGlhbG9nKGZvcm0pKSB7IHJldHVybjsgfVxuICAgIGV2LnByZXZlbnREZWZhdWx0KCk7XG5cbiAgICB2YXIgZGlhbG9nID0gZmluZE5lYXJlc3REaWFsb2coZm9ybSk7XG4gICAgaWYgKCFkaWFsb2cpIHsgcmV0dXJuOyB9XG5cbiAgICAvLyBGb3JtcyBjYW4gb25seSBiZSBzdWJtaXR0ZWQgdmlhIC5zdWJtaXQoKSBvciBhIGNsaWNrICg/KSwgYnV0IGFueXdheTogc2FuaXR5LWNoZWNrIHRoYXRcbiAgICAvLyB0aGUgc3VibWl0dGVyIGlzIGNvcnJlY3QgYmVmb3JlIHVzaW5nIGl0cyB2YWx1ZSBhcyAucmV0dXJuVmFsdWUuXG4gICAgdmFyIHMgPSBkaWFsb2dQb2x5ZmlsbC5mb3JtU3VibWl0dGVyO1xuICAgIGlmIChzICYmIHMuZm9ybSA9PT0gZm9ybSkge1xuICAgICAgZGlhbG9nLmNsb3NlKGRpYWxvZ1BvbHlmaWxsLnVzZVZhbHVlIHx8IHMudmFsdWUpO1xuICAgIH0gZWxzZSB7XG4gICAgICBkaWFsb2cuY2xvc2UoKTtcbiAgICB9XG4gICAgZGlhbG9nUG9seWZpbGwuZm9ybVN1Ym1pdHRlciA9IG51bGw7XG5cbiAgfSwgZmFsc2UpO1xufVxuXG5leHBvcnQgZGVmYXVsdCBkaWFsb2dQb2x5ZmlsbDtcbiIsICJmdW5jdGlvbiByZWdpc3RlckhlYWRlckxpc3RlbmVycygpIHtcbiAgY29uc3QgaGVhZGVyID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLWhlYWRlcicpO1xuICBjb25zdCBtZW51QnV0dG9ucyA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwoJy5qcy1oZWFkZXJNZW51QnV0dG9uJyk7XG4gIG1lbnVCdXR0b25zLmZvckVhY2goYnV0dG9uID0+IHtcbiAgICBidXR0b24uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgIGUucHJldmVudERlZmF1bHQoKTtcbiAgICAgIGhlYWRlcj8uY2xhc3NMaXN0LnRvZ2dsZSgnaXMtYWN0aXZlJyk7XG4gICAgICBidXR0b24uc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgU3RyaW5nKGhlYWRlcj8uY2xhc3NMaXN0LmNvbnRhaW5zKCdpcy1hY3RpdmUnKSkpO1xuICAgIH0pO1xuICB9KTtcblxuICBjb25zdCBzY3JpbSA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3IoJy5qcy1zY3JpbScpO1xuICBzY3JpbT8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICBlLnByZXZlbnREZWZhdWx0KCk7XG4gICAgaGVhZGVyPy5jbGFzc0xpc3QucmVtb3ZlKCdpcy1hY3RpdmUnKTtcbiAgICBtZW51QnV0dG9ucy5mb3JFYWNoKGJ1dHRvbiA9PiB7XG4gICAgICBidXR0b24uc2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJywgU3RyaW5nKGhlYWRlcj8uY2xhc3NMaXN0LmNvbnRhaW5zKCdpcy1hY3RpdmUnKSkpO1xuICAgIH0pO1xuICB9KTtcbn1cblxuZnVuY3Rpb24gcmVnaXN0ZXJTZWFyY2hGb3JtTGlzdGVuZXJzKCkge1xuICBjb25zdCBzZWFyY2hGb3JtID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLXNlYXJjaEZvcm0nKTtcbiAgY29uc3QgZXhwYW5kU2VhcmNoID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLWV4cGFuZFNlYXJjaCcpO1xuICBjb25zdCBpbnB1dCA9IHNlYXJjaEZvcm0/LnF1ZXJ5U2VsZWN0b3IoJ2lucHV0Jyk7XG4gIGNvbnN0IGhlYWRlckxvZ28gPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yKCcuanMtaGVhZGVyTG9nbycpO1xuICBjb25zdCBtZW51QnV0dG9uID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcignLmpzLWhlYWRlck1lbnVCdXR0b24nKTtcbiAgZXhwYW5kU2VhcmNoPy5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICBzZWFyY2hGb3JtPy5jbGFzc0xpc3QuYWRkKCdnby1TZWFyY2hGb3JtLS1leHBhbmRlZCcpO1xuICAgIGhlYWRlckxvZ28/LmNsYXNzTGlzdC5hZGQoJ2dvLUhlYWRlci1sb2dvLS1oaWRkZW4nKTtcbiAgICBtZW51QnV0dG9uPy5jbGFzc0xpc3QuYWRkKCdnby1IZWFkZXItbmF2T3Blbi0taGlkZGVuJyk7XG4gICAgaW5wdXQ/LmZvY3VzKCk7XG4gIH0pO1xuICBkb2N1bWVudD8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICBpZiAoIXNlYXJjaEZvcm0/LmNvbnRhaW5zKGUudGFyZ2V0IGFzIE5vZGUpKSB7XG4gICAgICBzZWFyY2hGb3JtPy5jbGFzc0xpc3QucmVtb3ZlKCdnby1TZWFyY2hGb3JtLS1leHBhbmRlZCcpO1xuICAgICAgaGVhZGVyTG9nbz8uY2xhc3NMaXN0LnJlbW92ZSgnZ28tSGVhZGVyLWxvZ28tLWhpZGRlbicpO1xuICAgICAgbWVudUJ1dHRvbj8uY2xhc3NMaXN0LnJlbW92ZSgnZ28tSGVhZGVyLW5hdk9wZW4tLWhpZGRlbicpO1xuICAgIH1cbiAgfSk7XG59XG5cbi8qKlxuICogTGlzdGVuIGZvciBjaGFuZ2VzIGluIHRoZSBzZWFyY2ggZHJvcGRvd24uXG4gKlxuICogVE9ETyhodHRwczovL2dvbGFuZy5vcmcvaXNzdWUvNDQxNDIpOiBGaXggdGhpcyBmbG93OlxuICogQSB1c2VyIHdpbGwgbGlrZWx5IGV4cGVjdCB0byBzdWJtaXQgdGhlIHNlYXJjaCBhZ2FpbiBhZnRlciBzZWxlY3RpbmcgdGhlXG4gKiB0eXBlIG9mIHNlYXJjaC4gVGhlIGNoYW5nZSBldmVudCBzaG91bGQgdHJpZ2dlciBhIGZvcm0gc3VibWlzc2lvbiwgc28gdGhhdCB0aGVcbiAqIHNlYXJjaCBldmVudCBpcyBzdGlsbCBjYXB0dXJlZCBpbiBhbmFseXRpY3Mgd2l0aG91dCBhIG1hbnVhbCBpbnN0cnVtZW50YXRpb24uXG4gKi9cbmRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwoJy5qcy1zZWFyY2hNb2RlU2VsZWN0JykuZm9yRWFjaChlbCA9PiB7XG4gIGVsLmFkZEV2ZW50TGlzdGVuZXIoJ2NoYW5nZScsIGUgPT4ge1xuICAgIGNvbnN0IHVybFNlYXJjaFBhcmFtcyA9IG5ldyBVUkxTZWFyY2hQYXJhbXMod2luZG93LmxvY2F0aW9uLnNlYXJjaCk7XG4gICAgY29uc3QgcGFyYW1zID0gT2JqZWN0LmZyb21FbnRyaWVzKHVybFNlYXJjaFBhcmFtcy5lbnRyaWVzKCkpO1xuICAgIGNvbnN0IHF1ZXJ5ID0gcGFyYW1zWydxJ107XG4gICAgaWYgKHF1ZXJ5KSB7XG4gICAgICB3aW5kb3cubG9jYXRpb24uc2VhcmNoID0gYHE9JHtxdWVyeX0mbT0keyhlLnRhcmdldCBhcyBIVE1MU2VsZWN0RWxlbWVudCkudmFsdWV9YDtcbiAgICB9XG4gIH0pO1xufSk7XG5cbnJlZ2lzdGVySGVhZGVyTGlzdGVuZXJzKCk7XG5yZWdpc3RlclNlYXJjaEZvcm1MaXN0ZW5lcnMoKTtcbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuLyoqXG4gKiBUaGlzIGNsYXNzIGRlY29yYXRlcyBhbiBlbGVtZW50IHRvIGNvcHkgYXJiaXRyYXJ5IGRhdGEgYXR0YWNoZWQgdmlhIGEgZGF0YS1cbiAqIGF0dHJpYnV0ZSB0byB0aGUgY2xpcGJvYXJkLlxuICovXG5leHBvcnQgY2xhc3MgQ2xpcGJvYXJkQ29udHJvbGxlciB7XG4gIC8qKlxuICAgKiBUaGUgZGF0YSB0byBiZSBjb3BpZWQgdG8gdGhlIGNsaXBib2FyZC5cbiAgICovXG4gIHByaXZhdGUgZGF0YTogc3RyaW5nO1xuXG4gIC8qKlxuICAgKiBAcGFyYW0gZWwgVGhlIGVsZW1lbnQgdGhhdCB3aWxsIHRyaWdnZXIgY29weWluZyB0ZXh0IHRvIHRoZSBjbGlwYm9hcmQuIFRoZSB0ZXh0IGlzXG4gICAqIGV4cGVjdGVkIHRvIGJlIHdpdGhpbiBpdHMgZGF0YS10by1jb3B5IGF0dHJpYnV0ZS5cbiAgICovXG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEhUTUxCdXR0b25FbGVtZW50KSB7XG4gICAgdGhpcy5kYXRhID0gZWwuZGF0YXNldFsndG9Db3B5J10gPz8gZWwuaW5uZXJUZXh0O1xuICAgIC8vIGlmIGRhdGEtdG8tY29weSBpcyBlbXB0eSBhbmQgdGhlIGJ1dHRvbiBpcyBwYXJ0IG9mIGFuIGlucHV0IGdyb3VwXG4gICAgLy8gY2FwdHVyZSB0aGUgdmFsdWUgb2YgdGhlIGlucHV0LlxuICAgIGlmICghdGhpcy5kYXRhICYmIGVsLnBhcmVudEVsZW1lbnQ/LmNsYXNzTGlzdC5jb250YWlucygnZ28tSW5wdXRHcm91cCcpKSB7XG4gICAgICB0aGlzLmRhdGEgPSAodGhpcy5kYXRhIHx8IGVsLnBhcmVudEVsZW1lbnQ/LnF1ZXJ5U2VsZWN0b3IoJ2lucHV0Jyk/LnZhbHVlKSA/PyAnJztcbiAgICB9XG4gICAgZWwuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHRoaXMuaGFuZGxlQ29weUNsaWNrKGUpKTtcbiAgfVxuXG4gIC8qKlxuICAgKiBIYW5kbGVzIHdoZW4gdGhlIHByaW1hcnkgZWxlbWVudCBpcyBjbGlja2VkLlxuICAgKi9cbiAgaGFuZGxlQ29weUNsaWNrKGU6IE1vdXNlRXZlbnQpOiB2b2lkIHtcbiAgICBlLnByZXZlbnREZWZhdWx0KCk7XG4gICAgY29uc3QgVE9PTFRJUF9TSE9XX0RVUkFUSU9OX01TID0gMTAwMDtcblxuICAgIC8vIFRoaXMgQVBJIGlzIG5vdCBhdmFpbGFibGUgb24gaU9TLlxuICAgIGlmICghbmF2aWdhdG9yLmNsaXBib2FyZCkge1xuICAgICAgdGhpcy5zaG93VG9vbHRpcFRleHQoJ1VuYWJsZSB0byBjb3B5JywgVE9PTFRJUF9TSE9XX0RVUkFUSU9OX01TKTtcbiAgICAgIHJldHVybjtcbiAgICB9XG4gICAgbmF2aWdhdG9yLmNsaXBib2FyZFxuICAgICAgLndyaXRlVGV4dCh0aGlzLmRhdGEpXG4gICAgICAudGhlbigoKSA9PiB7XG4gICAgICAgIHRoaXMuc2hvd1Rvb2x0aXBUZXh0KCdDb3BpZWQhJywgVE9PTFRJUF9TSE9XX0RVUkFUSU9OX01TKTtcbiAgICAgIH0pXG4gICAgICAuY2F0Y2goKCkgPT4ge1xuICAgICAgICB0aGlzLnNob3dUb29sdGlwVGV4dCgnVW5hYmxlIHRvIGNvcHknLCBUT09MVElQX1NIT1dfRFVSQVRJT05fTVMpO1xuICAgICAgfSk7XG4gIH1cblxuICAvKipcbiAgICogU2hvd3MgdGhlIGdpdmVuIHRleHQgaW4gYSB0b29sdGlwIGZvciBhIHNwZWNpZmllZCBhbW91bnQgb2YgdGltZSwgaW4gbWlsbGlzZWNvbmRzLlxuICAgKi9cbiAgc2hvd1Rvb2x0aXBUZXh0KHRleHQ6IHN0cmluZywgZHVyYXRpb25NczogbnVtYmVyKTogdm9pZCB7XG4gICAgdGhpcy5lbC5zZXRBdHRyaWJ1dGUoJ2RhdGEtdG9vbHRpcCcsIHRleHQpO1xuICAgIHNldFRpbWVvdXQoKCkgPT4gdGhpcy5lbC5zZXRBdHRyaWJ1dGUoJ2RhdGEtdG9vbHRpcCcsICcnKSwgZHVyYXRpb25Ncyk7XG4gIH1cbn1cbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuLyoqXG4gKiBUb29sVGlwQ29udHJvbGxlciBoYW5kbGVzIGNsb3NpbmcgdG9vbHRpcHMgb24gZXh0ZXJuYWwgY2xpY2tzLlxuICovXG5leHBvcnQgY2xhc3MgVG9vbFRpcENvbnRyb2xsZXIge1xuICBjb25zdHJ1Y3Rvcihwcml2YXRlIGVsOiBIVE1MRGV0YWlsc0VsZW1lbnQpIHtcbiAgICBkb2N1bWVudC5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsIGUgPT4ge1xuICAgICAgY29uc3QgaW5zaWRlVG9vbHRpcCA9IHRoaXMuZWwuY29udGFpbnMoZS50YXJnZXQgYXMgRWxlbWVudCk7XG4gICAgICBpZiAoIWluc2lkZVRvb2x0aXApIHtcbiAgICAgICAgdGhpcy5lbC5yZW1vdmVBdHRyaWJ1dGUoJ29wZW4nKTtcbiAgICAgIH1cbiAgICB9KTtcbiAgfVxufVxuIiwgIi8qKlxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG5pbXBvcnQgeyBUcmVlTmF2Q29udHJvbGxlciB9IGZyb20gJy4vdHJlZS5qcyc7XG5cbmV4cG9ydCBjbGFzcyBTZWxlY3ROYXZDb250cm9sbGVyIHtcbiAgY29uc3RydWN0b3IocHJpdmF0ZSBlbDogRWxlbWVudCkge1xuICAgIHRoaXMuZWwuYWRkRXZlbnRMaXN0ZW5lcignY2hhbmdlJywgZSA9PiB7XG4gICAgICBjb25zdCB0YXJnZXQgPSBlLnRhcmdldCBhcyBIVE1MU2VsZWN0RWxlbWVudDtcbiAgICAgIGxldCBocmVmID0gdGFyZ2V0LnZhbHVlO1xuICAgICAgaWYgKCF0YXJnZXQudmFsdWUuc3RhcnRzV2l0aCgnLycpKSB7XG4gICAgICAgIGhyZWYgPSAnLycgKyBocmVmO1xuICAgICAgfVxuICAgICAgd2luZG93LmxvY2F0aW9uLmhyZWYgPSBocmVmO1xuICAgIH0pO1xuICB9XG59XG5cbmV4cG9ydCBmdW5jdGlvbiBtYWtlU2VsZWN0TmF2KHRyZWU6IFRyZWVOYXZDb250cm9sbGVyKTogSFRNTExhYmVsRWxlbWVudCB7XG4gIGNvbnN0IGxhYmVsID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnbGFiZWwnKTtcbiAgbGFiZWwuY2xhc3NMaXN0LmFkZCgnZ28tTGFiZWwnKTtcbiAgbGFiZWwuc2V0QXR0cmlidXRlKCdhcmlhLWxhYmVsJywgJ01lbnUnKTtcbiAgY29uc3Qgc2VsZWN0ID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnc2VsZWN0Jyk7XG4gIHNlbGVjdC5jbGFzc0xpc3QuYWRkKCdnby1TZWxlY3QnLCAnanMtc2VsZWN0TmF2Jyk7XG4gIGxhYmVsLmFwcGVuZENoaWxkKHNlbGVjdCk7XG4gIGNvbnN0IG91dGxpbmUgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdvcHRncm91cCcpO1xuICBvdXRsaW5lLmxhYmVsID0gJ091dGxpbmUnO1xuICBzZWxlY3QuYXBwZW5kQ2hpbGQob3V0bGluZSk7XG4gIGNvbnN0IGdyb3VwTWFwOiBSZWNvcmQ8c3RyaW5nLCBIVE1MT3B0R3JvdXBFbGVtZW50PiA9IHt9O1xuICBsZXQgZ3JvdXA6IEhUTUxPcHRHcm91cEVsZW1lbnQ7XG4gIGZvciAoY29uc3QgdCBvZiB0cmVlLnRyZWVpdGVtcykge1xuICAgIGlmIChOdW1iZXIodC5kZXB0aCkgPiA0KSBjb250aW51ZTtcbiAgICBpZiAodC5ncm91cFRyZWVpdGVtKSB7XG4gICAgICBncm91cCA9IGdyb3VwTWFwW3QuZ3JvdXBUcmVlaXRlbS5sYWJlbF07XG4gICAgICBpZiAoIWdyb3VwKSB7XG4gICAgICAgIGdyb3VwID0gZ3JvdXBNYXBbdC5ncm91cFRyZWVpdGVtLmxhYmVsXSA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ29wdGdyb3VwJyk7XG4gICAgICAgIGdyb3VwLmxhYmVsID0gdC5ncm91cFRyZWVpdGVtLmxhYmVsO1xuICAgICAgICBzZWxlY3QuYXBwZW5kQ2hpbGQoZ3JvdXApO1xuICAgICAgfVxuICAgIH0gZWxzZSB7XG4gICAgICBncm91cCA9IG91dGxpbmU7XG4gICAgfVxuICAgIGNvbnN0IG8gPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdvcHRpb24nKTtcbiAgICBvLmxhYmVsID0gdC5sYWJlbDtcbiAgICBvLnRleHRDb250ZW50ID0gdC5sYWJlbDtcbiAgICBvLnZhbHVlID0gKHQuZWwgYXMgSFRNTEFuY2hvckVsZW1lbnQpLmhyZWYucmVwbGFjZSh3aW5kb3cubG9jYXRpb24ub3JpZ2luLCAnJykucmVwbGFjZSgnLycsICcnKTtcbiAgICBncm91cC5hcHBlbmRDaGlsZChvKTtcbiAgfVxuICB0cmVlLmFkZE9ic2VydmVyKHQgPT4ge1xuICAgIGNvbnN0IGhhc2ggPSAodC5lbCBhcyBIVE1MQW5jaG9yRWxlbWVudCkuaGFzaDtcbiAgICBjb25zdCB2YWx1ZSA9IHNlbGVjdC5xdWVyeVNlbGVjdG9yPEhUTUxPcHRpb25FbGVtZW50PihgW3ZhbHVlJD1cIiR7aGFzaH1cIl1gKT8udmFsdWU7XG4gICAgaWYgKHZhbHVlKSB7XG4gICAgICBzZWxlY3QudmFsdWUgPSB2YWx1ZTtcbiAgICB9XG4gIH0sIDUwKTtcbiAgcmV0dXJuIGxhYmVsO1xufVxuIiwgIi8qKlxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG4vKipcbiAqIE1vZGFsQ29udHJvbGxlciByZWdpc3RlcnMgYSBkaWFsb2cgZWxlbWVudCB3aXRoIHRoZSBwb2x5ZmlsbCBpZlxuICogbmVjZXNzYXJ5IGZvciB0aGUgY3VycmVudCBicm93c2VyLCBhZGQgYWRkcyBldmVudCBsaXN0ZW5lcnMgdG9cbiAqIGNsb3NlIGFuZCBvcGVuIG1vZGFscy5cbiAqL1xuZXhwb3J0IGNsYXNzIE1vZGFsQ29udHJvbGxlciB7XG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEhUTUxEaWFsb2dFbGVtZW50KSB7XG4gICAgLy8gT25seSBsb2FkIHRoZSBkaWFsb2cgcG9seWZpbGwgaWYgbmVjZXNzYXJ5IGZvciB0aGUgZW52aXJvbm1lbnQuXG4gICAgaWYgKCF3aW5kb3cuSFRNTERpYWxvZ0VsZW1lbnQgJiYgIWVsLnNob3dNb2RhbCkge1xuICAgICAgaW1wb3J0KCcuLi8uLi8uLi90aGlyZF9wYXJ0eS9kaWFsb2ctcG9seWZpbGwvZGlhbG9nLXBvbHlmaWxsLmVzbS5qcycpLnRoZW4oXG4gICAgICAgICh7IGRlZmF1bHQ6IHBvbHlmaWxsIH0pID0+IHtcbiAgICAgICAgICBwb2x5ZmlsbC5yZWdpc3RlckRpYWxvZyhlbCk7XG4gICAgICAgIH1cbiAgICAgICk7XG4gICAgfVxuICAgIGNvbnN0IGlkID0gZWwuaWQ7XG4gICAgY29uc3QgYnV0dG9uID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MQnV0dG9uRWxlbWVudD4oYFthcmlhLWNvbnRyb2xzPVwiJHtpZH1cIl1gKTtcbiAgICBpZiAoYnV0dG9uKSB7XG4gICAgICBidXR0b24uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICAgIGlmICh0aGlzLmVsLnNob3dNb2RhbCkge1xuICAgICAgICAgIHRoaXMuZWwuc2hvd01vZGFsKCk7XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgdGhpcy5lbC5vcGVuID0gdHJ1ZTtcbiAgICAgICAgfVxuICAgICAgICBlbC5xdWVyeVNlbGVjdG9yKCdpbnB1dCcpPy5mb2N1cygpO1xuICAgICAgfSk7XG4gICAgfVxuICAgIGZvciAoY29uc3QgY2xvc2Ugb2YgdGhpcy5lbC5xdWVyeVNlbGVjdG9yQWxsPEhUTUxCdXR0b25FbGVtZW50PignW2RhdGEtbW9kYWwtY2xvc2VdJykpIHtcbiAgICAgIGNsb3NlLmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4ge1xuICAgICAgICBpZiAodGhpcy5lbC5jbG9zZSkge1xuICAgICAgICAgIHRoaXMuZWwuY2xvc2UoKTtcbiAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICB0aGlzLmVsLm9wZW4gPSBmYWxzZTtcbiAgICAgICAgfVxuICAgICAgfSk7XG4gICAgfVxuICB9XG59XG4iLCAiaW50ZXJmYWNlIFRhZ01hbmFnZXJFdmVudCB7XG4gIC8qKlxuICAgKiBldmVudCBpcyB0aGUgbmFtZSBvZiB0aGUgZXZlbnQsIHVzZWQgdG8gZmlsdGVyIGV2ZW50cyBpblxuICAgKiBHb29nbGUgQW5hbHl0aWNzLlxuICAgKi9cbiAgZXZlbnQ6IHN0cmluZztcblxuICAvKipcbiAgICogZXZlbnRfY2F0ZWdvcnkgaXMgYSBuYW1lIHRoYXQgeW91IHN1cHBseSBhcyBhIHdheSB0byBncm91cCBvYmplY3RzXG4gICAqIHRoYXQgdG8gYW5hbHl6ZS4gVHlwaWNhbGx5LCB5b3Ugd2lsbCB1c2UgdGhlIHNhbWUgY2F0ZWdvcnkgbmFtZVxuICAgKiBtdWx0aXBsZSB0aW1lcyBvdmVyIHJlbGF0ZWQgVUkgZWxlbWVudHMgKGJ1dHRvbnMsIGxpbmtzLCBldGMpLlxuICAgKi9cbiAgZXZlbnRfY2F0ZWdvcnk/OiBzdHJpbmc7XG5cbiAgLyoqXG4gICAqIGV2ZW50X2FjdGlvbiBpcyB1c2VkIHRvIG5hbWUgdGhlIHR5cGUgb2YgZXZlbnQgb3IgaW50ZXJhY3Rpb24geW91XG4gICAqIHdhbnQgdG8gbWVhc3VyZSBmb3IgYSBwYXJ0aWN1bGFyIHdlYiBvYmplY3QuIEZvciBleGFtcGxlLCB3aXRoIGFcbiAgICogc2luZ2xlIFwiZm9ybVwiIGNhdGVnb3J5LCB5b3UgY2FuIGFuYWx5emUgYSBudW1iZXIgb2Ygc3BlY2lmaWMgZXZlbnRzXG4gICAqIHdpdGggdGhpcyBwYXJhbWV0ZXIsIHN1Y2ggYXM6IGZvcm0gZW50ZXJlZCwgZm9ybSBzdWJtaXR0ZWQuXG4gICAqL1xuICBldmVudF9hY3Rpb24/OiBzdHJpbmc7XG5cbiAgLyoqXG4gICAqIGV2ZW50X2xhYmVsIHByb3ZpZGUgYWRkaXRpb25hbCBpbmZvcm1hdGlvbiBmb3IgZXZlbnRzIHRoYXQgeW91IHdhbnRcbiAgICogdG8gYW5hbHl6ZSwgc3VjaCBhcyB0aGUgdGV4dCBsYWJlbCBvZiBhIGxpbmsuXG4gICAqL1xuICBldmVudF9sYWJlbD86IHN0cmluZztcblxuICAvKipcbiAgICogZ3RtLnN0YXJ0IGlzIHVzZWQgdG8gaW5pdGlhbGl6ZSBHb29nbGUgVGFnIE1hbmFnZXIuXG4gICAqL1xuICAnZ3RtLnN0YXJ0Jz86IG51bWJlcjtcbn1cblxuLy8gZXNsaW50LWRpc2FibGUtbmV4dC1saW5lIEB0eXBlc2NyaXB0LWVzbGludC9uby11bnVzZWQtdmFyc1xuZGVjbGFyZSBnbG9iYWwge1xuICBpbnRlcmZhY2UgV2luZG93IHtcbiAgICBkYXRhTGF5ZXI/OiAoVGFnTWFuYWdlckV2ZW50IHwgVm9pZEZ1bmN0aW9uKVtdO1xuICAgIGdhPzogdW5rbm93bjtcbiAgfVxufVxuXG4vKipcbiAqIHRyYWNrIHNlbmRzIGV2ZW50cyB0byBHb29nbGUgVGFnIE1hbmFnZXIuXG4gKi9cbmV4cG9ydCBmdW5jdGlvbiB0cmFjayhcbiAgZXZlbnQ6IHN0cmluZyB8IFRhZ01hbmFnZXJFdmVudCxcbiAgY2F0ZWdvcnk/OiBzdHJpbmcsXG4gIGFjdGlvbj86IHN0cmluZyxcbiAgbGFiZWw/OiBzdHJpbmdcbik6IHZvaWQge1xuICB3aW5kb3cuZGF0YUxheWVyID8/PSBbXTtcbiAgaWYgKHR5cGVvZiBldmVudCA9PT0gJ3N0cmluZycpIHtcbiAgICB3aW5kb3cuZGF0YUxheWVyLnB1c2goe1xuICAgICAgZXZlbnQsXG4gICAgICBldmVudF9jYXRlZ29yeTogY2F0ZWdvcnksXG4gICAgICBldmVudF9hY3Rpb246IGFjdGlvbixcbiAgICAgIGV2ZW50X2xhYmVsOiBsYWJlbCxcbiAgICB9KTtcbiAgfSBlbHNlIHtcbiAgICB3aW5kb3cuZGF0YUxheWVyLnB1c2goZXZlbnQpO1xuICB9XG59XG5cbi8qKlxuICogZnVuYyBhZGRzIGZ1bmN0aW9ucyB0byBydW4gc2VxdWVudGlvbmFsbHkgYWZ0ZXJcbiAqIEdvb2dsZSBUYWcgTWFuYWdlciBpcyByZWFkeS5cbiAqL1xuZXhwb3J0IGZ1bmN0aW9uIGZ1bmMoZm46ICgpID0+IHZvaWQpOiB2b2lkIHtcbiAgd2luZG93LmRhdGFMYXllciA/Pz0gW107XG4gIHdpbmRvdy5kYXRhTGF5ZXIucHVzaChmbik7XG59XG4iLCAiLyohXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMTktMjAyMCBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuaW1wb3J0IHsgdHJhY2sgfSBmcm9tICcuLi9hbmFseXRpY3MvYW5hbHl0aWNzJztcblxuLyoqXG4gKiBPcHRpb25zIGFyZSBrZXloYW5kbGVyIGNhbGxiYWNrIG9wdGlvbnMuXG4gKi9cbmludGVyZmFjZSBPcHRpb25zIHtcbiAgLyoqXG4gICAqIHRhcmdldCBpcyB0aGUgZWxlbWVudCB0aGUga2V5IGV2ZW50IHNob3VsZCBmaWx0ZXIgb24uIFRoZVxuICAgKiBkZWZhdWx0IHRhcmdldCBpcyB0aGUgZG9jdW1lbnQuXG4gICAqL1xuICB0YXJnZXQ/OiBFbGVtZW50O1xuXG4gIC8qKlxuICAgKiB3aXRoTWV0YSBzcGVjaWZpZXMgaWYgdGhlIGV2ZW50IGNhbGxiYWNrIHNob3VsZCBmaXJlIHdoZW5cbiAgICogdGhlIGtleSBpcyBwcmVzc2VkIHdpdGggYSBtZXRhIGtleSAoY3RybCwgYWx0LCBldGMpLiBCeVxuICAgKiBkZWZhdWx0IG1ldGEga2V5cHJlc3NlcyBhcmUgaWdub3JlZC5cbiAgICovXG4gIHdpdGhNZXRhPzogYm9vbGVhbjtcbn1cblxuLyoqXG4gKiBLZXlIYW5kbGVyIGlzIHRoZSBjb25maWcgZm9yIGEga2V5Ym9hcmQgZXZlbnQgY2FsbGJhY2suXG4gKi9cbmludGVyZmFjZSBLZXlIYW5kbGVyIGV4dGVuZHMgT3B0aW9ucyB7XG4gIGRlc2NyaXB0aW9uOiBzdHJpbmc7XG4gIGNhbGxiYWNrOiAoZTogS2V5Ym9hcmRFdmVudCkgPT4gdm9pZDtcbn1cblxuLyoqXG4gKiBLZXlib2FyZENvbnRyb2xsZXIgY29udHJvbHMgZXZlbnQgY2FsbGJhY2tzIGZvciBzaXRld2lkZVxuICoga2V5Ym9hcmQgZXZlbnRzLiBNdWx0aXBsZSBjYWxsYmFja3MgY2FuIGJlIHJlZ2lzdGVyZWQgZm9yXG4gKiBhIHNpbmdsZSBrZXkgYW5kIGJ5IGRlZmF1bHQgdGhlIGNvbnRyb2xsZXIgaWdub3JlcyBldmVudHNcbiAqIGZvciB0ZXh0IGlucHV0IHRhcmdldHMuXG4gKi9cbmNsYXNzIEtleWJvYXJkQ29udHJvbGxlciB7XG4gIGhhbmRsZXJzOiBSZWNvcmQ8c3RyaW5nLCBTZXQ8S2V5SGFuZGxlcj4+O1xuXG4gIGNvbnN0cnVjdG9yKCkge1xuICAgIHRoaXMuaGFuZGxlcnMgPSB7fTtcbiAgICBkb2N1bWVudC5hZGRFdmVudExpc3RlbmVyKCdrZXlkb3duJywgZSA9PiB0aGlzLmhhbmRsZUtleVByZXNzKGUpKTtcbiAgfVxuXG4gIC8qKlxuICAgKiBvbiByZWdpc3RlcnMga2V5Ym9hcmQgZXZlbnQgY2FsbGJhY2tzLlxuICAgKiBAcGFyYW0ga2V5IHRoZSBrZXkgdG8gcmVnaXN0ZXIuXG4gICAqIEBwYXJhbSBkZXNjcmlwdGlvbiBuYW1lIG9mIHRoZSBldmVudC5cbiAgICogQHBhcmFtIGNhbGxiYWNrIGV2ZW50IGNhbGxiYWNrLlxuICAgKiBAcGFyYW0gb3B0aW9ucyBzZXQgdGFyZ2V0IGFuZCB3aXRoTWV0YSBvcHRpb25zIHRvIG92ZXJyaWRlIHRoZSBkZWZhdWx0IGJlaGF2aW9ycy5cbiAgICovXG4gIG9uKGtleTogc3RyaW5nLCBkZXNjcmlwdGlvbjogc3RyaW5nLCBjYWxsYmFjazogKGU6IEtleWJvYXJkRXZlbnQpID0+IHZvaWQsIG9wdGlvbnM/OiBPcHRpb25zKSB7XG4gICAgdGhpcy5oYW5kbGVyc1trZXldID8/PSBuZXcgU2V0KCk7XG4gICAgdGhpcy5oYW5kbGVyc1trZXldLmFkZCh7IGRlc2NyaXB0aW9uLCBjYWxsYmFjaywgLi4ub3B0aW9ucyB9KTtcbiAgICByZXR1cm4gdGhpcztcbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlS2V5UHJlc3MoZTogS2V5Ym9hcmRFdmVudCkge1xuICAgIGZvciAoY29uc3QgaGFuZGxlciBvZiB0aGlzLmhhbmRsZXJzW2Uua2V5LnRvTG93ZXJDYXNlKCldID8/IG5ldyBTZXQoKSkge1xuICAgICAgaWYgKGhhbmRsZXIudGFyZ2V0ICYmIGhhbmRsZXIudGFyZ2V0ICE9PSBlLnRhcmdldCkge1xuICAgICAgICByZXR1cm47XG4gICAgICB9XG4gICAgICBjb25zdCB0ID0gZS50YXJnZXQgYXMgSFRNTEVsZW1lbnQgfCBudWxsO1xuICAgICAgaWYgKFxuICAgICAgICAhaGFuZGxlci50YXJnZXQgJiZcbiAgICAgICAgKHQ/LnRhZ05hbWUgPT09ICdJTlBVVCcgfHwgdD8udGFnTmFtZSA9PT0gJ1NFTEVDVCcgfHwgdD8udGFnTmFtZSA9PT0gJ1RFWFRBUkVBJylcbiAgICAgICkge1xuICAgICAgICByZXR1cm47XG4gICAgICB9XG4gICAgICBpZiAodD8uaXNDb250ZW50RWRpdGFibGUpIHtcbiAgICAgICAgcmV0dXJuO1xuICAgICAgfVxuICAgICAgaWYgKFxuICAgICAgICAoaGFuZGxlci53aXRoTWV0YSAmJiAhKGUuY3RybEtleSB8fCBlLm1ldGFLZXkpKSB8fFxuICAgICAgICAoIWhhbmRsZXIud2l0aE1ldGEgJiYgKGUuY3RybEtleSB8fCBlLm1ldGFLZXkpKVxuICAgICAgKSB7XG4gICAgICAgIHJldHVybjtcbiAgICAgIH1cbiAgICAgIHRyYWNrKCdrZXlwcmVzcycsICdob3RrZXlzJywgYCR7ZS5rZXl9IHByZXNzZWRgLCBoYW5kbGVyLmRlc2NyaXB0aW9uKTtcbiAgICAgIGhhbmRsZXIuY2FsbGJhY2soZSk7XG4gICAgfVxuICB9XG59XG5cbmV4cG9ydCBjb25zdCBrZXlib2FyZCA9IG5ldyBLZXlib2FyZENvbnRyb2xsZXIoKTtcbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMCBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuaW1wb3J0ICcuL2hlYWRlci9oZWFkZXInO1xuaW1wb3J0IHsgQ2xpcGJvYXJkQ29udHJvbGxlciB9IGZyb20gJy4vY2xpcGJvYXJkL2NsaXBib2FyZCc7XG5pbXBvcnQgeyBUb29sVGlwQ29udHJvbGxlciB9IGZyb20gJy4vdG9vbHRpcC90b29sdGlwJztcbmltcG9ydCB7IFNlbGVjdE5hdkNvbnRyb2xsZXIgfSBmcm9tICcuL291dGxpbmUvc2VsZWN0JztcbmltcG9ydCB7IE1vZGFsQ29udHJvbGxlciB9IGZyb20gJy4vbW9kYWwvbW9kYWwnO1xuXG5leHBvcnQgeyBrZXlib2FyZCB9IGZyb20gJy4va2V5Ym9hcmQva2V5Ym9hcmQnO1xuZXhwb3J0ICogYXMgYW5hbHl0aWNzIGZyb20gJy4vYW5hbHl0aWNzL2FuYWx5dGljcyc7XG5cbmZvciAoY29uc3QgZWwgb2YgZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbDxIVE1MQnV0dG9uRWxlbWVudD4oJy5qcy1jbGlwYm9hcmQnKSkge1xuICBuZXcgQ2xpcGJvYXJkQ29udHJvbGxlcihlbCk7XG59XG5cbmZvciAoY29uc3QgZWwgb2YgZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbDxIVE1MRGlhbG9nRWxlbWVudD4oJy5qcy1tb2RhbCcpKSB7XG4gIG5ldyBNb2RhbENvbnRyb2xsZXIoZWwpO1xufVxuXG5mb3IgKGNvbnN0IHQgb2YgZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbDxIVE1MRGV0YWlsc0VsZW1lbnQ+KCcuanMtdG9vbHRpcCcpKSB7XG4gIG5ldyBUb29sVGlwQ29udHJvbGxlcih0KTtcbn1cblxuZm9yIChjb25zdCBlbCBvZiBkb2N1bWVudC5xdWVyeVNlbGVjdG9yQWxsPEhUTUxTZWxlY3RFbGVtZW50PignLmpzLXNlbGVjdE5hdicpKSB7XG4gIG5ldyBTZWxlY3ROYXZDb250cm9sbGVyKGVsKTtcbn1cbiIsICIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMCBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuaW1wb3J0IHsgYW5hbHl0aWNzLCBrZXlib2FyZCB9IGZyb20gJy4uL3NoYXJlZC9zaGFyZWQnO1xuXG4vLyBUZW1wb3Jhcnkgc2hvcnRjdXQgZm9yIHRlc3Rpbmcgb3V0IHRoZSBkYXJrIHRoZW1lLlxua2V5Ym9hcmQub24oJ3QnLCAndG9nZ2xlIHRoZW1lJywgKCkgPT4ge1xuICBsZXQgbmV4dFRoZW1lID0gJ2RhcmsnO1xuICBjb25zdCB0aGVtZSA9IGRvY3VtZW50LmRvY3VtZW50RWxlbWVudC5nZXRBdHRyaWJ1dGUoJ2RhdGEtdGhlbWUnKTtcbiAgaWYgKHRoZW1lID09PSAnZGFyaycpIHtcbiAgICBuZXh0VGhlbWUgPSAnbGlnaHQnO1xuICB9IGVsc2UgaWYgKHRoZW1lID09PSAnbGlnaHQnKSB7XG4gICAgbmV4dFRoZW1lID0gJ2F1dG8nO1xuICB9XG4gIGRvY3VtZW50LmRvY3VtZW50RWxlbWVudC5zZXRBdHRyaWJ1dGUoJ2RhdGEtdGhlbWUnLCBuZXh0VGhlbWUpO1xuICBkb2N1bWVudC5jb29raWUgPSBgcHJlZmVycy1jb2xvci1zY2hlbWU9JHtuZXh0VGhlbWV9O3BhdGg9LzttYXgtYWdlPTMxNTM2MDAwO2A7XG59KTtcblxuLy8gUHJlc3NpbmcgJy8nIGZvY3VzZXMgdGhlIHNlYXJjaCBib3hcbmtleWJvYXJkLm9uKCcvJywgJ2ZvY3VzIHNlYXJjaCcsIGUgPT4ge1xuICBjb25zdCBzZWFyY2hJbnB1dCA9IEFycmF5LmZyb20oXG4gICAgZG9jdW1lbnQucXVlcnlTZWxlY3RvckFsbDxIVE1MSW5wdXRFbGVtZW50PignLmpzLXNlYXJjaEZvY3VzJylcbiAgKS5wb3AoKTtcbiAgLy8gRmF2b3JpbmcgdGhlIEZpcmVmb3ggcXVpY2sgZmluZCBmZWF0dXJlIG92ZXIgc2VhcmNoIGlucHV0XG4gIC8vIGZvY3VzLiBTZWU6IGh0dHBzOi8vZ2l0aHViLmNvbS9nb2xhbmcvZ28vaXNzdWVzLzQxMDkzLlxuICBpZiAoc2VhcmNoSW5wdXQgJiYgIXdpbmRvdy5uYXZpZ2F0b3IudXNlckFnZW50LmluY2x1ZGVzKCdGaXJlZm94JykpIHtcbiAgICBlLnByZXZlbnREZWZhdWx0KCk7XG4gICAgc2VhcmNoSW5wdXQuZm9jdXMoKTtcbiAgfVxufSk7XG5cbi8vIFByZXNzaW5nICd5JyBjaGFuZ2VzIHRoZSBicm93c2VyIFVSTCB0byB0aGUgY2Fub25pY2FsIFVSTFxuLy8gd2l0aG91dCB0cmlnZ2VyaW5nIGEgcmVsb2FkLlxua2V5Ym9hcmQub24oJ3knLCAnc2V0IGNhbm9uaWNhbCB1cmwnLCAoKSA9PiB7XG4gIGNvbnN0IGNhbm9uaWNhbFVSTFBhdGggPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxEaXZFbGVtZW50PignLmpzLWNhbm9uaWNhbFVSTFBhdGgnKT8uZGF0YXNldFtcbiAgICAnY2Fub25pY2FsVXJsUGF0aCdcbiAgXTtcbiAgaWYgKGNhbm9uaWNhbFVSTFBhdGggJiYgY2Fub25pY2FsVVJMUGF0aCAhPT0gJycpIHtcbiAgICB3aW5kb3cuaGlzdG9yeS5yZXBsYWNlU3RhdGUobnVsbCwgJycsIGNhbm9uaWNhbFVSTFBhdGgpO1xuICB9XG59KTtcblxuLyoqXG4gKiBzZXR1cEdvb2dsZVRhZ01hbmFnZXIgaW50aWFsaXplcyBHb29nbGUgVGFnIE1hbmFnZXIuXG4gKi9cbihmdW5jdGlvbiBzZXR1cEdvb2dsZVRhZ01hbmFnZXIoKSB7XG4gIGFuYWx5dGljcy50cmFjayh7XG4gICAgJ2d0bS5zdGFydCc6IG5ldyBEYXRlKCkuZ2V0VGltZSgpLFxuICAgIGV2ZW50OiAnZ3RtLmpzJyxcbiAgfSk7XG59KSgpO1xuXG4vKipcbiAqIHJlbW92ZVVUTVNvdXJjZSByZW1vdmVzIHRoZSB1dG1fc291cmNlIEdFVCBwYXJhbWV0ZXIgaWYgcHJlc2VudC5cbiAqIFRoaXMgaXMgZG9uZSB1c2luZyBKYXZhU2NyaXB0LCBzbyB0aGF0IHRoZSB1dG1fc291cmNlIGlzIHN0aWxsXG4gKiBjYXB0dXJlZCBieSBHb29nbGUgQW5hbHl0aWNzLlxuICovXG5mdW5jdGlvbiByZW1vdmVVVE1Tb3VyY2UoKSB7XG4gIGNvbnN0IHVybFBhcmFtcyA9IG5ldyBVUkxTZWFyY2hQYXJhbXMod2luZG93LmxvY2F0aW9uLnNlYXJjaCk7XG4gIGNvbnN0IHV0bVNvdXJjZSA9IHVybFBhcmFtcy5nZXQoJ3V0bV9zb3VyY2UnKTtcbiAgaWYgKHV0bVNvdXJjZSAhPT0gJ2dvcGxzJyAmJiB1dG1Tb3VyY2UgIT09ICdnb2RvYycgJiYgdXRtU291cmNlICE9PSAncGtnZ29kZXYnKSB7XG4gICAgcmV0dXJuO1xuICB9XG5cbiAgLyoqIFN0cmlwIHRoZSB1dG1fc291cmNlIHF1ZXJ5IHBhcmFtZXRlciBhbmQgcmVwbGFjZSB0aGUgVVJMLiAqKi9cbiAgY29uc3QgbmV3VVJMID0gbmV3IFVSTCh3aW5kb3cubG9jYXRpb24uaHJlZik7XG4gIHVybFBhcmFtcy5kZWxldGUoJ3V0bV9zb3VyY2UnKTtcbiAgbmV3VVJMLnNlYXJjaCA9IHVybFBhcmFtcy50b1N0cmluZygpO1xuICB3aW5kb3cuaGlzdG9yeS5yZXBsYWNlU3RhdGUobnVsbCwgJycsIG5ld1VSTC50b1N0cmluZygpKTtcbn1cblxuaWYgKGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEVsZW1lbnQ+KCcuanMtZ3RtSUQnKT8uZGF0YXNldC5ndG1pZCAmJiB3aW5kb3cuZGF0YUxheWVyKSB7XG4gIGFuYWx5dGljcy5mdW5jKGZ1bmN0aW9uICgpIHtcbiAgICByZW1vdmVVVE1Tb3VyY2UoKTtcbiAgfSk7XG59IGVsc2Uge1xuICByZW1vdmVVVE1Tb3VyY2UoKTtcbn1cbiJdLAogICJtYXBwaW5ncyI6ICI7Ozs7Ozs7Ozs7Ozs7Ozs7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQ0EsUUFBSSxxQkFBcUIsT0FBTztBQUNoQyxRQUFJLENBQUMsc0JBQXNCLE9BQU8sdUJBQXVCLFVBQVU7QUFDakUsMkJBQXFCLHFCQUFxQixPQUFPLEdBQUc7QUFDbEQsWUFBSSxLQUFLO0FBQ1QsWUFBSSxLQUFLLFNBQVMsWUFBWTtBQUM5QixXQUFHLGdCQUFnQixPQUFPLENBQUMsQ0FBQyxFQUFFLFNBQVMsQ0FBQyxDQUFDLEVBQUUsWUFBWSxFQUFFLFVBQVU7QUFDbkUsZUFBTztBQUFBO0FBRVQseUJBQW1CLFlBQVksT0FBTyxNQUFNO0FBQUE7QUFPOUMsb0NBQWdDLElBQUk7QUFDbEMsYUFBTyxNQUFNLE9BQU8sU0FBUyxNQUFNO0FBQ2pDLFlBQUksSUFBSSxPQUFPLGlCQUFpQjtBQUNoQyxZQUFJLFVBQVUsU0FBUyxHQUFHLElBQUk7QUFDNUIsaUJBQU8sQ0FBRSxHQUFFLE9BQU8sVUFBYSxFQUFFLE9BQU87QUFBQTtBQUcxQyxZQUFJLEVBQUUsVUFBVSxLQUNaLFFBQVEsVUFBVSxXQUNsQixRQUFRLGFBQWEsV0FDckIsUUFBUSxnQkFBZ0IsYUFDeEIsUUFBUSxVQUFVLFdBQ2xCLFFBQVEsZUFBZSxXQUN2QixFQUFFLGlCQUFpQixhQUNuQixFQUFFLGFBQWEsV0FDZixF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
