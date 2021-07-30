(() => {
  // static/shared/table/table.ts
  /*!
   * @license
   * Copyright 2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var ExpandableRowsTableController = class {
    constructor(table, toggleAll) {
      this.table = table;
      this.toggleAll = toggleAll;
      this.expandAllItems = () => {
        this.toggles.map((t) => t.setAttribute("aria-expanded", "true"));
        this.update();
      };
      this.collapseAllItems = () => {
        this.toggles.map((t) => t.setAttribute("aria-expanded", "false"));
        this.update();
      };
      this.update = () => {
        this.updateVisibleItems();
        setTimeout(() => this.updateGlobalToggle());
      };
      this.rows = Array.from(table.querySelectorAll("[data-aria-controls]"));
      this.toggles = Array.from(this.table.querySelectorAll("[aria-expanded]"));
      this.setAttributes();
      this.attachEventListeners();
      this.update();
    }
    setAttributes() {
      for (const a of ["data-aria-controls", "data-aria-labelledby", "data-id"]) {
        this.table.querySelectorAll(`[${a}]`).forEach((t) => {
          t.setAttribute(a.replace("data-", ""), t.getAttribute(a) ?? "");
          t.removeAttribute(a);
        });
      }
    }
    attachEventListeners() {
      this.rows.forEach((t) => {
        t.addEventListener("click", (e) => {
          this.handleToggleClick(e);
        });
      });
      this.toggleAll?.addEventListener("click", () => {
        this.expandAllItems();
      });
      document.addEventListener("keydown", (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === "f") {
          this.expandAllItems();
        }
      });
    }
    handleToggleClick(e) {
      let target = e.currentTarget;
      if (!target?.hasAttribute("aria-expanded")) {
        target = this.table.querySelector(`button[aria-controls="${target?.getAttribute("aria-controls")}"]`);
      }
      const isExpanded = target?.getAttribute("aria-expanded") === "true";
      target?.setAttribute("aria-expanded", isExpanded ? "false" : "true");
      e.stopPropagation();
      this.update();
    }
    updateVisibleItems() {
      this.rows.map((t) => {
        const isExpanded = t?.getAttribute("aria-expanded") === "true";
        const rowIds = t?.getAttribute("aria-controls")?.trimEnd().split(" ");
        rowIds?.map((id) => {
          const target = document.getElementById(`${id}`);
          if (isExpanded) {
            target?.classList.add("visible");
            target?.classList.remove("hidden");
          } else {
            target?.classList.add("hidden");
            target?.classList.remove("visible");
          }
        });
      });
    }
    updateGlobalToggle() {
      if (!this.toggleAll)
        return;
      if (this.rows.some((t) => t.hasAttribute("aria-expanded"))) {
        this.toggleAll.style.display = "block";
      }
      const someCollapsed = this.toggles.some((el2) => el2.getAttribute("aria-expanded") === "false");
      if (someCollapsed) {
        this.toggleAll.innerText = "Expand all";
        this.toggleAll.onclick = this.expandAllItems;
      } else {
        this.toggleAll.innerText = "Collapse all";
        this.toggleAll.onclick = this.collapseAllItems;
      }
    }
  };

  // static/frontend/unit/unit.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  document.querySelectorAll(".js-expandableTable").forEach((el2) => new ExpandableRowsTableController(el2, document.querySelector(".js-expandAllDirectories")));
  var headerHeight = 3.5;
  var breadcumbHeight = 2.5;
  var MainLayoutController = class {
    constructor(mainHeader, mainNav, mainAside) {
      this.mainHeader = mainHeader;
      this.mainNav = mainNav;
      this.mainAside = mainAside;
      this.handleDoubleClick = (e) => {
        const target = e.target;
        if (target === this.mainHeader?.lastElementChild) {
          window.getSelection()?.removeAllRanges();
          window.scrollTo({top: 0, behavior: "smooth"});
        }
      };
      this.handleResize = () => {
        const setProp = (name, value) => document.documentElement.style.setProperty(name, value);
        setProp("--js-unit-header-height", "0");
        setTimeout(() => {
          const mainHeaderHeight = (this.mainHeader?.getBoundingClientRect().height ?? 0) / 16;
          setProp("--js-unit-header-height", `${mainHeaderHeight}rem`);
          setProp("--js-sticky-header-height", `${headerHeight}rem`);
          setProp("--js-unit-header-top", `${(mainHeaderHeight - headerHeight) * -1}rem`);
        });
      };
      this.headerObserver = new IntersectionObserver(([e]) => {
        if (e.intersectionRatio < 1) {
          for (const x of document.querySelectorAll('[class^="go-Main-header"')) {
            x.setAttribute("data-fixed", "true");
          }
        } else {
          for (const x of document.querySelectorAll('[class^="go-Main-header"')) {
            x.removeAttribute("data-fixed");
          }
          this.handleResize();
        }
      }, {threshold: 1, rootMargin: `${breadcumbHeight * 16}px`});
      this.navObserver = new IntersectionObserver(([e]) => {
        if (e.intersectionRatio < 1) {
          this.mainNav?.classList.add("go-Main-nav--fixed");
          this.mainNav?.setAttribute("data-fixed", "true");
        } else {
          this.mainNav?.classList.remove("go-Main-nav--fixed");
          this.mainNav?.removeAttribute("data-fixed");
        }
      }, {threshold: 1, rootMargin: `-${headerHeight * 16 + 10}px`});
      this.asideObserver = new IntersectionObserver(([e]) => {
        if (e.intersectionRatio < 1) {
          this.mainHeader?.setAttribute("data-raised", "true");
        } else {
          this.mainHeader?.removeAttribute("data-raised");
        }
      }, {threshold: 1, rootMargin: `-${headerHeight * 16 + 20}px 0px 0px 0px`});
      this.init();
    }
    init() {
      this.handleResize();
      window.addEventListener("resize", this.handleResize);
      this.mainHeader?.addEventListener("dblclick", this.handleDoubleClick);
      if (this.mainHeader?.hasChildNodes()) {
        const headerSentinel = document.createElement("div");
        this.mainHeader.prepend(headerSentinel);
        this.headerObserver.observe(headerSentinel);
      }
      if (this.mainNav?.hasChildNodes()) {
        const navSentinel = document.createElement("div");
        this.mainNav.prepend(navSentinel);
        this.navObserver.observe(navSentinel);
      }
      if (this.mainAside) {
        const asideSentinel = document.createElement("div");
        this.mainAside.prepend(asideSentinel);
        this.asideObserver.observe(asideSentinel);
      }
    }
  };
  var el = (selector) => document.querySelector(selector);
  new MainLayoutController(el(".js-mainHeader"), el(".js-mainNav"), el(".js-mainAside"));
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiLi4vLi4vc2hhcmVkL3RhYmxlL3RhYmxlLnRzIiwgInVuaXQudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIwIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG4vKipcbiAqIENvbnRyb2xsZXIgZm9yIGEgdGFibGUgZWxlbWVudCB3aXRoIGV4cGFuZGFibGUgcm93cy4gQWRkcyBldmVudCBsaXN0ZW5lcnMgdG9cbiAqIGEgdG9nZ2xlIHdpdGhpbiBhIHRhYmxlIHJvdyB0aGF0IGNvbnRyb2xzIHZpc2libGl0eSBvZiBhZGRpdGlvbmFsIHJlbGF0ZWRcbiAqIHJvd3MgaW4gdGhlIHRhYmxlLlxuICpcbiAqIEBleGFtcGxlXG4gKiBgYGB0eXBlc2NyaXB0XG4gKiBpbXBvcnQge0V4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyfSBmcm9tICcvc3RhdGljL2pzL3RhYmxlJztcbiAqXG4gKiBjb25zdCBlbCA9IGRvY3VtZW50IC5xdWVyeVNlbGVjdG9yPEhUTUxUYWJsZUVsZW1lbnQ+KCcuanMtbXlUYWJsZUVsZW1lbnQnKVxuICogbmV3IEV4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyKGVsKSk7XG4gKiBgYGBcbiAqL1xuZXhwb3J0IGNsYXNzIEV4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyIHtcbiAgcHJpdmF0ZSByb3dzOiBIVE1MVGFibGVSb3dFbGVtZW50W107XG4gIHByaXZhdGUgdG9nZ2xlczogSFRNTEJ1dHRvbkVsZW1lbnRbXTtcblxuICAvKipcbiAgICogQ3JlYXRlIGEgdGFibGUgY29udHJvbGxlci5cbiAgICogQHBhcmFtIHRhYmxlIC0gVGhlIHRhYmxlIGVsZW1lbnQgdG8gd2hpY2ggdGhlIGNvbnRyb2xsZXIgYmluZHMuXG4gICAqL1xuICBjb25zdHJ1Y3Rvcihwcml2YXRlIHRhYmxlOiBIVE1MVGFibGVFbGVtZW50LCBwcml2YXRlIHRvZ2dsZUFsbD86IEhUTUxCdXR0b25FbGVtZW50IHwgbnVsbCkge1xuICAgIHRoaXMucm93cyA9IEFycmF5LmZyb20odGFibGUucXVlcnlTZWxlY3RvckFsbDxIVE1MVGFibGVSb3dFbGVtZW50PignW2RhdGEtYXJpYS1jb250cm9sc10nKSk7XG4gICAgdGhpcy50b2dnbGVzID0gQXJyYXkuZnJvbSh0aGlzLnRhYmxlLnF1ZXJ5U2VsZWN0b3JBbGwoJ1thcmlhLWV4cGFuZGVkXScpKTtcbiAgICB0aGlzLnNldEF0dHJpYnV0ZXMoKTtcbiAgICB0aGlzLmF0dGFjaEV2ZW50TGlzdGVuZXJzKCk7XG4gICAgdGhpcy51cGRhdGUoKTtcbiAgfVxuXG4gIC8qKlxuICAgKiBzZXRBdHRyaWJ1dGVzIHNldHMgZGF0YS1hcmlhLSogYW5kIGRhdGEtaWQgYXR0cmlidXRlcyB0byByZWd1bGFyXG4gICAqIGh0bWwgYXR0cmlidXRlcyBhcyBhIHdvcmthcm91bmQgZm9yIGxpbWl0YXRpb25zIGZyb20gc2FmZWh0bWwuXG4gICAqL1xuICBwcml2YXRlIHNldEF0dHJpYnV0ZXMoKSB7XG4gICAgZm9yIChjb25zdCBhIG9mIFsnZGF0YS1hcmlhLWNvbnRyb2xzJywgJ2RhdGEtYXJpYS1sYWJlbGxlZGJ5JywgJ2RhdGEtaWQnXSkge1xuICAgICAgdGhpcy50YWJsZS5xdWVyeVNlbGVjdG9yQWxsKGBbJHthfV1gKS5mb3JFYWNoKHQgPT4ge1xuICAgICAgICB0LnNldEF0dHJpYnV0ZShhLnJlcGxhY2UoJ2RhdGEtJywgJycpLCB0LmdldEF0dHJpYnV0ZShhKSA/PyAnJyk7XG4gICAgICAgIHQucmVtb3ZlQXR0cmlidXRlKGEpO1xuICAgICAgfSk7XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBhdHRhY2hFdmVudExpc3RlbmVycygpIHtcbiAgICB0aGlzLnJvd3MuZm9yRWFjaCh0ID0+IHtcbiAgICAgIHQuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgICAgdGhpcy5oYW5kbGVUb2dnbGVDbGljayhlKTtcbiAgICAgIH0pO1xuICAgIH0pO1xuICAgIHRoaXMudG9nZ2xlQWxsPy5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICAgIHRoaXMuZXhwYW5kQWxsSXRlbXMoKTtcbiAgICB9KTtcblxuICAgIGRvY3VtZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2tleWRvd24nLCBlID0+IHtcbiAgICAgIGlmICgoZS5jdHJsS2V5IHx8IGUubWV0YUtleSkgJiYgZS5rZXkgPT09ICdmJykge1xuICAgICAgICB0aGlzLmV4cGFuZEFsbEl0ZW1zKCk7XG4gICAgICB9XG4gICAgfSk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZVRvZ2dsZUNsaWNrKGU6IE1vdXNlRXZlbnQpIHtcbiAgICBsZXQgdGFyZ2V0ID0gZS5jdXJyZW50VGFyZ2V0IGFzIEhUTUxUYWJsZVJvd0VsZW1lbnQgfCBudWxsO1xuICAgIGlmICghdGFyZ2V0Py5oYXNBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnKSkge1xuICAgICAgdGFyZ2V0ID0gdGhpcy50YWJsZS5xdWVyeVNlbGVjdG9yKFxuICAgICAgICBgYnV0dG9uW2FyaWEtY29udHJvbHM9XCIke3RhcmdldD8uZ2V0QXR0cmlidXRlKCdhcmlhLWNvbnRyb2xzJyl9XCJdYFxuICAgICAgKTtcbiAgICB9XG4gICAgY29uc3QgaXNFeHBhbmRlZCA9IHRhcmdldD8uZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICd0cnVlJztcbiAgICB0YXJnZXQ/LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsIGlzRXhwYW5kZWQgPyAnZmFsc2UnIDogJ3RydWUnKTtcbiAgICBlLnN0b3BQcm9wYWdhdGlvbigpO1xuICAgIHRoaXMudXBkYXRlKCk7XG4gIH1cblxuICBwcml2YXRlIGV4cGFuZEFsbEl0ZW1zID0gKCkgPT4ge1xuICAgIHRoaXMudG9nZ2xlcy5tYXAodCA9PiB0LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICd0cnVlJykpO1xuICAgIHRoaXMudXBkYXRlKCk7XG4gIH07XG5cbiAgcHJpdmF0ZSBjb2xsYXBzZUFsbEl0ZW1zID0gKCkgPT4ge1xuICAgIHRoaXMudG9nZ2xlcy5tYXAodCA9PiB0LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICdmYWxzZScpKTtcbiAgICB0aGlzLnVwZGF0ZSgpO1xuICB9O1xuXG4gIHByaXZhdGUgdXBkYXRlID0gKCkgPT4ge1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZUl0ZW1zKCk7XG4gICAgc2V0VGltZW91dCgoKSA9PiB0aGlzLnVwZGF0ZUdsb2JhbFRvZ2dsZSgpKTtcbiAgfTtcblxuICBwcml2YXRlIHVwZGF0ZVZpc2libGVJdGVtcygpIHtcbiAgICB0aGlzLnJvd3MubWFwKHQgPT4ge1xuICAgICAgY29uc3QgaXNFeHBhbmRlZCA9IHQ/LmdldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcpID09PSAndHJ1ZSc7XG4gICAgICBjb25zdCByb3dJZHMgPSB0Py5nZXRBdHRyaWJ1dGUoJ2FyaWEtY29udHJvbHMnKT8udHJpbUVuZCgpLnNwbGl0KCcgJyk7XG4gICAgICByb3dJZHM/Lm1hcChpZCA9PiB7XG4gICAgICAgIGNvbnN0IHRhcmdldCA9IGRvY3VtZW50LmdldEVsZW1lbnRCeUlkKGAke2lkfWApO1xuICAgICAgICBpZiAoaXNFeHBhbmRlZCkge1xuICAgICAgICAgIHRhcmdldD8uY2xhc3NMaXN0LmFkZCgndmlzaWJsZScpO1xuICAgICAgICAgIHRhcmdldD8uY2xhc3NMaXN0LnJlbW92ZSgnaGlkZGVuJyk7XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgdGFyZ2V0Py5jbGFzc0xpc3QuYWRkKCdoaWRkZW4nKTtcbiAgICAgICAgICB0YXJnZXQ/LmNsYXNzTGlzdC5yZW1vdmUoJ3Zpc2libGUnKTtcbiAgICAgICAgfVxuICAgICAgfSk7XG4gICAgfSk7XG4gIH1cblxuICBwcml2YXRlIHVwZGF0ZUdsb2JhbFRvZ2dsZSgpIHtcbiAgICBpZiAoIXRoaXMudG9nZ2xlQWxsKSByZXR1cm47XG4gICAgaWYgKHRoaXMucm93cy5zb21lKHQgPT4gdC5oYXNBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnKSkpIHtcbiAgICAgIHRoaXMudG9nZ2xlQWxsLnN0eWxlLmRpc3BsYXkgPSAnYmxvY2snO1xuICAgIH1cbiAgICBjb25zdCBzb21lQ29sbGFwc2VkID0gdGhpcy50b2dnbGVzLnNvbWUoZWwgPT4gZWwuZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICdmYWxzZScpO1xuICAgIGlmIChzb21lQ29sbGFwc2VkKSB7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5pbm5lclRleHQgPSAnRXhwYW5kIGFsbCc7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5vbmNsaWNrID0gdGhpcy5leHBhbmRBbGxJdGVtcztcbiAgICB9IGVsc2Uge1xuICAgICAgdGhpcy50b2dnbGVBbGwuaW5uZXJUZXh0ID0gJ0NvbGxhcHNlIGFsbCc7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5vbmNsaWNrID0gdGhpcy5jb2xsYXBzZUFsbEl0ZW1zO1xuICAgIH1cbiAgfVxufVxuIiwgIi8qKlxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG5pbXBvcnQgeyBFeHBhbmRhYmxlUm93c1RhYmxlQ29udHJvbGxlciB9IGZyb20gJy4uLy4uL3NoYXJlZC90YWJsZS90YWJsZSc7XG5cbmRvY3VtZW50XG4gIC5xdWVyeVNlbGVjdG9yQWxsPEhUTUxUYWJsZUVsZW1lbnQ+KCcuanMtZXhwYW5kYWJsZVRhYmxlJylcbiAgLmZvckVhY2goXG4gICAgZWwgPT5cbiAgICAgIG5ldyBFeHBhbmRhYmxlUm93c1RhYmxlQ29udHJvbGxlcihcbiAgICAgICAgZWwsXG4gICAgICAgIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEJ1dHRvbkVsZW1lbnQ+KCcuanMtZXhwYW5kQWxsRGlyZWN0b3JpZXMnKVxuICAgICAgKVxuICApO1xuXG4vKipcbiAqIE1haW5MYXlvdXRDb250cm9sbGVyIGNhbGN1bGF0ZXMgZHluYW1pYyBoZWlnaHQgdmFsdWVzIGZvciBoZWFkZXIgZWxlbWVudHNcbiAqIHRvIHN1cHBvcnQgdmFyaWFibGUgc2l6ZSBzdGlja3kgcG9zaXRpb25lZCBlbGVtZW50cyBpbiB0aGUgaGVhZGVyIHNvIHRoYXRcbiAqIGJhbm5lcnMgYW5kIGJyZWFkY3VtYnMgbWF5IG92ZXJmbG93IHRvIG11bHRpcGxlIGxpbmVzLlxuICovXG5cbmNvbnN0IGhlYWRlckhlaWdodCA9IDMuNTtcbmNvbnN0IGJyZWFkY3VtYkhlaWdodCA9IDIuNTtcblxuZXhwb3J0IGNsYXNzIE1haW5MYXlvdXRDb250cm9sbGVyIHtcbiAgcHJpdmF0ZSBoZWFkZXJPYnNlcnZlcjogSW50ZXJzZWN0aW9uT2JzZXJ2ZXI7XG4gIHByaXZhdGUgbmF2T2JzZXJ2ZXI6IEludGVyc2VjdGlvbk9ic2VydmVyO1xuICBwcml2YXRlIGFzaWRlT2JzZXJ2ZXI6IEludGVyc2VjdGlvbk9ic2VydmVyO1xuXG4gIGNvbnN0cnVjdG9yKFxuICAgIHByaXZhdGUgbWFpbkhlYWRlcj86IEVsZW1lbnQgfCBudWxsLFxuICAgIHByaXZhdGUgbWFpbk5hdj86IEVsZW1lbnQgfCBudWxsLFxuICAgIHByaXZhdGUgbWFpbkFzaWRlPzogRWxlbWVudCB8IG51bGxcbiAgKSB7XG4gICAgdGhpcy5oZWFkZXJPYnNlcnZlciA9IG5ldyBJbnRlcnNlY3Rpb25PYnNlcnZlcihcbiAgICAgIChbZV0pID0+IHtcbiAgICAgICAgaWYgKGUuaW50ZXJzZWN0aW9uUmF0aW8gPCAxKSB7XG4gICAgICAgICAgZm9yIChjb25zdCB4IG9mIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwoJ1tjbGFzc149XCJnby1NYWluLWhlYWRlclwiJykpIHtcbiAgICAgICAgICAgIHguc2V0QXR0cmlidXRlKCdkYXRhLWZpeGVkJywgJ3RydWUnKTtcbiAgICAgICAgICB9XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgZm9yIChjb25zdCB4IG9mIGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGwoJ1tjbGFzc149XCJnby1NYWluLWhlYWRlclwiJykpIHtcbiAgICAgICAgICAgIHgucmVtb3ZlQXR0cmlidXRlKCdkYXRhLWZpeGVkJyk7XG4gICAgICAgICAgfVxuICAgICAgICAgIHRoaXMuaGFuZGxlUmVzaXplKCk7XG4gICAgICAgIH1cbiAgICAgIH0sXG4gICAgICB7IHRocmVzaG9sZDogMSwgcm9vdE1hcmdpbjogYCR7YnJlYWRjdW1iSGVpZ2h0ICogMTZ9cHhgIH1cbiAgICApO1xuICAgIHRoaXMubmF2T2JzZXJ2ZXIgPSBuZXcgSW50ZXJzZWN0aW9uT2JzZXJ2ZXIoXG4gICAgICAoW2VdKSA9PiB7XG4gICAgICAgIGlmIChlLmludGVyc2VjdGlvblJhdGlvIDwgMSkge1xuICAgICAgICAgIHRoaXMubWFpbk5hdj8uY2xhc3NMaXN0LmFkZCgnZ28tTWFpbi1uYXYtLWZpeGVkJyk7XG4gICAgICAgICAgdGhpcy5tYWluTmF2Py5zZXRBdHRyaWJ1dGUoJ2RhdGEtZml4ZWQnLCAndHJ1ZScpO1xuICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgIHRoaXMubWFpbk5hdj8uY2xhc3NMaXN0LnJlbW92ZSgnZ28tTWFpbi1uYXYtLWZpeGVkJyk7XG4gICAgICAgICAgdGhpcy5tYWluTmF2Py5yZW1vdmVBdHRyaWJ1dGUoJ2RhdGEtZml4ZWQnKTtcbiAgICAgICAgfVxuICAgICAgfSxcbiAgICAgIHsgdGhyZXNob2xkOiAxLCByb290TWFyZ2luOiBgLSR7aGVhZGVySGVpZ2h0ICogMTYgKyAxMH1weGAgfVxuICAgICk7XG4gICAgdGhpcy5hc2lkZU9ic2VydmVyID0gbmV3IEludGVyc2VjdGlvbk9ic2VydmVyKFxuICAgICAgKFtlXSkgPT4ge1xuICAgICAgICBpZiAoZS5pbnRlcnNlY3Rpb25SYXRpbyA8IDEpIHtcbiAgICAgICAgICB0aGlzLm1haW5IZWFkZXI/LnNldEF0dHJpYnV0ZSgnZGF0YS1yYWlzZWQnLCAndHJ1ZScpO1xuICAgICAgICB9IGVsc2Uge1xuICAgICAgICAgIHRoaXMubWFpbkhlYWRlcj8ucmVtb3ZlQXR0cmlidXRlKCdkYXRhLXJhaXNlZCcpO1xuICAgICAgICB9XG4gICAgICB9LFxuICAgICAgeyB0aHJlc2hvbGQ6IDEsIHJvb3RNYXJnaW46IGAtJHtoZWFkZXJIZWlnaHQgKiAxNiArIDIwfXB4IDBweCAwcHggMHB4YCB9XG4gICAgKTtcbiAgICB0aGlzLmluaXQoKTtcbiAgfVxuXG4gIHByaXZhdGUgaW5pdCgpIHtcbiAgICB0aGlzLmhhbmRsZVJlc2l6ZSgpO1xuICAgIHdpbmRvdy5hZGRFdmVudExpc3RlbmVyKCdyZXNpemUnLCB0aGlzLmhhbmRsZVJlc2l6ZSk7XG4gICAgdGhpcy5tYWluSGVhZGVyPy5hZGRFdmVudExpc3RlbmVyKCdkYmxjbGljaycsIHRoaXMuaGFuZGxlRG91YmxlQ2xpY2spO1xuICAgIGlmICh0aGlzLm1haW5IZWFkZXI/Lmhhc0NoaWxkTm9kZXMoKSkge1xuICAgICAgY29uc3QgaGVhZGVyU2VudGluZWwgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdkaXYnKTtcbiAgICAgIHRoaXMubWFpbkhlYWRlci5wcmVwZW5kKGhlYWRlclNlbnRpbmVsKTtcbiAgICAgIHRoaXMuaGVhZGVyT2JzZXJ2ZXIub2JzZXJ2ZShoZWFkZXJTZW50aW5lbCk7XG4gICAgfVxuICAgIGlmICh0aGlzLm1haW5OYXY/Lmhhc0NoaWxkTm9kZXMoKSkge1xuICAgICAgY29uc3QgbmF2U2VudGluZWwgPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdkaXYnKTtcbiAgICAgIHRoaXMubWFpbk5hdi5wcmVwZW5kKG5hdlNlbnRpbmVsKTtcbiAgICAgIHRoaXMubmF2T2JzZXJ2ZXIub2JzZXJ2ZShuYXZTZW50aW5lbCk7XG4gICAgfVxuICAgIGlmICh0aGlzLm1haW5Bc2lkZSkge1xuICAgICAgY29uc3QgYXNpZGVTZW50aW5lbCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ2RpdicpO1xuICAgICAgdGhpcy5tYWluQXNpZGUucHJlcGVuZChhc2lkZVNlbnRpbmVsKTtcbiAgICAgIHRoaXMuYXNpZGVPYnNlcnZlci5vYnNlcnZlKGFzaWRlU2VudGluZWwpO1xuICAgIH1cbiAgfVxuXG4gIHByaXZhdGUgaGFuZGxlRG91YmxlQ2xpY2s6IEV2ZW50TGlzdGVuZXIgPSBlID0+IHtcbiAgICBjb25zdCB0YXJnZXQgPSBlLnRhcmdldDtcbiAgICBpZiAodGFyZ2V0ID09PSB0aGlzLm1haW5IZWFkZXI/Lmxhc3RFbGVtZW50Q2hpbGQpIHtcbiAgICAgIHdpbmRvdy5nZXRTZWxlY3Rpb24oKT8ucmVtb3ZlQWxsUmFuZ2VzKCk7XG4gICAgICB3aW5kb3cuc2Nyb2xsVG8oeyB0b3A6IDAsIGJlaGF2aW9yOiAnc21vb3RoJyB9KTtcbiAgICB9XG4gIH07XG5cbiAgcHJpdmF0ZSBoYW5kbGVSZXNpemUgPSAoKSA9PiB7XG4gICAgY29uc3Qgc2V0UHJvcCA9IChuYW1lOiBzdHJpbmcsIHZhbHVlOiBzdHJpbmcpID0+XG4gICAgICBkb2N1bWVudC5kb2N1bWVudEVsZW1lbnQuc3R5bGUuc2V0UHJvcGVydHkobmFtZSwgdmFsdWUpO1xuICAgIHNldFByb3AoJy0tanMtdW5pdC1oZWFkZXItaGVpZ2h0JywgJzAnKTtcbiAgICBzZXRUaW1lb3V0KCgpID0+IHtcbiAgICAgIGNvbnN0IG1haW5IZWFkZXJIZWlnaHQgPSAodGhpcy5tYWluSGVhZGVyPy5nZXRCb3VuZGluZ0NsaWVudFJlY3QoKS5oZWlnaHQgPz8gMCkgLyAxNjtcbiAgICAgIHNldFByb3AoJy0tanMtdW5pdC1oZWFkZXItaGVpZ2h0JywgYCR7bWFpbkhlYWRlckhlaWdodH1yZW1gKTtcbiAgICAgIHNldFByb3AoJy0tanMtc3RpY2t5LWhlYWRlci1oZWlnaHQnLCBgJHtoZWFkZXJIZWlnaHR9cmVtYCk7XG4gICAgICBzZXRQcm9wKCctLWpzLXVuaXQtaGVhZGVyLXRvcCcsIGAkeyhtYWluSGVhZGVySGVpZ2h0IC0gaGVhZGVySGVpZ2h0KSAqIC0xfXJlbWApO1xuICAgIH0pO1xuICB9O1xufVxuXG5jb25zdCBlbCA9IDxUIGV4dGVuZHMgSFRNTEVsZW1lbnQ+KHNlbGVjdG9yOiBzdHJpbmcpID0+IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8VD4oc2VsZWN0b3IpO1xubmV3IE1haW5MYXlvdXRDb250cm9sbGVyKGVsKCcuanMtbWFpbkhlYWRlcicpLCBlbCgnLmpzLW1haW5OYXYnKSwgZWwoJy5qcy1tYWluQXNpZGUnKSk7XG4iXSwKICAibWFwcGluZ3MiOiAiOztBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQW9CTyw0Q0FBb0M7QUFBQSxJQVF6QyxZQUFvQixPQUFpQyxXQUFzQztBQUF2RTtBQUFpQztBQW1EN0MsNEJBQWlCLE1BQU07QUFDN0IsYUFBSyxRQUFRLElBQUksT0FBSyxFQUFFLGFBQWEsaUJBQWlCO0FBQ3RELGFBQUs7QUFBQTtBQUdDLDhCQUFtQixNQUFNO0FBQy9CLGFBQUssUUFBUSxJQUFJLE9BQUssRUFBRSxhQUFhLGlCQUFpQjtBQUN0RCxhQUFLO0FBQUE7QUFHQyxvQkFBUyxNQUFNO0FBQ3JCLGFBQUs7QUFDTCxtQkFBVyxNQUFNLEtBQUs7QUFBQTtBQTlEdEIsV0FBSyxPQUFPLE1BQU0sS0FBSyxNQUFNLGlCQUFzQztBQUNuRSxXQUFLLFVBQVUsTUFBTSxLQUFLLEtBQUssTUFBTSxpQkFBaUI7QUFDdEQsV0FBSztBQUNMLFdBQUs7QUFDTCxXQUFLO0FBQUE7QUFBQSxJQU9DLGdCQUFnQjtBQUN0QixpQkFBVyxLQUFLLENBQUMsc0JBQXNCLHdCQUF3QixZQUFZ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
