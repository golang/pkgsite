var o=document.querySelector(".js-siteHeader"),n=document.createElement("div");o==null||o.prepend(n);var c=new IntersectionObserver(([r])=>{if(r.intersectionRatio<1)for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.setAttribute("data-fixed","true");else for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.removeAttribute("data-fixed")},{threshold:1,rootMargin:`${3.5*16*3}px`});c.observe(n);var t=document.querySelector(".js-searchHeader");t==null||t.addEventListener("dblclick",r=>{var s;let e=r.target;(e===t||e===t.lastElementChild)&&((s=window.getSelection())==null||s.removeAllRanges(),window.scrollTo({top:0,behavior:"smooth"}))});
/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=search.js.map
