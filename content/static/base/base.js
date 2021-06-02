(()=>{function l(){let t=document.querySelector(".js-header"),r=document.querySelectorAll(".js-headerMenuButton");r.forEach(o=>{o.addEventListener("click",e=>{e.preventDefault(),t.classList.toggle("is-active"),o.setAttribute("aria-expanded",String(t.classList.contains("is-active")))})}),document.querySelector(".js-scrim").addEventListener("click",o=>{o.preventDefault(),t.classList.remove("is-active"),r.forEach(e=>{e.setAttribute("aria-expanded",String(t.classList.contains("is-active")))})})}function m(){let t=512,r=document.querySelector(".js-headerLogo"),s=document.querySelector(".js-searchForm"),o=document.querySelector(".js-searchFormSubmit"),e=s.querySelector("input");i(),window.addEventListener("resize",i);function i(){window.innerWidth>t?(r.classList.remove("go-Header-logo--hidden"),s.classList.remove("go-SearchForm--open"),e.removeEventListener("focus",n),e.removeEventListener("keypress",a),e.removeEventListener("focusout",d)):(o.addEventListener("click",u),e.addEventListener("focus",n),e.addEventListener("keypress",a),e.addEventListener("focusout",d))}function a(c){c.key==="Enter"&&s.submit()}function n(){r.classList.add("go-Header-logo--hidden"),s.classList.add("go-SearchForm--open")}function d(){r.classList.remove("go-Header-logo--hidden"),s.classList.remove("go-SearchForm--open")}function u(c){c.preventDefault(),n(),e.focus()}}l();m();})();
/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=base.js.map
