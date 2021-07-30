(() => {
  // static/frontend/unit/versions/versions.ts
  /*!
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var VersionsController = class {
    constructor() {
      this.expand = document.querySelector(".js-versionsExpand");
      this.collapse = document.querySelector(".js-versionsCollapse");
      this.details = [...document.querySelectorAll(".js-versionDetails")];
      if (!this.expand?.parentElement)
        return;
      if (this.details.some((d) => d.tagName === "DETAILS")) {
        this.expand.parentElement.style.display = "block";
      }
      for (const d of this.details) {
        d.addEventListener("click", () => {
          this.updateButtons();
        });
      }
      this.expand?.addEventListener("click", () => {
        this.details.map((d) => d.open = true);
        this.updateButtons();
      });
      this.collapse?.addEventListener("click", () => {
        this.details.map((d) => d.open = false);
        this.updateButtons();
      });
      this.updateButtons();
      this.setCurrent();
    }
    setCurrent() {
      const canonicalPath = document.querySelector(".js-canonicalURLPath")?.dataset?.canonicalUrlPath;
      const versionLink = document.querySelector(`.js-versionLink[href="${canonicalPath}"]`);
      if (versionLink) {
        versionLink.style.fontWeight = "bold";
      }
    }
    updateButtons() {
      setTimeout(() => {
        if (!this.expand || !this.collapse)
          return;
        let someOpen, someClosed;
        for (const d of this.details) {
          someOpen = someOpen || d.open;
          someClosed = someClosed || !d.open;
        }
        this.expand.style.display = someClosed ? "inline-block" : "none";
        this.collapse.style.display = someClosed ? "none" : "inline-block";
      });
    }
  };
  new VersionsController();
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidmVyc2lvbnMudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIxIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG4vKipcbiAqIFZlcnNpb25zQ29udHJvbGxlciBlbmNhcHN1bGF0ZXMgZXZlbnQgbGlzdGVuZXJzIGFuZCBVSSB1cGRhdGVzXG4gKiBmb3IgdGhlIHZlcnNpb25zIHBhZ2UuIEFzIHRoZSB0aGUgZXhwYW5kYWJsZSBzZWN0aW9ucyBjb250YWluaW5nXG4gKiB0aGUgc3ltYm9sIGhpc3RvcnkgZm9yIGEgcGFja2FnZSBhcmUgb3BlbmVkIGFuZCBjbG9zZWQgaXQgdG9nZ2xlc1xuICogdmlzaWJsaXR5IG9mIHRoZSBidXR0b25zIHRvIGV4cGFuZCBvciBjb2xsYXBzZSB0aGVtLiBPbiBwYWdlIGxvYWRcbiAqIGl0IGFkZHMgYW4gaW5kaWNhdG9yIHRvIHRoZSB2ZXJzaW9uIHRoYXQgbWF0Y2hlcyB0aGUgdmVyc2lvbiByZXF1ZXN0XG4gKiBieSB0aGUgdXNlciBmb3IgdGhlIHBhZ2Ugb3IgdGhlIGNhbm9uaWNhbCB1cmwgcGF0aC5cbiAqL1xuY2xhc3MgVmVyc2lvbnNDb250cm9sbGVyIHtcbiAgcHJpdmF0ZSBleHBhbmQgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxCdXR0b25FbGVtZW50PignLmpzLXZlcnNpb25zRXhwYW5kJyk7XG4gIHByaXZhdGUgY29sbGFwc2UgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxCdXR0b25FbGVtZW50PignLmpzLXZlcnNpb25zQ29sbGFwc2UnKTtcbiAgcHJpdmF0ZSBkZXRhaWxzID0gWy4uLmRvY3VtZW50LnF1ZXJ5U2VsZWN0b3JBbGw8SFRNTERldGFpbHNFbGVtZW50PignLmpzLXZlcnNpb25EZXRhaWxzJyldO1xuXG4gIGNvbnN0cnVjdG9yKCkge1xuICAgIGlmICghdGhpcy5leHBhbmQ/LnBhcmVudEVsZW1lbnQpIHJldHVybjtcbiAgICBpZiAodGhpcy5kZXRhaWxzLnNvbWUoZCA9PiBkLnRhZ05hbWUgPT09ICdERVRBSUxTJykpIHtcbiAgICAgIHRoaXMuZXhwYW5kLnBhcmVudEVsZW1lbnQuc3R5bGUuZGlzcGxheSA9ICdibG9jayc7XG4gICAgfVxuXG4gICAgZm9yIChjb25zdCBkIG9mIHRoaXMuZGV0YWlscykge1xuICAgICAgZC5hZGRFdmVudExpc3RlbmVyKCdjbGljaycsICgpID0+IHtcbiAgICAgICAgdGhpcy51cGRhdGVCdXR0b25zKCk7XG4gICAgICB9KTtcbiAgICB9XG5cbiAgICB0aGlzLmV4cGFuZD8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICB0aGlzLmRldGFpbHMubWFwKGQgPT4gKGQub3BlbiA9IHRydWUpKTtcbiAgICAgIHRoaXMudXBkYXRlQnV0dG9ucygpO1xuICAgIH0pO1xuXG4gICAgdGhpcy5jb2xsYXBzZT8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gICAgICB0aGlzLmRldGFpbHMubWFwKGQgPT4gKGQub3BlbiA9IGZhbHNlKSk7XG4gICAgICB0aGlzLnVwZGF0ZUJ1dHRvbnMoKTtcbiAgICB9KTtcblxuICAgIHRoaXMudXBkYXRlQnV0dG9ucygpO1xuICAgIHRoaXMuc2V0Q3VycmVudCgpO1xuICB9XG5cbiAgLyoqXG4gICAqIHNldEN1cnJlbnQgYXBwbGllcyB0aGUgYWN0aXZlIHN0eWxlIHRvIHRoZSB2ZXJzaW9uIGRvdFxuICAgKiBmb3IgdGhlIHZlcnNpb24gdGhhdCBtYXRjaGVzIHRoZSBjYW5vbmljYWwgVVJMIHBhdGguXG4gICAqL1xuICBwcml2YXRlIHNldEN1cnJlbnQoKSB7XG4gICAgY29uc3QgY2Fub25pY2FsUGF0aCA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEVsZW1lbnQ+KCcuanMtY2Fub25pY2FsVVJMUGF0aCcpPy5kYXRhc2V0XG4gICAgICA/LmNhbm9uaWNhbFVybFBhdGg7XG4gICAgY29uc3QgdmVyc2lvbkxpbmsgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxFbGVtZW50PihcbiAgICAgIGAuanMtdmVyc2lvbkxpbmtbaHJlZj1cIiR7Y2Fub25pY2FsUGF0aH1cIl1gXG4gICAgKTtcbiAgICBpZiAodmVyc2lvbkxpbmspIHtcbiAgICAgIHZlcnNpb25MaW5rLnN0eWxlLmZvbnRXZWlnaHQgPSAnYm9sZCc7XG4gICAgfVxuICB9XG5cbiAgcHJpdmF0ZSB1cGRhdGVCdXR0b25zKCkge1xuICAgIHNldFRpbWVvdXQoKCkgPT4ge1xuICAgICAgaWYgKCF0aGlzLmV4cGFuZCB8fCAhdGhpcy5jb2xsYXBzZSkgcmV0dXJuO1xuICAgICAgbGV0IHNvbWVPcGVuLCBzb21lQ2xvc2VkO1xuICAgICAgZm9yIChjb25zdCBkIG9mIHRoaXMuZGV0YWlscykge1xuICAgICAgICBzb21lT3BlbiA9IHNvbWVPcGVuIHx8IGQub3BlbjtcbiAgICAgICAgc29tZUNsb3NlZCA9IHNvbWVDbG9zZWQgfHwgIWQub3BlbjtcbiAgICAgIH1cbiAgICAgIHRoaXMuZXhwYW5kLnN0eWxlLmRpc3BsYXkgPSBzb21lQ2xvc2VkID8gJ2lubGluZS1ibG9jaycgOiAnbm9uZSc7XG4gICAgICB0aGlzLmNvbGxhcHNlLnN0eWxlLmRpc3BsYXkgPSBzb21lQ2xvc2VkID8gJ25vbmUnIDogJ2lubGluZS1ibG9jayc7XG4gICAgfSk7XG4gIH1cbn1cblxubmV3IFZlcnNpb25zQ29udHJvbGxlcigpO1xuIl0sCiAgIm1hcHBpbmdzIjogIjs7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFlQSxpQ0FBeUI7QUFBQSxJQUt2QixjQUFjO0FBSk4sb0JBQVMsU0FBUyxjQUFpQztBQUNuRCxzQkFBVyxTQUFTLGNBQWlDO0FBQ3JELHFCQUFVLENBQUMsR0FBRyxTQUFTLGlCQUFxQztBQUdsRSxVQUFJLENBQUMsS0FBSyxRQUFRO0FBQWU7QUFDakMsVUFBSSxLQUFLLFFBQVEsS0FBSyxPQUFLLEVBQUUsWUFBWSxZQUFZO0FBQ25ELGFBQUssT0FBTyxjQUFjLE1BQU0sVUFBVTtBQUFBO0FBRzVDLGlCQUFXLEtBQUssS0FBSyxTQUFTO0FBQzVCLFVBQUUsaUJBQWlCLFNBQVMsTUFBTTtBQUNoQyxlQUFLO0FBQUE7QUFBQTtBQUlULFdBQUssUUFBUSxpQkFBaUIsU0FBUyxNQUFNO0FBQzNDLGFBQUssUUFBUSxJQUFJLE9BQU0sRUFBRSxPQUFPO0FBQ2hDLGFBQUs7QUFBQTtBQUdQLFdBQUssVUFBVSxpQkFBaUIsU0FBUyxNQUFNO0FBQzdDLGFBQUssUUFBUSxJQUFJLE9BQU0sRUFBRSxPQUFPO0FBQ2hDLGFBQUs7QUFBQTtBQUdQLFdBQUs7QUFDTCxXQUFLO0FBQUE7QUFBQSxJQU9DLGFBQWE7QUFDbkIsWUFBTSxnQkFBZ0IsU0FBUyxjQUEyQix5QkFBeUIsU0FDL0U7QUFDSixZQUFNLGNBQWMsU0FBUyxjQUMzQix5QkFBeUI7QUFFM0IsVUFBSSxhQUFhO0FBQ2Ysb0JBQVksTUFBTSxhQUFhO0FBQUE7QUFBQTtBQUFBLElBSTNCLGdCQUFnQjtBQUN0QixpQkFBVyxNQUFNO0FBQ2YsWUFBSSxDQUFDLEtBQUssVUFBVSxDQUFDLEtBQUs7QUFBVTtBQUNwQyxZQUFJLFVBQVU7QUFDZCxtQkFBVyxLQUFLLEtBQUssU0FBUztBQUM1QixxQkFBVyxZQUFZLEVBQUU7QUFDekIsdUJBQWEsY0FBYyxDQUFDLEVBQUU7QUFBQTtBQUVoQyxhQUFLLE9BQU8sTUFBTSxVQUFVLGFBQWEsaUJBQWlCO0FBQzFELGFBQUssU0FBUyxNQUFNLFVBQVUsYUFBYSxTQUFTO0FBQUE7QUFBQTtBQUFBO0FBSzFELE1BQUk7IiwKICAibmFtZXMiOiBbXQp9Cg==
