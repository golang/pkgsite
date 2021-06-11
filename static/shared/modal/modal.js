/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class ModalController {
  constructor(el) {
    this.el = el;
    if (!window.HTMLDialogElement && !el.showModal) {
      import("../../../third_party/dialog-polyfill/dialog-polyfill.esm.js").then(({default: polyfill}) => {
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
}
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsibW9kYWwudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qKlxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG4vKipcbiAqIE1vZGFsQ29udHJvbGxlciByZWdpc3RlcnMgYSBkaWFsb2cgZWxlbWVudCB3aXRoIHRoZSBwb2x5ZmlsbCBpZlxuICogbmVjZXNzYXJ5IGZvciB0aGUgY3VycmVudCBicm93c2VyLCBhZGQgYWRkcyBldmVudCBsaXN0ZW5lcnMgdG9cbiAqIGNsb3NlIGFuZCBvcGVuIG1vZGFscy5cbiAqL1xuZXhwb3J0IGNsYXNzIE1vZGFsQ29udHJvbGxlciB7XG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEhUTUxEaWFsb2dFbGVtZW50KSB7XG4gICAgLy8gT25seSBsb2FkIHRoZSBkaWFsb2cgcG9seWZpbGwgaWYgbmVjZXNzYXJ5IGZvciB0aGUgZW52aXJvbm1lbnQuXG4gICAgaWYgKCF3aW5kb3cuSFRNTERpYWxvZ0VsZW1lbnQgJiYgIWVsLnNob3dNb2RhbCkge1xuICAgICAgaW1wb3J0KCcuLi8uLi8uLi90aGlyZF9wYXJ0eS9kaWFsb2ctcG9seWZpbGwvZGlhbG9nLXBvbHlmaWxsLmVzbS5qcycpLnRoZW4oXG4gICAgICAgICh7IGRlZmF1bHQ6IHBvbHlmaWxsIH0pID0+IHtcbiAgICAgICAgICBwb2x5ZmlsbC5yZWdpc3RlckRpYWxvZyhlbCk7XG4gICAgICAgIH1cbiAgICAgICk7XG4gICAgfVxuICAgIGNvbnN0IGlkID0gZWwuaWQ7XG4gICAgY29uc3QgYnV0dG9uID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MQnV0dG9uRWxlbWVudD4oYFthcmlhLWNvbnRyb2xzPVwiJHtpZH1cIl1gKTtcbiAgICBpZiAoYnV0dG9uKSB7XG4gICAgICBidXR0b24uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICAgIGlmICh0aGlzLmVsLnNob3dNb2RhbCkge1xuICAgICAgICAgIHRoaXMuZWwuc2hvd01vZGFsKCk7XG4gICAgICAgIH0gZWxzZSB7XG4gICAgICAgICAgdGhpcy5lbC5vcGVuID0gdHJ1ZTtcbiAgICAgICAgfVxuICAgICAgICBlbC5xdWVyeVNlbGVjdG9yKCdpbnB1dCcpPy5mb2N1cygpO1xuICAgICAgfSk7XG4gICAgfVxuICAgIGZvciAoY29uc3QgY2xvc2Ugb2YgdGhpcy5lbC5xdWVyeVNlbGVjdG9yQWxsPEhUTUxCdXR0b25FbGVtZW50PignW2RhdGEtbW9kYWwtY2xvc2VdJykpIHtcbiAgICAgIGNsb3NlLmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgKCkgPT4ge1xuICAgICAgICBpZiAodGhpcy5lbC5jbG9zZSkge1xuICAgICAgICAgIHRoaXMuZWwuY2xvc2UoKTtcbiAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICB0aGlzLmVsLm9wZW4gPSBmYWxzZTtcbiAgICAgICAgfVxuICAgICAgfSk7XG4gICAgfVxuICB9XG59XG4iXSwKICAibWFwcGluZ3MiOiAiQUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFZTyw2QkFBc0I7QUFBQSxFQUMzQixZQUFvQixJQUF1QjtBQUF2QjtBQUVsQixRQUFJLENBQUMsT0FBTyxxQkFBcUIsQ0FBQyxHQUFHLFdBQVc7QUFDOUMsTUFBTyxzRUFBK0QsS0FDcEUsQ0FBQyxDQUFFLFNBQVMsY0FBZTtBQUN6QixpQkFBUyxlQUFlO0FBQUE7QUFBQTtBQUk5QixVQUFNLEtBQUssR0FBRztBQUNkLFVBQU0sU0FBUyxTQUFTLGNBQWlDLG1CQUFtQjtBQUM1RSxRQUFJLFFBQVE7QUFDVixhQUFPLGlCQUFpQixTQUFTLE1BQU07QUFDckMsWUFBSSxLQUFLLEdBQUcsV0FBVztBQUNyQixlQUFLLEdBQUc7QUFBQSxlQUNIO0FBQ0wsZUFBSyxHQUFHLE9BQU87QUFBQTtBQUVqQixXQUFHLGNBQWMsVUFBVTtBQUFBO0FBQUE7QUFHL0IsZUFBVyxTQUFTLEtBQUssR0FBRyxpQkFBb0MsdUJBQXVCO0FBQ3JGLFlBQU0saUJBQWlCLFNBQVMsTUFBTTtBQUNwQyxZQUFJLEtBQUssR0FBRyxPQUFPO0FBQ2pCLGVBQUssR0FBRztBQUFBLGVBQ0g7QUFDTCxlQUFLLEdBQUcsT0FBTztBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7IiwKICAibmFtZXMiOiBbXQp9Cg==
