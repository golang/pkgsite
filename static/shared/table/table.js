/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class ExpandableRowsTableController {
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
    const someCollapsed = this.toggles.some((el) => el.getAttribute("aria-expanded") === "false");
    if (someCollapsed) {
      this.toggleAll.innerText = "Expand all";
      this.toggleAll.onclick = this.expandAllItems;
    } else {
      this.toggleAll.innerText = "Collapse all";
      this.toggleAll.onclick = this.collapseAllItems;
    }
  }
}
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidGFibGUudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIwIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG4vKipcbiAqIENvbnRyb2xsZXIgZm9yIGEgdGFibGUgZWxlbWVudCB3aXRoIGV4cGFuZGFibGUgcm93cy4gQWRkcyBldmVudCBsaXN0ZW5lcnMgdG9cbiAqIGEgdG9nZ2xlIHdpdGhpbiBhIHRhYmxlIHJvdyB0aGF0IGNvbnRyb2xzIHZpc2libGl0eSBvZiBhZGRpdGlvbmFsIHJlbGF0ZWRcbiAqIHJvd3MgaW4gdGhlIHRhYmxlLlxuICpcbiAqIEBleGFtcGxlXG4gKiBgYGB0eXBlc2NyaXB0XG4gKiBpbXBvcnQge0V4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyfSBmcm9tICcvc3RhdGljL2pzL3RhYmxlJztcbiAqXG4gKiBjb25zdCBlbCA9IGRvY3VtZW50IC5xdWVyeVNlbGVjdG9yPEhUTUxUYWJsZUVsZW1lbnQ+KCcuanMtbXlUYWJsZUVsZW1lbnQnKVxuICogbmV3IEV4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyKGVsKSk7XG4gKiBgYGBcbiAqL1xuZXhwb3J0IGNsYXNzIEV4cGFuZGFibGVSb3dzVGFibGVDb250cm9sbGVyIHtcbiAgcHJpdmF0ZSByb3dzOiBIVE1MVGFibGVSb3dFbGVtZW50W107XG4gIHByaXZhdGUgdG9nZ2xlczogSFRNTEJ1dHRvbkVsZW1lbnRbXTtcblxuICAvKipcbiAgICogQ3JlYXRlIGEgdGFibGUgY29udHJvbGxlci5cbiAgICogQHBhcmFtIHRhYmxlIC0gVGhlIHRhYmxlIGVsZW1lbnQgdG8gd2hpY2ggdGhlIGNvbnRyb2xsZXIgYmluZHMuXG4gICAqL1xuICBjb25zdHJ1Y3Rvcihwcml2YXRlIHRhYmxlOiBIVE1MVGFibGVFbGVtZW50LCBwcml2YXRlIHRvZ2dsZUFsbD86IEhUTUxCdXR0b25FbGVtZW50IHwgbnVsbCkge1xuICAgIHRoaXMucm93cyA9IEFycmF5LmZyb20odGFibGUucXVlcnlTZWxlY3RvckFsbDxIVE1MVGFibGVSb3dFbGVtZW50PignW2RhdGEtYXJpYS1jb250cm9sc10nKSk7XG4gICAgdGhpcy50b2dnbGVzID0gQXJyYXkuZnJvbSh0aGlzLnRhYmxlLnF1ZXJ5U2VsZWN0b3JBbGwoJ1thcmlhLWV4cGFuZGVkXScpKTtcbiAgICB0aGlzLnNldEF0dHJpYnV0ZXMoKTtcbiAgICB0aGlzLmF0dGFjaEV2ZW50TGlzdGVuZXJzKCk7XG4gICAgdGhpcy51cGRhdGUoKTtcbiAgfVxuXG4gIC8qKlxuICAgKiBzZXRBdHRyaWJ1dGVzIHNldHMgZGF0YS1hcmlhLSogYW5kIGRhdGEtaWQgYXR0cmlidXRlcyB0byByZWd1bGFyXG4gICAqIGh0bWwgYXR0cmlidXRlcyBhcyBhIHdvcmthcm91bmQgZm9yIGxpbWl0YXRpb25zIGZyb20gc2FmZWh0bWwuXG4gICAqL1xuICBwcml2YXRlIHNldEF0dHJpYnV0ZXMoKSB7XG4gICAgZm9yIChjb25zdCBhIG9mIFsnZGF0YS1hcmlhLWNvbnRyb2xzJywgJ2RhdGEtYXJpYS1sYWJlbGxlZGJ5JywgJ2RhdGEtaWQnXSkge1xuICAgICAgdGhpcy50YWJsZS5xdWVyeVNlbGVjdG9yQWxsKGBbJHthfV1gKS5mb3JFYWNoKHQgPT4ge1xuICAgICAgICB0LnNldEF0dHJpYnV0ZShhLnJlcGxhY2UoJ2RhdGEtJywgJycpLCB0LmdldEF0dHJpYnV0ZShhKSA/PyAnJyk7XG4gICAgICAgIHQucmVtb3ZlQXR0cmlidXRlKGEpO1xuICAgICAgfSk7XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSBhdHRhY2hFdmVudExpc3RlbmVycygpIHtcbiAgICB0aGlzLnJvd3MuZm9yRWFjaCh0ID0+IHtcbiAgICAgIHQuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgICAgdGhpcy5oYW5kbGVUb2dnbGVDbGljayhlKTtcbiAgICAgIH0pO1xuICAgIH0pO1xuICAgIHRoaXMudG9nZ2xlQWxsPy5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICAgIHRoaXMuZXhwYW5kQWxsSXRlbXMoKTtcbiAgICB9KTtcblxuICAgIGRvY3VtZW50LmFkZEV2ZW50TGlzdGVuZXIoJ2tleWRvd24nLCBlID0+IHtcbiAgICAgIGlmICgoZS5jdHJsS2V5IHx8IGUubWV0YUtleSkgJiYgZS5rZXkgPT09ICdmJykge1xuICAgICAgICB0aGlzLmV4cGFuZEFsbEl0ZW1zKCk7XG4gICAgICB9XG4gICAgfSk7XG4gIH1cblxuICBwcml2YXRlIGhhbmRsZVRvZ2dsZUNsaWNrKGU6IE1vdXNlRXZlbnQpIHtcbiAgICBsZXQgdGFyZ2V0ID0gZS5jdXJyZW50VGFyZ2V0IGFzIEhUTUxUYWJsZVJvd0VsZW1lbnQgfCBudWxsO1xuICAgIGlmICghdGFyZ2V0Py5oYXNBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnKSkge1xuICAgICAgdGFyZ2V0ID0gdGhpcy50YWJsZS5xdWVyeVNlbGVjdG9yKFxuICAgICAgICBgYnV0dG9uW2FyaWEtY29udHJvbHM9XCIke3RhcmdldD8uZ2V0QXR0cmlidXRlKCdhcmlhLWNvbnRyb2xzJyl9XCJdYFxuICAgICAgKTtcbiAgICB9XG4gICAgY29uc3QgaXNFeHBhbmRlZCA9IHRhcmdldD8uZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICd0cnVlJztcbiAgICB0YXJnZXQ/LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsIGlzRXhwYW5kZWQgPyAnZmFsc2UnIDogJ3RydWUnKTtcbiAgICBlLnN0b3BQcm9wYWdhdGlvbigpO1xuICAgIHRoaXMudXBkYXRlKCk7XG4gIH1cblxuICBwcml2YXRlIGV4cGFuZEFsbEl0ZW1zID0gKCkgPT4ge1xuICAgIHRoaXMudG9nZ2xlcy5tYXAodCA9PiB0LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICd0cnVlJykpO1xuICAgIHRoaXMudXBkYXRlKCk7XG4gIH07XG5cbiAgcHJpdmF0ZSBjb2xsYXBzZUFsbEl0ZW1zID0gKCkgPT4ge1xuICAgIHRoaXMudG9nZ2xlcy5tYXAodCA9PiB0LnNldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcsICdmYWxzZScpKTtcbiAgICB0aGlzLnVwZGF0ZSgpO1xuICB9O1xuXG4gIHByaXZhdGUgdXBkYXRlID0gKCkgPT4ge1xuICAgIHRoaXMudXBkYXRlVmlzaWJsZUl0ZW1zKCk7XG4gICAgc2V0VGltZW91dCgoKSA9PiB0aGlzLnVwZGF0ZUdsb2JhbFRvZ2dsZSgpKTtcbiAgfTtcblxuICBwcml2YXRlIHVwZGF0ZVZpc2libGVJdGVtcygpIHtcbiAgICB0aGlzLnJvd3MubWFwKHQgPT4ge1xuICAgICAgY29uc3QgaXNFeHBhbmRlZCA9IHQ/LmdldEF0dHJpYnV0ZSgnYXJpYS1leHBhbmRlZCcpID09PSAndHJ1ZSc7XG4gICAgICBjb25zdCByb3dJZHMgPSB0Py5nZXRBdHRyaWJ1dGUoJ2FyaWEtY29udHJvbHMnKT8udHJpbUVuZCgpLnNwbGl0KCcgJyk7XG4gICAgICByb3dJZHM/Lm1hcChpZCA9PiB7XG4gICAgICAgIGNvbnN0IHRhcmdldCA9IGRvY3VtZW50LmdldEVsZW1lbnRCeUlkKGAke2lkfWApO1xuICAgICAgICBpZiAoaXNFeHBhbmRlZCkge1xuICAgICAgICAgIHRhcmdldD8uY2xhc3NMaXN0LmFkZCgndmlzaWJsZScpO1xuICAgICAgICAgIHRhcmdldD8uY2xhc3NMaXN0LnJlbW92ZSgnaGlkZGVuJyk7XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgdGFyZ2V0Py5jbGFzc0xpc3QuYWRkKCdoaWRkZW4nKTtcbiAgICAgICAgICB0YXJnZXQ/LmNsYXNzTGlzdC5yZW1vdmUoJ3Zpc2libGUnKTtcbiAgICAgICAgfVxuICAgICAgfSk7XG4gICAgfSk7XG4gIH1cblxuICBwcml2YXRlIHVwZGF0ZUdsb2JhbFRvZ2dsZSgpIHtcbiAgICBpZiAoIXRoaXMudG9nZ2xlQWxsKSByZXR1cm47XG4gICAgaWYgKHRoaXMucm93cy5zb21lKHQgPT4gdC5oYXNBdHRyaWJ1dGUoJ2FyaWEtZXhwYW5kZWQnKSkpIHtcbiAgICAgIHRoaXMudG9nZ2xlQWxsLnN0eWxlLmRpc3BsYXkgPSAnYmxvY2snO1xuICAgIH1cbiAgICBjb25zdCBzb21lQ29sbGFwc2VkID0gdGhpcy50b2dnbGVzLnNvbWUoZWwgPT4gZWwuZ2V0QXR0cmlidXRlKCdhcmlhLWV4cGFuZGVkJykgPT09ICdmYWxzZScpO1xuICAgIGlmIChzb21lQ29sbGFwc2VkKSB7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5pbm5lclRleHQgPSAnRXhwYW5kIGFsbCc7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5vbmNsaWNrID0gdGhpcy5leHBhbmRBbGxJdGVtcztcbiAgICB9IGVsc2Uge1xuICAgICAgdGhpcy50b2dnbGVBbGwuaW5uZXJUZXh0ID0gJ0NvbGxhcHNlIGFsbCc7XG4gICAgICB0aGlzLnRvZ2dsZUFsbC5vbmNsaWNrID0gdGhpcy5jb2xsYXBzZUFsbEl0ZW1zO1xuICAgIH1cbiAgfVxufVxuIl0sCiAgIm1hcHBpbmdzIjogIkFBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBb0JPLDJDQUFvQztBQUFBLEVBUXpDLFlBQW9CLE9BQWlDLFdBQXNDO0FBQXZFO0FBQWlDO0FBbUQ3QywwQkFBaUIsTUFBTTtBQUM3QixXQUFLLFFBQVEsSUFBSSxPQUFLLEVBQUUsYUFBYSxpQkFBaUI7QUFDdEQsV0FBSztBQUFBO0FBR0MsNEJBQW1CLE1BQU07QUFDL0IsV0FBSyxRQUFRLElBQUksT0FBSyxFQUFFLGFBQWEsaUJBQWlCO0FBQ3RELFdBQUs7QUFBQTtBQUdDLGtCQUFTLE1BQU07QUFDckIsV0FBSztBQUNMLGlCQUFXLE1BQU0sS0FBSztBQUFBO0FBOUR0QixTQUFLLE9BQU8sTUFBTSxLQUFLLE1BQU0saUJBQXNDO0FBQ25FLFNBQUssVUFBVSxNQUFNLEtBQUssS0FBSyxNQUFNLGlCQUFpQjtBQUN0RCxTQUFLO0FBQ0wsU0FBSztBQUNMLFNBQUs7QUFBQTtBQUFBLEVBT0MsZ0JBQWdCO0FBQ3RCLGVBQVcsS0FBSyxDQUFDLHNCQUFzQix3QkFBd0IsWUFBWTtBQUN6RSxXQUFLLE1BQU0saUJBQWlCLElBQUksTUFBTSxRQUFRLE9BQUs7QUFDakQsVUFBRSxhQUFhLEVBQUUsUUFBUSxTQUFTLEtBQUssRUFBRSxhQUFhLE1BQU07QUFDNUQsVUFBRSxnQkFBZ0I7QUFBQTtBQUFBO0FBQUE7QUFBQSxFQUtoQix1QkFBdUI7QUFDN0IsU0FBSyxLQUFLLFFBQVEsT0FBSztBQUNyQixRQUFFLGlCQUFpQixTQUFTLE9BQUs7QUFDL0IsYUFBSyxrQkFBa0I7QUFBQTtBQUFBO0FBRzNCLFNBQUssV0FBVyxpQkFBaUIsU0FBUyxNQUFNO0FBQzlDLFdBQUs7QUFBQTtBQUdQLGFBQVMsaUJBQWlCLFdBQVcsT0FBSztBQUN4QyxVQUFLLEdBQUUsV0FBVyxFQUFFLFlBQVksRUFBRSxRQUFRLEtBQUs7QUFDN0MsYUFBSztBQUFBO0FBQUE7QUFBQTtBQUFBLEVBS0gsa0JBQWtCLEdBQWU7QUFDdkMsUUFBSSxTQUFTLEVBQUU7QUFDZixRQUFJLENBQUMsUUFBUSxhQUFhLGtCQUFrQjtBQUMxQyxlQUFTLEtBQUssTUFBTSxjQUNsQix5QkFBeUIsUUFBUSxhQUFhO0FBQUE7QUFHbEQsVUFBTSxhQUFhLFFBQVEsYUFBYSxxQkFBcUI7QUFDN0QsWUFBUSxhQUFhLGlCQUFpQixhQUFhLFVBQVU7QUFDN0QsTUFBRTtBQUNGLFNBQUs7QUFBQTtBQUFBLEVBa0JDLHFCQUFxQjtBQUMzQixTQUFLLEtBQUssSUFBSSxPQUFLO0FBQ2pCLFlBQU0sYUFBYSxHQUFHLGFBQWEscUJBQXFCO0FBQ3hELFlBQU0sU0FBUyxHQUFHLGFBQWEsa0JBQWtCLFVBQVUsTUFBTTtBQUNqRSxjQUFRLElBQUksUUFBTTtBQUNoQixjQUFNLFNBQVMsU0FBUyxlQUFlLEdBQUc7QUFDMUMsWUFBSSxZQUFZO0FBQ2Qsa0JBQVEsVUFBVSxJQUFJO0FBQ3RCLGtCQUFRLFVBQVUsT0FBTztBQUFBLGVBQ3BCO0FBQ0wsa0JBQVEsVUFBVSxJQUFJO0FBQ3RCLGtCQUFRLFVBQVUsT0FBTztBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUEsRUFNekIscUJBQXFCO0FBQzNCLFFBQUksQ0FBQyxLQUFLO0FBQVc7QUFDckIsUUFBSSxLQUFLLEtBQUssS0FBSyxPQUFLLEVBQUUsYUFBYSxtQkFBbUI7QUFDeEQsV0FBSyxVQUFVLE1BQU0sVUFBVTtBQUFBO0FBRWpDLFVBQU0sZ0JBQWdCLEtBQUssUUFBUSxLQUFLLFFBQU0sR0FBRyxhQUFhLHFCQUFxQjtBQUNuRixRQUFJLGVBQWU7QUFDakIsV0FBSyxVQUFVLFlBQVk7QUFDM0IsV0FBSyxVQUFVLFVBQVUsS0FBSztBQUFBLFdBQ3pCO0FBQ0wsV0FBSyxVQUFVLFlBQVk7QUFDM0IsV0FBSyxVQUFVLFVBQVUsS0FBSztBQUFBO0FBQUE7QUFBQTsiLAogICJuYW1lcyI6IFtdCn0K
