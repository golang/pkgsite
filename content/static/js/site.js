/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */(function(){"use strict";const e=document.querySelector(".js-header"),a=document.querySelectorAll(".js-headerMenuButton");a.forEach(n=>{n.addEventListener("click",s=>{s.preventDefault(),e?.classList.toggle("is-active"),n.setAttribute("aria-expanded",`${e?.classList.contains("is-active")??!1}`)})});const r=document.querySelector(".js-scrim");r&&r.hasOwnProperty("addEventListener")&&r.addEventListener("click",n=>{n.preventDefault(),e?.classList.remove("is-active"),a.forEach(s=>{s.setAttribute("aria-expanded",`${e?.classList.contains("is-active")??!1}`)})})})(),function(){window.dataLayer=window.dataLayer||[],window.dataLayer.push({"gtm.start":new Date().getTime(),event:"gtm.js"})}();function removeUTMSource(){const t=new URLSearchParams(window.location.search),e=t.get("utm_source");if(e!=="gopls"&&e!=="godoc")return;const a=new URL(window.location.href);t.delete("utm_source"),a.search=t.toString(),window.history.replaceState(null,"",a.toString())}document.querySelector(".js-gtmID")?.dataset.gtmid&&window.dataLayer?window.dataLayer.push(function(){removeUTMSource()}):removeUTMSource();
//# sourceMappingURL=site.js.map
