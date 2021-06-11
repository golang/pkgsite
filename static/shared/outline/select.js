/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class SelectNavController {
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
}
export function makeSelectNav(tree) {
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
    const value = select.querySelector(`[label="${t.label}"]`)?.value;
    if (value) {
      select.value = value;
    }
  }, 50);
  return label;
}
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsic2VsZWN0LnRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyIvKipcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuaW1wb3J0IHsgVHJlZU5hdkNvbnRyb2xsZXIgfSBmcm9tICcuL3RyZWUuanMnO1xuXG5leHBvcnQgY2xhc3MgU2VsZWN0TmF2Q29udHJvbGxlciB7XG4gIGNvbnN0cnVjdG9yKHByaXZhdGUgZWw6IEVsZW1lbnQpIHtcbiAgICB0aGlzLmVsLmFkZEV2ZW50TGlzdGVuZXIoJ2NoYW5nZScsIGUgPT4ge1xuICAgICAgY29uc3QgdGFyZ2V0ID0gZS50YXJnZXQgYXMgSFRNTFNlbGVjdEVsZW1lbnQ7XG4gICAgICBsZXQgaHJlZiA9IHRhcmdldC52YWx1ZTtcbiAgICAgIGlmICghdGFyZ2V0LnZhbHVlLnN0YXJ0c1dpdGgoJy8nKSkge1xuICAgICAgICBocmVmID0gJy8nICsgaHJlZjtcbiAgICAgIH1cbiAgICAgIHdpbmRvdy5sb2NhdGlvbi5ocmVmID0gaHJlZjtcbiAgICB9KTtcbiAgfVxufVxuXG5leHBvcnQgZnVuY3Rpb24gbWFrZVNlbGVjdE5hdih0cmVlOiBUcmVlTmF2Q29udHJvbGxlcik6IEhUTUxMYWJlbEVsZW1lbnQge1xuICBjb25zdCBsYWJlbCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ2xhYmVsJyk7XG4gIGxhYmVsLmNsYXNzTGlzdC5hZGQoJ2dvLUxhYmVsJyk7XG4gIGxhYmVsLnNldEF0dHJpYnV0ZSgnYXJpYS1sYWJlbCcsICdNZW51Jyk7XG4gIGNvbnN0IHNlbGVjdCA9IGRvY3VtZW50LmNyZWF0ZUVsZW1lbnQoJ3NlbGVjdCcpO1xuICBzZWxlY3QuY2xhc3NMaXN0LmFkZCgnZ28tU2VsZWN0JywgJ2pzLXNlbGVjdE5hdicpO1xuICBsYWJlbC5hcHBlbmRDaGlsZChzZWxlY3QpO1xuICBjb25zdCBvdXRsaW5lID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnb3B0Z3JvdXAnKTtcbiAgb3V0bGluZS5sYWJlbCA9ICdPdXRsaW5lJztcbiAgc2VsZWN0LmFwcGVuZENoaWxkKG91dGxpbmUpO1xuICBjb25zdCBncm91cE1hcDogUmVjb3JkPHN0cmluZywgSFRNTE9wdEdyb3VwRWxlbWVudD4gPSB7fTtcbiAgbGV0IGdyb3VwOiBIVE1MT3B0R3JvdXBFbGVtZW50O1xuICBmb3IgKGNvbnN0IHQgb2YgdHJlZS50cmVlaXRlbXMpIHtcbiAgICBpZiAoTnVtYmVyKHQuZGVwdGgpID4gNCkgY29udGludWU7XG4gICAgaWYgKHQuZ3JvdXBUcmVlaXRlbSkge1xuICAgICAgZ3JvdXAgPSBncm91cE1hcFt0Lmdyb3VwVHJlZWl0ZW0ubGFiZWxdO1xuICAgICAgaWYgKCFncm91cCkge1xuICAgICAgICBncm91cCA9IGdyb3VwTWFwW3QuZ3JvdXBUcmVlaXRlbS5sYWJlbF0gPSBkb2N1bWVudC5jcmVhdGVFbGVtZW50KCdvcHRncm91cCcpO1xuICAgICAgICBncm91cC5sYWJlbCA9IHQuZ3JvdXBUcmVlaXRlbS5sYWJlbDtcbiAgICAgICAgc2VsZWN0LmFwcGVuZENoaWxkKGdyb3VwKTtcbiAgICAgIH1cbiAgICB9IGVsc2Uge1xuICAgICAgZ3JvdXAgPSBvdXRsaW5lO1xuICAgIH1cbiAgICBjb25zdCBvID0gZG9jdW1lbnQuY3JlYXRlRWxlbWVudCgnb3B0aW9uJyk7XG4gICAgby5sYWJlbCA9IHQubGFiZWw7XG4gICAgby50ZXh0Q29udGVudCA9IHQubGFiZWw7XG4gICAgby52YWx1ZSA9ICh0LmVsIGFzIEhUTUxBbmNob3JFbGVtZW50KS5ocmVmLnJlcGxhY2Uod2luZG93LmxvY2F0aW9uLm9yaWdpbiwgJycpLnJlcGxhY2UoJy8nLCAnJyk7XG4gICAgZ3JvdXAuYXBwZW5kQ2hpbGQobyk7XG4gIH1cbiAgdHJlZS5hZGRPYnNlcnZlcih0ID0+IHtcbiAgICBjb25zdCB2YWx1ZSA9IHNlbGVjdC5xdWVyeVNlbGVjdG9yPEhUTUxPcHRpb25FbGVtZW50PihgW2xhYmVsPVwiJHt0LmxhYmVsfVwiXWApPy52YWx1ZTtcbiAgICBpZiAodmFsdWUpIHtcbiAgICAgIHNlbGVjdC52YWx1ZSA9IHZhbHVlO1xuICAgIH1cbiAgfSwgNTApO1xuICByZXR1cm4gbGFiZWw7XG59XG4iXSwKICAibWFwcGluZ3MiOiAiQUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFTTyxpQ0FBMEI7QUFBQSxFQUMvQixZQUFvQixJQUFhO0FBQWI7QUFDbEIsU0FBSyxHQUFHLGlCQUFpQixVQUFVLE9BQUs7QUFDdEMsWUFBTSxTQUFTLEVBQUU7QUFDakIsVUFBSSxPQUFPLE9BQU87QUFDbEIsVUFBSSxDQUFDLE9BQU8sTUFBTSxXQUFXLE1BQU07QUFDakMsZUFBTyxNQUFNO0FBQUE7QUFFZixhQUFPLFNBQVMsT0FBTztBQUFBO0FBQUE7QUFBQTtBQUt0Qiw4QkFBdUIsTUFBMkM7QUFDdkUsUUFBTSxRQUFRLFNBQVMsY0FBYztBQUNyQyxRQUFNLFVBQVUsSUFBSTtBQUNwQixRQUFNLGFBQWEsY0FBYztBQUNqQyxRQUFNLFNBQVMsU0FBUyxjQUFjO0FBQ3RDLFNBQU8sVUFBVSxJQUFJLGFBQWE7QUFDbEMsUUFBTSxZQUFZO0FBQ2xCLFFBQU0sVUFBVSxTQUFTLGNBQWM7QUFDdkMsVUFBUSxRQUFRO0FBQ2hCLFNBQU8sWUFBWTtBQUNuQixRQUFNLFdBQWdEO0FBQ3RELE1BQUk7QUFDSixhQUFXLEtBQUssS0FBSyxXQUFXO0FBQzlCLFFBQUksT0FBTyxFQUFFLFNBQVM7QUFBRztBQUN6QixRQUFJLEVBQUUsZUFBZTtBQUNuQixjQUFRLFNBQVMsRUFBRSxjQUFjO0FBQ2pDLFVBQUksQ0FBQyxPQUFPO0FBQ1YsZ0JBQVEsU0FBUyxFQUFFLGNBQWMsU0FBUyxTQUFTLGNBQWM7QUFDakUsY0FBTSxRQUFRLEVBQUUsY0FBYztBQUM5QixlQUFPLFlBQVk7QUFBQTtBQUFBLFdBRWhCO0FBQ0wsY0FBUTtBQUFBO0FBRVYsVUFBTSxJQUFJLFNBQVMsY0FBYztBQUNqQyxNQUFFLFFBQVEsRUFBRTtBQUNaLE1BQUUsY0FBYyxFQUFFO0FBQ2xCLE1BQUUsUUFBUyxFQUFFLEdBQXlCLEtBQUssUUFBUSxPQUFPLFNBQVMsUUFBUSxJQUFJLFFBQVEsS0FBSztBQUM1RixVQUFNLFlBQVk7QUFBQTtBQUVwQixPQUFLLFlBQVksT0FBSztBQUNwQixVQUFNLFFBQVEsT0FBTyxjQUFpQyxXQUFXLEVBQUUsWUFBWTtBQUMvRSxRQUFJLE9BQU87QUFDVCxhQUFPLFFBQVE7QUFBQTtBQUFBLEtBRWhCO0FBQ0gsU0FBTztBQUFBOyIsCiAgIm5hbWVzIjogW10KfQo=
