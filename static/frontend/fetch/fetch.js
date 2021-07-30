(() => {
  // static/frontend/fetch/fetch.ts
  /*!
   * @license
   * Copyright 2020 The Go Authors. All rights reserved.
   * Use of this source code is governed by a BSD-style
   * license that can be found in the LICENSE file.
   */
  var fetchButton = document.querySelector(".js-fetchButton");
  if (fetchButton) {
    fetchButton.addEventListener("click", (e) => {
      e.preventDefault();
      fetchPath();
    });
  }
  async function fetchPath() {
    const fetchMessageEl = document.querySelector(".js-fetchMessage");
    const fetchMessageSecondary = document.querySelector(".js-fetchMessageSecondary");
    const fetchButton2 = document.querySelector(".js-fetchButton");
    const fetchLoading = document.querySelector(".js-fetchLoading");
    if (!(fetchMessageEl && fetchMessageSecondary && fetchButton2 && fetchLoading)) {
      return;
    }
    fetchMessageEl.textContent = `Fetching ${fetchMessageEl.dataset.path}`;
    fetchMessageSecondary.textContent = "Feel free to navigate away and check back later, we\u2019ll keep working on it!";
    fetchButton2.style.display = "none";
    fetchLoading.style.display = "block";
    const response = await fetch(`/fetch${window.location.pathname}`, {method: "POST"});
    if (response.ok) {
      window.location.reload();
      return;
    }
    const responseText = await response.text();
    fetchLoading.style.display = "none";
    fetchMessageSecondary.textContent = "";
    const responseTextParsedDOM = new DOMParser().parseFromString(responseText, "text/html");
    fetchMessageEl.innerHTML = responseTextParsedDOM.documentElement.textContent ?? "";
  }
})();
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiZmV0Y2gudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbIi8qIVxuICogQGxpY2Vuc2VcbiAqIENvcHlyaWdodCAyMDIwIFRoZSBHbyBBdXRob3JzLiBBbGwgcmlnaHRzIHJlc2VydmVkLlxuICogVXNlIG9mIHRoaXMgc291cmNlIGNvZGUgaXMgZ292ZXJuZWQgYnkgYSBCU0Qtc3R5bGVcbiAqIGxpY2Vuc2UgdGhhdCBjYW4gYmUgZm91bmQgaW4gdGhlIExJQ0VOU0UgZmlsZS5cbiAqL1xuXG5jb25zdCBmZXRjaEJ1dHRvbiA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3IoJy5qcy1mZXRjaEJ1dHRvbicpO1xuaWYgKGZldGNoQnV0dG9uKSB7XG4gIGZldGNoQnV0dG9uLmFkZEV2ZW50TGlzdGVuZXIoJ2NsaWNrJywgZSA9PiB7XG4gICAgZS5wcmV2ZW50RGVmYXVsdCgpO1xuICAgIGZldGNoUGF0aCgpO1xuICB9KTtcbn1cblxuYXN5bmMgZnVuY3Rpb24gZmV0Y2hQYXRoKCkge1xuICBjb25zdCBmZXRjaE1lc3NhZ2VFbCA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEhlYWRpbmdFbGVtZW50PignLmpzLWZldGNoTWVzc2FnZScpO1xuICBjb25zdCBmZXRjaE1lc3NhZ2VTZWNvbmRhcnkgPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxQYXJhZ3JhcGhFbGVtZW50PihcbiAgICAnLmpzLWZldGNoTWVzc2FnZVNlY29uZGFyeSdcbiAgKTtcbiAgY29uc3QgZmV0Y2hCdXR0b24gPSBkb2N1bWVudC5xdWVyeVNlbGVjdG9yPEhUTUxCdXR0b25FbGVtZW50PignLmpzLWZldGNoQnV0dG9uJyk7XG4gIGNvbnN0IGZldGNoTG9hZGluZyA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTERpdkVsZW1lbnQ+KCcuanMtZmV0Y2hMb2FkaW5nJyk7XG4gIGlmICghKGZldGNoTWVzc2FnZUVsICYmIGZldGNoTWVzc2FnZVNlY29uZGFyeSAmJiBmZXRjaEJ1dHRvbiAmJiBmZXRjaExvYWRpbmcpKSB7XG4gICAgcmV0dXJuO1xuICB9XG4gIGZldGNoTWVzc2FnZUVsLnRleHRDb250ZW50ID0gYEZldGNoaW5nICR7ZmV0Y2hNZXNzYWdlRWwuZGF0YXNldC5wYXRofWA7XG4gIGZldGNoTWVzc2FnZVNlY29uZGFyeS50ZXh0Q29udGVudCA9XG4gICAgJ0ZlZWwgZnJlZSB0byBuYXZpZ2F0ZSBhd2F5IGFuZCBjaGVjayBiYWNrIGxhdGVyLCB3ZVx1MjAxOWxsIGtlZXAgd29ya2luZyBvbiBpdCEnO1xuICBmZXRjaEJ1dHRvbi5zdHlsZS5kaXNwbGF5ID0gJ25vbmUnO1xuICBmZXRjaExvYWRpbmcuc3R5bGUuZGlzcGxheSA9ICdibG9jayc7XG5cbiAgY29uc3QgcmVzcG9uc2UgPSBhd2FpdCBmZXRjaChgL2ZldGNoJHt3aW5kb3cubG9jYXRpb24ucGF0aG5hbWV9YCwgeyBtZXRob2Q6ICdQT1NUJyB9KTtcbiAgaWYgKHJlc3BvbnNlLm9rKSB7XG4gICAgd2luZG93LmxvY2F0aW9uLnJlbG9hZCgpO1xuICAgIHJldHVybjtcbiAgfVxuICBjb25zdCByZXNwb25zZVRleHQgPSBhd2FpdCByZXNwb25zZS50ZXh0KCk7XG4gIGZldGNoTG9hZGluZy5zdHlsZS5kaXNwbGF5ID0gJ25vbmUnO1xuICBmZXRjaE1lc3NhZ2VTZWNvbmRhcnkudGV4dENvbnRlbnQgPSAnJztcbiAgY29uc3QgcmVzcG9uc2VUZXh0UGFyc2VkRE9NID0gbmV3IERPTVBhcnNlcigpLnBhcnNlRnJvbVN0cmluZyhyZXNwb25zZVRleHQsICd0ZXh0L2h0bWwnKTtcbiAgZmV0Y2hNZXNzYWdlRWwuaW5uZXJIVE1MID0gcmVzcG9uc2VUZXh0UGFyc2VkRE9NLmRvY3VtZW50RWxlbWVudC50ZXh0Q29udGVudCA/PyAnJztcbn1cbiJdLAogICJtYXBwaW5ncyI6ICI7O0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBT0EsTUFBTSxjQUFjLFNBQVMsY0FBYztBQUMzQyxNQUFJLGFBQWE7QUFDZixnQkFBWSxpQkFBaUIsU0FBUyxPQUFLO0FBQ3pDLFFBQUU7QUFDRjtBQUFBO0FBQUE7QUFJSiw2QkFBMkI7QUFDekIsVUFBTSxpQkFBaUIsU0FBUyxjQUFrQztBQUNsRSxVQUFNLHdCQUF3QixTQUFTLGNBQ3JDO0FBRUYsVUFBTSxlQUFjLFNBQVMsY0FBaUM7QUFDOUQsVUFBTSxlQUFlLFNBQVMsY0FBOEI7QUFDNUQsUUFBSSxDQUFFLG1CQUFrQix5QkFBeUIsZ0JBQWUsZUFBZTtBQUM3RTtBQUFBO0FBRUYsbUJBQWUsY0FBYyxZQUFZLGVBQWUsUUFBUTtBQUNoRSwwQkFBc0IsY0FDcEI7QUFDRixpQkFBWSxNQUFNLFVBQVU7QUFDNUIsaUJBQWEsTUFBTSxVQUFVO0FBRTdCLFVBQU0sV0FBVyxNQUFNLE1BQU0sU0FBUyxPQUFPLFNBQVMsWUFBWSxDQUFFLFFBQVE7QUFDNUUsUUFBSSxTQUFTLElBQUk7QUFDZixhQUFPLFNBQVM7QUFDaEI7QUFBQTtBQUVGLFVBQU0sZUFBZSxNQUFNLFNBQVM7QUFDcEMsaUJBQWEsTUFBTSxVQUFVO0FBQzdCLDBCQUFzQixjQUFjO0FBQ3BDLFVBQU0sd0JBQXdCLElBQUksWUFBWSxnQkFBZ0IsY0FBYztBQUM1RSxtQkFBZSxZQUFZLHNCQUFzQixnQkFBZ0IsZUFBZTtBQUFBOyIsCiAgIm5hbWVzIjogW10KfQo=
