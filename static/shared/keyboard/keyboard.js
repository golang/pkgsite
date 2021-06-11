/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
const searchInput = document.querySelector(".js-searchFocus");
const canonicalURLPath = document.querySelector(".js-canonicalURLPath")?.dataset["canonicalUrlPath"];
document.addEventListener("keydown", (e) => {
  const t = e.target?.tagName;
  if (t === "INPUT" || t === "SELECT" || t === "TEXTAREA") {
    return;
  }
  if (e.target?.isContentEditable) {
    return;
  }
  if (e.metaKey || e.ctrlKey) {
    return;
  }
  switch (e.key) {
    case "y":
      if (canonicalURLPath && canonicalURLPath !== "") {
        window.history.replaceState(null, "", canonicalURLPath);
      }
      break;
    case "/":
      if (searchInput && !window.navigator.userAgent.includes("Firefox")) {
        e.preventDefault();
        searchInput.focus();
      }
      break;
  }
});
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsia2V5Ym9hcmQudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDE5LTIwMjAgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8vIEtleWJvYXJkIHNob3J0Y3V0czpcbi8vIC0gUHJlc3NpbmcgJy8nIGZvY3VzZXMgdGhlIHNlYXJjaCBib3hcbi8vIC0gUHJlc3NpbmcgJ3knIGNoYW5nZXMgdGhlIGJyb3dzZXIgVVJMIHRvIHRoZSBjYW5vbmljYWwgVVJMXG4vLyB3aXRob3V0IHRyaWdnZXJpbmcgYSByZWxvYWQuXG5cbmNvbnN0IHNlYXJjaElucHV0ID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MSW5wdXRFbGVtZW50PignLmpzLXNlYXJjaEZvY3VzJyk7XG5jb25zdCBjYW5vbmljYWxVUkxQYXRoID0gZG9jdW1lbnQucXVlcnlTZWxlY3RvcjxIVE1MRGl2RWxlbWVudD4oJy5qcy1jYW5vbmljYWxVUkxQYXRoJyk/LmRhdGFzZXRbXG4gICdjYW5vbmljYWxVcmxQYXRoJ1xuXTtcblxuZG9jdW1lbnQuYWRkRXZlbnRMaXN0ZW5lcigna2V5ZG93bicsIGUgPT4ge1xuICAvLyBUT0RPKGdvbGFuZy5vcmcvaXNzdWUvNDAyNDYpOiBjb25zb2xpZGF0ZSBrZXlib2FyZCBzaG9ydGN1dCBiZWhhdmlvciBhY3Jvc3MgdGhlIHNpdGUuXG4gIGNvbnN0IHQgPSAoZS50YXJnZXQgYXMgSFRNTEVsZW1lbnQpPy50YWdOYW1lO1xuICBpZiAodCA9PT0gJ0lOUFVUJyB8fCB0ID09PSAnU0VMRUNUJyB8fCB0ID09PSAnVEVYVEFSRUEnKSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGlmICgoZS50YXJnZXQgYXMgSFRNTEVsZW1lbnQpPy5pc0NvbnRlbnRFZGl0YWJsZSkge1xuICAgIHJldHVybjtcbiAgfVxuICBpZiAoZS5tZXRhS2V5IHx8IGUuY3RybEtleSkge1xuICAgIHJldHVybjtcbiAgfVxuICBzd2l0Y2ggKGUua2V5KSB7XG4gICAgY2FzZSAneSc6XG4gICAgICBpZiAoY2Fub25pY2FsVVJMUGF0aCAmJiBjYW5vbmljYWxVUkxQYXRoICE9PSAnJykge1xuICAgICAgICB3aW5kb3cuaGlzdG9yeS5yZXBsYWNlU3RhdGUobnVsbCwgJycsIGNhbm9uaWNhbFVSTFBhdGgpO1xuICAgICAgfVxuICAgICAgYnJlYWs7XG4gICAgY2FzZSAnLyc6XG4gICAgICAvLyBGYXZvcmluZyB0aGUgRmlyZWZveCBxdWljayBmaW5kIGZlYXR1cmUgb3ZlciBzZWFyY2ggaW5wdXRcbiAgICAgIC8vIGZvY3VzLiBTZWU6IGh0dHBzOi8vZ2l0aHViLmNvbS9nb2xhbmcvZ28vaXNzdWVzLzQxMDkzLlxuICAgICAgaWYgKHNlYXJjaElucHV0ICYmICF3aW5kb3cubmF2aWdhdG9yLnVzZXJBZ2VudC5pbmNsdWRlcygnRmlyZWZveCcpKSB7XG4gICAgICAgIGUucHJldmVudERlZmF1bHQoKTtcbiAgICAgICAgc2VhcmNoSW5wdXQuZm9jdXMoKTtcbiAgICAgIH1cbiAgICAgIGJyZWFrO1xuICB9XG59KTtcbiJdLAogICJtYXBwaW5ncyI6ICJBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQVlBLE1BQU0sY0FBYyxTQUFTLGNBQWdDO0FBQzdELE1BQU0sbUJBQW1CLFNBQVMsY0FBOEIseUJBQXlCLFFBQ3ZGO0FBR0YsU0FBUyxpQkFBaUIsV0FBVyxPQUFLO0FBRXhDLFFBQU0sSUFBSyxFQUFFLFFBQXdCO0FBQ3JDLE1BQUksTUFBTSxXQUFXLE1BQU0sWUFBWSxNQUFNLFlBQVk7QUFDdkQ7QUFBQTtBQUVGLE1BQUssRUFBRSxRQUF3QixtQkFBbUI7QUFDaEQ7QUFBQTtBQUVGLE1BQUksRUFBRSxXQUFXLEVBQUUsU0FBUztBQUMxQjtBQUFBO0FBRUYsVUFBUSxFQUFFO0FBQUEsU0FDSDtBQUNILFVBQUksb0JBQW9CLHFCQUFxQixJQUFJO0FBQy9DLGVBQU8sUUFBUSxhQUFhLE1BQU0sSUFBSTtBQUFBO0FBRXhDO0FBQUEsU0FDRztBQUdILFVBQUksZUFBZSxDQUFDLE9BQU8sVUFBVSxVQUFVLFNBQVMsWUFBWTtBQUNsRSxVQUFFO0FBQ0Ysb0JBQVk7QUFBQTtBQUVkO0FBQUE7QUFBQTsiLAogICJuYW1lcyI6IFtdCn0K
