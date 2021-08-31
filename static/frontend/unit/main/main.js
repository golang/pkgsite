var p={PLAY_HREF:".js-exampleHref",PLAY_CONTAINER:".js-exampleContainer",EXAMPLE_INPUT:".Documentation-exampleCode",EXAMPLE_OUTPUT:".Documentation-exampleOutput",EXAMPLE_ERROR:".Documentation-exampleError",PLAY_BUTTON:".Documentation-examplePlayButton",SHARE_BUTTON:".Documentation-exampleShareButton",FORMAT_BUTTON:".Documentation-exampleFormatButton",RUN_BUTTON:".Documentation-exampleRunButton"},I=class{constructor(e){this.exampleEl=e;var t,i,s,l;this.exampleEl=e,this.anchorEl=e.querySelector("a"),this.errorEl=e.querySelector(p.EXAMPLE_ERROR),this.playButtonEl=e.querySelector(p.PLAY_BUTTON),this.shareButtonEl=e.querySelector(p.SHARE_BUTTON),this.formatButtonEl=e.querySelector(p.FORMAT_BUTTON),this.runButtonEl=e.querySelector(p.RUN_BUTTON),this.inputEl=this.makeTextArea(e.querySelector(p.EXAMPLE_INPUT)),this.outputEl=e.querySelector(p.EXAMPLE_OUTPUT),(t=this.playButtonEl)==null||t.addEventListener("click",()=>this.handleShareButtonClick()),(i=this.shareButtonEl)==null||i.addEventListener("click",()=>this.handleShareButtonClick()),(s=this.formatButtonEl)==null||s.addEventListener("click",()=>this.handleFormatButtonClick()),(l=this.runButtonEl)==null||l.addEventListener("click",()=>this.handleRunButtonClick()),!!this.inputEl&&(this.resize(),this.inputEl.addEventListener("keyup",()=>this.resize()),this.inputEl.addEventListener("keydown",n=>this.onKeydown(n)))}makeTextArea(e){var i,s;let t=document.createElement("textarea");return t.classList.add("Documentation-exampleCode","code"),t.spellcheck=!1,t.value=(i=e==null?void 0:e.textContent)!=null?i:"",(s=e==null?void 0:e.parentElement)==null||s.replaceChild(t,e),t}getAnchorHash(){var e;return(e=this.anchorEl)==null?void 0:e.hash}expand(){this.exampleEl.open=!0}resize(){var e;if((e=this.inputEl)==null?void 0:e.value){let t=(this.inputEl.value.match(/\n/g)||[]).length;this.inputEl.style.height=`${(20+t*20+12+2)/16}rem`}}onKeydown(e){e.key==="Tab"&&(document.execCommand("insertText",!1,"	"),e.preventDefault())}setInputText(e){this.inputEl&&(this.inputEl.value=e)}setOutputText(e){this.outputEl&&(this.outputEl.textContent=e)}setErrorText(e){this.errorEl&&(this.errorEl.textContent=e),this.setOutputText("An error has occurred\u2026")}handleShareButtonClick(){var t;let e="https://play.golang.org/p/";this.setOutputText("Waiting for remote server\u2026"),fetch("/play/share",{method:"POST",body:(t=this.inputEl)==null?void 0:t.value}).then(i=>i.text()).then(i=>{let s=e+i;this.setOutputText(`<a href="${s}">${s}</a>`),window.open(s)}).catch(i=>{this.setErrorText(i)})}handleFormatButtonClick(){var t,i;this.setOutputText("Waiting for remote server\u2026");let e=new FormData;e.append("body",(i=(t=this.inputEl)==null?void 0:t.value)!=null?i:""),fetch("/play/fmt",{method:"POST",body:e}).then(s=>s.json()).then(({Body:s,Error:l})=>{this.setOutputText(l||"Done."),s&&(this.setInputText(s),this.resize())}).catch(s=>{this.setErrorText(s)})}handleRunButtonClick(){var e;this.setOutputText("Waiting for remote server\u2026"),fetch("/play/compile",{method:"POST",body:JSON.stringify({body:(e=this.inputEl)==null?void 0:e.value,version:2})}).then(t=>t.json()).then(async({Events:t,Errors:i})=>{this.setOutputText(i||"");for(let s of t||[])this.setOutputText(s.Message),await new Promise(l=>setTimeout(l,s.Delay/1e6))}).catch(t=>{this.setErrorText(t)})}};function M(){let r=location.hash.match(/^#(example-.*)$/);if(r){let i=document.getElementById(r[1]);i&&(i.open=!0)}let e=[...document.querySelectorAll(p.PLAY_HREF)],t=i=>e.find(s=>s.hash===i.getAnchorHash());for(let i of document.querySelectorAll(p.PLAY_CONTAINER)){let s=new I(i),l=t(s);l?l.addEventListener("click",()=>{s.expand()}):console.warn("example href not found")}}var c=document.querySelector(".JumpDialog"),f=c==null?void 0:c.querySelector(".JumpDialog-body"),o=c==null?void 0:c.querySelector(".JumpDialog-list"),u=c==null?void 0:c.querySelector(".JumpDialog-input"),S=document.querySelector(".js-documentation"),h;function _(){let r=[];if(!!S){for(let e of S.querySelectorAll("[data-kind]"))r.push(V(e));for(let e of r)e.link.addEventListener("click",function(){c==null||c.close()});return r.sort(function(e,t){return e.lower.localeCompare(t.lower)}),r}}function V(r){var s;let e=document.createElement("a"),t=r.getAttribute("id");e.setAttribute("href","#"+t),e.setAttribute("tabindex","-1"),e.setAttribute("data-gtmc","jump to link");let i=r.getAttribute("data-kind");return{link:e,name:t!=null?t:"",kind:i!=null?i:"",lower:(s=t==null?void 0:t.toLowerCase())!=null?s:""}}var H,T=-1;function v(r){for(H=r,h||(h=_()),x(-1);o==null?void 0:o.firstChild;)o.firstChild.remove();if(r){let e=r.toLowerCase(),t=[],i=[],s=[],l=(n,a,d)=>n.name.substring(0,a)+"<b>"+n.name.substring(a,d)+"</b>"+n.name.substring(d);for(let n of h!=null?h:[]){let a=n.name.toLowerCase();if(a===e)n.link.innerHTML=l(n,0,n.name.length),t.push(n);else if(a.startsWith(e))n.link.innerHTML=l(n,0,r.length),i.push(n);else{let d=a.indexOf(e);d>-1&&(n.link.innerHTML=l(n,d,d+r.length),s.push(n))}}for(let n of t.concat(i).concat(s))o==null||o.appendChild(n.link)}else{if(!h||h.length===0){let e=document.createElement("i");e.innerHTML="There are no symbols on this page.",o==null||o.appendChild(e)}for(let e of h!=null?h:[])e.link.innerHTML=e.name+" <i>"+e.kind+"</i>",o==null||o.appendChild(e.link)}f&&(f.scrollTop=0),(h==null?void 0:h.length)&&o&&o.children.length>0&&x(0)}function x(r){let e=o==null?void 0:o.children;if(!(!e||!f)){if(T>=0&&e[T].classList.remove("JumpDialog-active"),r>=e.length&&(r=e.length-1),r>=0){e[r].classList.add("JumpDialog-active");let t=e[r].offsetTop-e[0].offsetTop,i=t+e[r].clientHeight;t<f.scrollTop?f.scrollTop=t:i>f.scrollTop+f.clientHeight&&(f.scrollTop=i-f.clientHeight)}T=r}}function O(r){if(T<0)return;let e=T+r;e<0&&(e=0),x(e)}function B(){u==null||u.addEventListener("keyup",function(){u.value.toUpperCase()!=H.toUpperCase()&&v(u.value)}),u==null||u.addEventListener("keydown",function(t){let i=38,s=40,l=13;switch(t.which){case i:O(-1),t.preventDefault();break;case s:O(1),t.preventDefault();break;case l:T>=0&&o&&(o.children[T].click(),t.preventDefault());break}});let r=document.querySelector(".ShortcutsDialog");document.addEventListener("keypress",function(t){if((c==null?void 0:c.open)||(r==null?void 0:r.open))return;let i=t.target,s=i==null?void 0:i.tagName;if(s=="INPUT"||s=="SELECT"||s=="TEXTAREA"||(i==null?void 0:i.contentEditable)=="true"||t.metaKey||t.ctrlKey)return;switch(String.fromCharCode(t.which)){case"f":case"F":t.preventDefault(),u&&(u.value=""),c==null||c.showModal(),u==null||u.focus(),v("");break;case"?":r==null||r.showModal();break}});let e=document.querySelector(".js-jumpToInput");e&&e.addEventListener("click",()=>{u&&(u.value=""),v("")})}var g=class{constructor(e){this.el=e;this.el.addEventListener("change",t=>{let i=t.target,s=i.value;i.value.startsWith("/")||(s="/"+s),window.location.href=s})}};function R(r){let e=document.createElement("label");e.classList.add("go-Label"),e.setAttribute("aria-label","Menu");let t=document.createElement("select");t.classList.add("go-Select","js-selectNav"),e.appendChild(t);let i=document.createElement("optgroup");i.label="Outline",t.appendChild(i);let s={},l;for(let n of r.treeitems){if(Number(n.depth)>4)continue;n.groupTreeitem?(l=s[n.groupTreeitem.label],l||(l=s[n.groupTreeitem.label]=document.createElement("optgroup"),l.label=n.groupTreeitem.label,t.appendChild(l))):l=i;let a=document.createElement("option");a.label=n.label,a.textContent=n.label,a.value=n.el.href.replace(window.location.origin,"").replace("/",""),l.appendChild(a)}return r.addObserver(n=>{var E;let a=n.el.hash,d=(E=t.querySelector(`[value$="${a}"]`))==null?void 0:E.value;d&&(t.value=d)},50),e}var L=class{constructor(e){this.el=e;this.handleResize=()=>{this.el.style.setProperty("--js-tree-height","100vh"),this.el.style.setProperty("--js-tree-height",this.el.clientHeight+"px")};this.treeitems=[],this.firstChars=[],this.firstTreeitem=null,this.lastTreeitem=null,this.observerCallbacks=[],this.init()}init(){this.handleResize(),window.addEventListener("resize",this.handleResize),this.findTreeItems(),this.updateVisibleTreeitems(),this.observeTargets(),this.firstTreeitem&&(this.firstTreeitem.el.tabIndex=0)}observeTargets(){this.addObserver(i=>{this.expandTreeitem(i),this.setSelected(i)});let e=new Map,t=new IntersectionObserver(i=>{for(let s of i)e.set(s.target.id,s.isIntersecting||s.intersectionRatio===1);for(let[s,l]of e)if(l){let n=this.treeitems.find(a=>{var d;return(d=a.el)==null?void 0:d.href.endsWith(`#${s}`)});if(n)for(let a of this.observerCallbacks)a(n);break}},{threshold:1,rootMargin:"-60px 0px 0px 0px"});for(let i of this.treeitems.map(s=>s.el.getAttribute("href")))if(i){let s=i.replace(window.location.origin,"").replace("/","").replace("#",""),l=document.getElementById(s);l&&t.observe(l)}}addObserver(e,t=200){this.observerCallbacks.push(J(e,t))}setFocusToNextItem(e){let t=null;for(let i=e.index+1;i<this.treeitems.length;i++){let s=this.treeitems[i];if(s.isVisible){t=s;break}}t&&this.setFocusToItem(t)}setFocusToPreviousItem(e){let t=null;for(let i=e.index-1;i>-1;i--){let s=this.treeitems[i];if(s.isVisible){t=s;break}}t&&this.setFocusToItem(t)}setFocusToParentItem(e){e.groupTreeitem&&this.setFocusToItem(e.groupTreeitem)}setFocusToFirstItem(){this.firstTreeitem&&this.setFocusToItem(this.firstTreeitem)}setFocusToLastItem(){this.lastTreeitem&&this.setFocusToItem(this.lastTreeitem)}setSelected(e){var t;for(let i of this.el.querySelectorAll('[aria-expanded="true"]'))i!==e.el&&(((t=i.nextElementSibling)==null?void 0:t.contains(e.el))||i.setAttribute("aria-expanded","false"));for(let i of this.el.querySelectorAll("[aria-selected]"))i!==e.el&&i.setAttribute("aria-selected","false");e.el.setAttribute("aria-selected","true"),this.updateVisibleTreeitems(),this.setFocusToItem(e,!1)}expandTreeitem(e){let t=e;for(;t;)t.isExpandable&&t.el.setAttribute("aria-expanded","true"),t=t.groupTreeitem;this.updateVisibleTreeitems()}expandAllSiblingItems(e){for(let t of this.treeitems)t.groupTreeitem===e.groupTreeitem&&t.isExpandable&&this.expandTreeitem(t)}collapseTreeitem(e){let t=null;e.isExpanded()?t=e:t=e.groupTreeitem,t&&(t.el.setAttribute("aria-expanded","false"),this.updateVisibleTreeitems(),this.setFocusToItem(t))}setFocusByFirstCharacter(e,t){let i,s;t=t.toLowerCase(),i=e.index+1,i===this.treeitems.length&&(i=0),s=this.getIndexFirstChars(i,t),s===-1&&(s=this.getIndexFirstChars(0,t)),s>-1&&this.setFocusToItem(this.treeitems[s])}findTreeItems(){let e=(t,i)=>{let s=i,l=t.firstElementChild;for(;l;)(l.tagName==="A"||l.tagName==="SPAN")&&(s=new N(l,this,i),this.treeitems.push(s),this.firstChars.push(s.label.substring(0,1).toLowerCase())),l.firstElementChild&&e(l,s),l=l.nextElementSibling};e(this.el,null),this.treeitems.map((t,i)=>t.index=i)}updateVisibleTreeitems(){this.firstTreeitem=this.treeitems[0];for(let e of this.treeitems){let t=e.groupTreeitem;for(e.isVisible=!0;t&&t.el!==this.el;)t.isExpanded()||(e.isVisible=!1),t=t.groupTreeitem;e.isVisible&&(this.lastTreeitem=e)}}setFocusToItem(e,t=!0){e.el.tabIndex=0,t&&e.el.focus();for(let i of this.treeitems)i!==e&&(i.el.tabIndex=-1)}getIndexFirstChars(e,t){for(let i=e;i<this.firstChars.length;i++)if(this.treeitems[i].isVisible&&t===this.firstChars[i])return i;return-1}},N=class{constructor(e,t,i){var n,a,d,E,k;e.tabIndex=-1,this.el=e,this.groupTreeitem=i,this.label=(a=(n=e.textContent)==null?void 0:n.trim())!=null?a:"",this.tree=t,this.depth=((i==null?void 0:i.depth)||0)+1,this.index=0;let s=e.parentElement;(s==null?void 0:s.tagName.toLowerCase())==="li"&&(s==null||s.setAttribute("role","none")),e.setAttribute("aria-level",this.depth+""),e.getAttribute("aria-label")&&(this.label=(E=(d=e==null?void 0:e.getAttribute("aria-label"))==null?void 0:d.trim())!=null?E:""),this.isExpandable=!1,this.isVisible=!1,this.isInGroup=!!i;let l=e.nextElementSibling;for(;l;){if(l.tagName.toLowerCase()=="ul"){let w=`${(k=i==null?void 0:i.label)!=null?k:""} nav group ${this.label}`.replace(/[\W_]+/g,"_");e.setAttribute("aria-owns",w),e.setAttribute("aria-expanded","false"),l.setAttribute("role","group"),l.setAttribute("id",w),this.isExpandable=!0;break}l=l.nextElementSibling}this.init()}init(){this.el.tabIndex=-1,this.el.getAttribute("role")||this.el.setAttribute("role","treeitem"),this.el.addEventListener("keydown",this.handleKeydown.bind(this)),this.el.addEventListener("click",this.handleClick.bind(this)),this.el.addEventListener("focus",this.handleFocus.bind(this)),this.el.addEventListener("blur",this.handleBlur.bind(this))}isExpanded(){return this.isExpandable?this.el.getAttribute("aria-expanded")==="true":!1}isSelected(){return this.el.getAttribute("aria-selected")==="true"}handleClick(e){e.target!==this.el&&e.target!==this.el.firstElementChild||(this.isExpandable&&(this.isExpanded()&&this.isSelected()?this.tree.collapseTreeitem(this):this.tree.expandTreeitem(this),e.stopPropagation()),this.tree.setSelected(this))}handleFocus(){var t;let e=this.el;this.isExpandable&&(e=(t=e.firstElementChild)!=null?t:e),e.classList.add("focus")}handleBlur(){var t;let e=this.el;this.isExpandable&&(e=(t=e.firstElementChild)!=null?t:e),e.classList.remove("focus")}handleKeydown(e){if(e.altKey||e.ctrlKey||e.metaKey)return;let t=!1;switch(e.key){case" ":case"Enter":this.isExpandable?(this.isExpanded()&&this.isSelected()?this.tree.collapseTreeitem(this):this.tree.expandTreeitem(this),t=!0):e.stopPropagation(),this.tree.setSelected(this);break;case"ArrowUp":this.tree.setFocusToPreviousItem(this),t=!0;break;case"ArrowDown":this.tree.setFocusToNextItem(this),t=!0;break;case"ArrowRight":this.isExpandable&&(this.isExpanded()?this.tree.setFocusToNextItem(this):this.tree.expandTreeitem(this)),t=!0;break;case"ArrowLeft":this.isExpandable&&this.isExpanded()?(this.tree.collapseTreeitem(this),t=!0):this.isInGroup&&(this.tree.setFocusToParentItem(this),t=!0);break;case"Home":this.tree.setFocusToFirstItem(),t=!0;break;case"End":this.tree.setFocusToLastItem(),t=!0;break;default:e.key.length===1&&e.key.match(/\S/)&&(e.key=="*"?this.tree.expandAllSiblingItems(this):this.tree.setFocusByFirstCharacter(this,e.key),t=!0);break}t&&(e.stopPropagation(),e.preventDefault())}};function J(r,e){let t;return(...i)=>{let s=()=>{t=null,r(...i)};t&&clearTimeout(t),t=setTimeout(s,e)}}var y=class{constructor(e,t){this.table=e;this.toggleAll=t;this.expandAllItems=()=>{this.toggles.map(e=>e.setAttribute("aria-expanded","true")),this.update()};this.collapseAllItems=()=>{this.toggles.map(e=>e.setAttribute("aria-expanded","false")),this.update()};this.update=()=>{this.updateVisibleItems(),setTimeout(()=>this.updateGlobalToggle())};this.rows=Array.from(e.querySelectorAll("[data-aria-controls]")),this.toggles=Array.from(this.table.querySelectorAll("[aria-expanded]")),this.setAttributes(),this.attachEventListeners(),this.update()}setAttributes(){for(let e of["data-aria-controls","data-aria-labelledby","data-id"])this.table.querySelectorAll(`[${e}]`).forEach(t=>{var i;t.setAttribute(e.replace("data-",""),(i=t.getAttribute(e))!=null?i:""),t.removeAttribute(e)})}attachEventListeners(){var e;this.rows.forEach(t=>{t.addEventListener("click",i=>{this.handleToggleClick(i)})}),(e=this.toggleAll)==null||e.addEventListener("click",()=>{this.expandAllItems()}),document.addEventListener("keydown",t=>{(t.ctrlKey||t.metaKey)&&t.key==="f"&&this.expandAllItems()})}handleToggleClick(e){let t=e.currentTarget;(t==null?void 0:t.hasAttribute("aria-expanded"))||(t=this.table.querySelector(`button[aria-controls="${t==null?void 0:t.getAttribute("aria-controls")}"]`));let i=(t==null?void 0:t.getAttribute("aria-expanded"))==="true";t==null||t.setAttribute("aria-expanded",i?"false":"true"),e.stopPropagation(),this.update()}updateVisibleItems(){this.rows.map(e=>{var s;let t=(e==null?void 0:e.getAttribute("aria-expanded"))==="true",i=(s=e==null?void 0:e.getAttribute("aria-controls"))==null?void 0:s.trimEnd().split(" ");i==null||i.map(l=>{let n=document.getElementById(`${l}`);t?(n==null||n.classList.add("visible"),n==null||n.classList.remove("hidden")):(n==null||n.classList.add("hidden"),n==null||n.classList.remove("visible"))})})}updateGlobalToggle(){if(!this.toggleAll)return;this.rows.some(t=>t.hasAttribute("aria-expanded"))&&(this.toggleAll.style.display="block"),this.toggles.some(t=>t.getAttribute("aria-expanded")==="false")?(this.toggleAll.innerText="Expand all",this.toggleAll.onclick=this.expandAllItems):(this.toggleAll.innerText="Collapse all",this.toggleAll.onclick=this.collapseAllItems)}};B();M();var P=document.querySelector(".js-expandableTable");if(P){let r=new y(P,document.querySelector(".js-expandAllDirectories"));window.location.search.includes("expand-directories")&&r.expandAllItems()}var q=document.querySelector(".js-tree");if(q){let r=new L(q),e=R(r),t=document.querySelector(".js-mainNavMobile");t&&t.firstElementChild&&(t==null||t.replaceChild(e,t.firstElementChild)),e.firstElementChild&&new g(e.firstElementChild)}var m=document.querySelector(".js-readme"),A=document.querySelector(".js-readmeContent"),F=document.querySelector(".js-readmeOutline"),b=document.querySelectorAll(".js-readmeExpand"),U=document.querySelector(".js-readmeCollapse"),C=document.querySelector(".DocNavMobile-select");m&&A&&F&&b.length&&U&&(window.location.hash.includes("readme")&&m.classList.add("UnitReadme--expanded"),C==null||C.addEventListener("change",r=>{r.target.value.startsWith("readme-")&&m.classList.add("UnitReadme--expanded")}),b.forEach(r=>r.addEventListener("click",e=>{e.preventDefault(),m.classList.add("UnitReadme--expanded"),m.scrollIntoView()})),U.addEventListener("click",r=>{r.preventDefault(),m.classList.remove("UnitReadme--expanded"),b[1]&&b[1].scrollIntoView({block:"center"})}),A.addEventListener("keyup",()=>{m.classList.add("UnitReadme--expanded")}),A.addEventListener("click",()=>{m.classList.add("UnitReadme--expanded")}),F.addEventListener("click",()=>{m.classList.add("UnitReadme--expanded")}),document.addEventListener("keydown",r=>{(r.ctrlKey||r.metaKey)&&r.key==="f"&&m.classList.add("UnitReadme--expanded")}));function D(){var t;if(!location.hash)return;let r=document.getElementById(location.hash.slice(1)),e=(t=r==null?void 0:r.parentElement)==null?void 0:t.parentElement;(e==null?void 0:e.nodeName)==="DETAILS"&&(e.open=!0)}D();window.addEventListener("hashchange",()=>D());document.querySelectorAll(".js-buildContextSelect").forEach(r=>{r.addEventListener("change",e=>{window.location.search=`?GOOS=${e.target.value}`})});
/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=main.js.map
