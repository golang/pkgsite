var r=document.querySelector(".js-siteHeader"),n=document.createElement("div");r==null||r.prepend(n);var c=new IntersectionObserver(([o])=>{if(o.intersectionRatio<1)for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.setAttribute("data-fixed","true");else for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.removeAttribute("data-fixed")},{threshold:1,rootMargin:"245px"});c.observe(n);var t=document.querySelector(".js-searchHeader");t==null||t.addEventListener("dblclick",o=>{var s;let e=o.target;(e===t||e===t.lastElementChild)&&((s=window.getSelection())==null||s.removeAllRanges(),window.scrollTo({top:0,behavior:"smooth"}))});
/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=search.js.map
