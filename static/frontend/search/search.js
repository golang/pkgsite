(() => {
  // static/frontend/search/search.ts
  /**
   * @license
   * Copyright 2021 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var hiddenChips = document.querySelectorAll(".js-sameModChip[data-hidden]");
  var showMore = document.querySelector(".js-showMoreChip");
  var searchLimit = document.querySelector(".js-searchLimit");
  showMore?.addEventListener("click", () => {
    for (const el of hiddenChips) {
      el.removeAttribute("data-hidden");
    }
    showMore.parentElement?.removeChild(showMore);
  });
  searchLimit?.addEventListener("change", (e) => {
    e.target.parentNode.submit();
  });
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsic2VhcmNoLnRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuY29uc3QgaGlkZGVuQ2hpcHMgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yQWxsKCcuanMtc2FtZU1vZENoaXBbZGF0YS1oaWRkZW5dJyk7XG5jb25zdCBzaG93TW9yZSA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEJ1dHRvbkVsZW1lbnQ+KCcuanMtc2hvd01vcmVDaGlwJyk7XG5jb25zdCBzZWFyY2hMaW1pdCA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3IoJy5qcy1zZWFyY2hMaW1pdCcpO1xuXG5zaG93TW9yZT8uYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCAoKSA9PiB7XG4gIGZvciAoY29uc3QgZWwgb2YgaGlkZGVuQ2hpcHMpIHtcbiAgICBlbC5yZW1vdmVBdHRyaWJ1dGUoJ2RhdGEtaGlkZGVuJyk7XG4gIH1cbiAgc2hvd01vcmUucGFyZW50RWxlbWVudD8ucmVtb3ZlQ2hpbGQoc2hvd01vcmUpO1xufSk7XG5cbnNlYXJjaExpbWl0Py5hZGRFdmVudExpc3RlbmVyKCdjaGFuZ2UnLCAoZSkgPT4ge1xuICBlLnRhcmdldC5wYXJlbnROb2RlLnN1Ym1pdCgpO1xufSk7XG4iXSwKICAibWFwcGluZ3MiOiAiOztBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQU9BLE1BQU0sY0FBYyxTQUFTLGlCQUFpQjtBQUM5QyxNQUFNLFdBQVcsU0FBUyxjQUFpQztBQUMzRCxNQUFNLGNBQWMsU0FBUyxjQUFjO0FBRTNDLFlBQVUsaUJBQWlCLFNBQVMsTUFBTTtBQUN4QyxlQUFXLE1BQU0sYUFBYTtBQUM1QixTQUFHLGdCQUFnQjtBQUFBO0FBRXJCLGFBQVMsZUFBZSxZQUFZO0FBQUE7QUFHdEMsZUFBYSxpQkFBaUIsVUFBVSxDQUFDLE1BQU07QUFDN0MsTUFBRSxPQUFPLFdBQVc7QUFBQTsiLAogICJuYW1lcyI6IFtdCn0K
