/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */class FixedHeaderController{constructor(e,t){this.el=e;this.fixedEl=t;this.intersectionObserverCallback=e=>{e.forEach(t=>{t.isIntersecting?this.fixedEl.classList.remove("UnitFixedHeader--visible"):this.fixedEl.classList.add("UnitFixedHeader--visible")})};if(!e||!t)throw new Error("Must provide sentinel and fixed elements to constructor.");this.intersectionObserver=new IntersectionObserver(this.intersectionObserverCallback,{threshold:1}),this.intersectionObserver.observe(this.el),window.getComputedStyle(document.body)["-webkit-overflow-scrolling"]!==void 0&&[document.documentElement,document.body].forEach(i=>{i.style.overflow="auto"})}}const fixedHeaderSentinel=document.querySelector(".js-fixedHeaderSentinel"),fixedHeader=document.querySelector(".js-fixedHeader");fixedHeaderSentinel&&fixedHeader&&new FixedHeaderController(fixedHeaderSentinel,fixedHeader);const overflowSelect=document.querySelector(".js-overflowSelect");overflowSelect&&overflowSelect.addEventListener("change",r=>{window.location.href=r.target.value});
//# sourceMappingURL=unit_fixed_header.js.map
