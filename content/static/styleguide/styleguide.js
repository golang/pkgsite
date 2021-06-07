/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import{ClipboardController as i}from"../clipboard/clipboard.js";import{SelectNavController as c,makeSelectNav as s}from"../outline/select.js";import{ToolTipController as a}from"../tooltip/tooltip.js";import{TreeNavController as n}from"../outline/tree.js";import{ModalController as u}from"../modal/modal.js";window.addEventListener("load",()=>{const t=document.querySelector(".js-tree");if(t){const e=new n(t),r=s(e);document.querySelector(".js-mainNavMobile").appendChild(r)}const l=document.querySelector(".Outline .js-tree");if(l){const e=new n(l),r=s(e);document.querySelector(".Outline .js-select").appendChild(r)}for(const e of document.querySelectorAll(".js-toggleTheme"))e.addEventListener("click",r=>{const o=r.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-theme",o)});for(const e of document.querySelectorAll(".js-toggleLayout"))e.addEventListener("click",r=>{const o=r.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-layout",o)});for(const e of document.querySelectorAll(".js-clipboard"))new i(e);for(const e of document.querySelectorAll(".js-selectNav"))new c(e);for(const e of document.querySelectorAll(".js-tooltip"))new a(e);for(const e of document.querySelectorAll(".js-modal"))new u(e)}),customElements.define("go-color",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const t=this.id;this.removeAttribute("id"),this.innerHTML=`
        <div style="--color: var(${t});" class="GoColor-circle"></div>
        <span>
          <div id="${t}" class="go-textLabel GoColor-title">${t.replace("--color-","").replaceAll("-"," ")}</div>
          <pre class="StringifyElement-markup">var(${t})</pre>
        </span>
      `,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(`var(${t})`)}}}),customElements.define("go-icon",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const t=this.getAttribute("name");this.innerHTML=`
        <p id="icon-${t}" class="go-textLabel GoIcon-title">${t.replaceAll("_"," ")}</p>
        <stringify-el>
          <img class="go-Icon" height="24" width="24" src="/static/icon/${t}_gm_grey_24dp.svg" alt="">
        </stringify-el>
      `}}),customElements.define("clone-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const t=this.getAttribute("selector"),l="    "+document.querySelector(t).outerHTML;this.innerHTML=`
        <stringify-el collapsed>${l}</stringify-el>
      `}}),customElements.define("stringify-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const t=this.innerHTML,l=this.id?` id="${this.id}"`:"";this.removeAttribute("id");let e='<pre class="StringifyElement-markup">'+m(d(t))+"</pre>";this.hasAttribute("collapsed")&&(e=`<details class="StringifyElement-details"><summary>Markup</summary>${e}</details>`),this.innerHTML=`<span${l}>${t}</span>${e}`,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(t)}}});function d(t){return t.split(`
`).reduce((l,e)=>{if(l.result.length===0){const r=e.indexOf("<");l.start=r===-1?0:r}return e=e.slice(l.start),e&&l.result.push(e),l},{result:[],start:0}).result.join(`
`)}function m(t){return t.replaceAll("<","&lt;").replaceAll(">","&gt;")}
//# sourceMappingURL=styleguide.js.map
