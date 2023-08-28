var s=document.querySelector(".js-siteHeader"),o=document.createElement("div");s?.prepend(o);var c=new IntersectionObserver(([t])=>{if(t.intersectionRatio<1)for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.setAttribute("data-fixed","true");else for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.removeAttribute("data-fixed")},{threshold:1,rootMargin:`${3.5*16*3}px`});c.observe(o);var r=document.querySelector(".js-searchHeader");r?.addEventListener("dblclick",t=>{let e=t.target;(e===r||e===r.lastElementChild)&&(window.getSelection()?.removeAllRanges(),window.scrollTo({top:0,behavior:"smooth"}))});
/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=search.js.map
