/* Levier — documentation shell.
   manifest.json drives navigation; content/*.md is fetched and rendered in place. */
(function () {
  "use strict";

  var MANIFEST_URL = "manifest.json";
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)");

  var els = {
    nav: document.getElementById("docs-nav"),
    state: document.getElementById("doc-state"),
    view: document.getElementById("doc-view"),
    doc: document.getElementById("doc"),
    pager: document.getElementById("doc-pager"),
    toc: document.getElementById("doc-toc"),
    tocList: document.getElementById("doc-toc-list"),
    sidebar: document.getElementById("docs-sidebar"),
    backdrop: document.getElementById("docs-backdrop"),
    menuBtn: document.getElementById("docs-menu-btn"),
    closeBtn: document.getElementById("docs-close-btn")
  };

  var app = {
    manifest: null,
    flat: [],
    current: null,
    tocObserver: null
  };

  /* ------------------------------------------------------------------------
     Utilities
     ------------------------------------------------------------------------ */

  function flatten(manifest) {
    var flat = [];
    (manifest.sections || []).forEach(function (section) {
      (section.items || []).forEach(function (item) {
        flat.push({
          slug: item.slug,
          title: item.title,
          status: item.status || "draft",
          file: item.file,
          description: item.description || "",
          section: section.title || ""
        });
      });
    });
    return flat;
  }

  function find(slug) {
    for (var i = 0; i < app.flat.length; i += 1) {
      if (app.flat[i].slug === slug) return app.flat[i];
    }
    return null;
  }

  function currentSlug() {
    var raw = window.location.hash.replace(/^#\/?/, "").trim();
    return raw.split("/")[0] || "";
  }

  function slugify(text, used) {
    var base = text
      .trim()
      .toLowerCase()
      .replace(/\s+/g, "-")
      .replace(/[^\p{L}\p{N}\-]+/gu, "")
      .replace(/-+/g, "-")
      .replace(/^-|-$/g, "");
    base = base || "section";
    var id = base;
    var n = 2;
    while (used[id]) {
      id = base + "-" + n;
      n += 1;
    }
    used[id] = true;
    return id;
  }

  function parseFrontmatter(raw) {
    var text = raw.replace(/^﻿/, "");
    var match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(text);
    if (!match) return { meta: {}, body: text };

    var meta = {};
    match[1].split(/\r?\n/).forEach(function (line) {
      var pair = /^([A-Za-z0-9_-]+)\s*:\s*(.*)$/.exec(line);
      if (!pair) return;
      var value = pair[2].trim().replace(/^["']|["']$/g, "");
      meta[pair[1]] = value;
    });

    return { meta: meta, body: text.slice(match[0].length) };
  }

  function escapeHtml(value) {
    return value
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy") ? resolve() : reject(new Error("copy failed"));
      } catch (error) {
        reject(error);
      } finally {
        document.body.removeChild(area);
      }
    });
  }

  /* ------------------------------------------------------------------------
     Navigation
     ------------------------------------------------------------------------ */

  function renderNav() {
    if (!els.nav) return;
    els.nav.textContent = "";

    (app.manifest.sections || []).forEach(function (section) {
      var wrap = document.createElement("section");
      wrap.className = "docs-nav__section";
      wrap.dataset.section = section.title || "";

      var title = document.createElement("p");
      title.className = "docs-nav__title mono mono--sm";
      title.textContent = section.title || "文档";
      wrap.appendChild(title);

      var list = document.createElement("ul");
      list.className = "docs-nav__list";

      (section.items || []).forEach(function (item) {
        var li = document.createElement("li");
        var link = document.createElement("a");
        link.className = "docs-nav__link";
        link.href = "#/" + item.slug;
        link.dataset.slug = item.slug;
        link.textContent = item.title;

        li.appendChild(link);
        list.appendChild(li);
      });

      wrap.appendChild(list);
      els.nav.appendChild(wrap);
    });

    setActiveNav(currentSlug());
  }

  function setActiveNav(slug) {
    if (!els.nav) return;
    var links = els.nav.querySelectorAll(".docs-nav__link");
    Array.prototype.forEach.call(links, function (link) {
      var active = link.dataset.slug === slug;
      link.classList.toggle("is-active", active);
      if (active) {
        link.setAttribute("aria-current", "page");
        link.scrollIntoView({ block: "nearest" });
      } else {
        link.removeAttribute("aria-current");
      }
    });
  }

  /* ------------------------------------------------------------------------
     Rendering
     ------------------------------------------------------------------------ */

  function buildHeader(item, meta) {
    var title = meta.title || item.title;
    var description = meta.description || item.description;
    var status = meta.status || item.status || "draft";

    var header = document.createElement("header");
    header.className = "doc-header";

    var kicker = document.createElement("p");
    kicker.className = "doc-kicker";
    kicker.textContent = item.section || "文档";
    header.appendChild(kicker);

    var h1 = document.createElement("h1");
    h1.className = "doc-title";
    h1.textContent = title;
    header.appendChild(h1);

    if (description) {
      var desc = document.createElement("p");
      desc.className = "doc-desc";
      desc.textContent = description;
      header.appendChild(desc);
    }

    var metaRow = document.createElement("div");
    metaRow.className = "doc-meta";
    if (meta.updated) {
      var updated = document.createElement("span");
      updated.textContent = "更新于 " + meta.updated;
      metaRow.appendChild(updated);
    }
    if (status === "draft") {
      var draft = document.createElement("span");
      draft.textContent = "MVP 草稿 · 以仓库为准";
      metaRow.appendChild(draft);
    }
    if (metaRow.children.length) header.appendChild(metaRow);
    return header;
  }

  function renderMarkdown(body) {
    if (window.marked && typeof window.marked.parse === "function") {
      var html = window.marked.parse(body, { gfm: true, breaks: false });
      if (window.DOMPurify) {
        return window.DOMPurify.sanitize(html, {
          USE_PROFILES: { html: true }
        });
      }
      return escapeHtml(body);
    }
    return "<pre>" + escapeHtml(body) + "</pre>";
  }

  var CALLOUTS = {
    NOTE: "说明",
    TIP: "提示",
    IMPORTANT: "重要",
    WARNING: "注意",
    CAUTION: "注意",
    OUTPUT: "应该看到",
    FAIL: "失败时"
  };

  var CALLOUT_KINDS = {
    OUTPUT: "output",
    FAIL: "fail",
    TIP: "tip",
    WARNING: "warn",
    CAUTION: "fail",
    IMPORTANT: "note",
    NOTE: "note"
  };

  function enhanceCallouts(content) {
    Array.prototype.forEach.call(
      content.querySelectorAll("blockquote"),
      function (quote) {
        var first = quote.querySelector("p");
        if (!first) return;

        var match = /^\[!(\w+)\]\s*(?:<br\s*\/?>)?/.exec(first.innerHTML);
        if (!match) return;

        var kind = match[1].toUpperCase();
        first.innerHTML = first.innerHTML.replace(match[0], "");
        if (!first.textContent.trim()) first.remove();

        quote.className = "callout callout--" + (CALLOUT_KINDS[kind] || "note");

        var label = document.createElement("span");
        label.className = "callout__label";
        label.textContent = CALLOUTS[kind] || kind;
        quote.insertBefore(label, quote.firstChild);
      }
    );
  }

  function enhanceContent(content, item) {
    enhanceCallouts(content);

    /* Heading ids + table of contents */
    var used = Object.create(null);
    var headings = content.querySelectorAll("h2, h3");

    Array.prototype.forEach.call(headings, function (heading) {
      heading.id = slugify(heading.textContent, used);
    });

    /* Tables scroll horizontally */
    Array.prototype.forEach.call(content.querySelectorAll("table"), function (table) {
      if (table.parentNode.classList.contains("table-wrap")) return;
      var wrap = document.createElement("div");
      wrap.className = "table-wrap";
      table.parentNode.insertBefore(wrap, table);
      wrap.appendChild(table);
    });

    /* Code blocks: language label + copy */
    Array.prototype.forEach.call(content.querySelectorAll("pre"), function (pre) {
      /* Expected-output blocks inside callouts are already framed; keep them plain. */
      if (pre.closest("blockquote")) return;

      var code = pre.querySelector("code");
      var lang = "";
      if (code) {
        var cls = code.className.match(/language-([\w-]+)/);
        if (cls) lang = cls[1];
      }

      var block = document.createElement("div");
      block.className = "code-block";

      var bar = document.createElement("div");
      bar.className = "code-block__bar";

      var label = document.createElement("span");
      label.className = "code-block__lang";
      label.textContent = lang || "text";

      var copy = document.createElement("button");
      copy.type = "button";
      copy.className = "code-block__copy";
      copy.textContent = "copy";

      copy.addEventListener("click", function () {
        copyText(code ? code.textContent : pre.textContent)
          .then(function () {
            copy.textContent = "copié";
            window.setTimeout(function () {
              copy.textContent = "copy";
            }, 1400);
          })
          .catch(function () {
            copy.textContent = "failed";
            window.setTimeout(function () {
              copy.textContent = "copy";
            }, 1400);
          });
      });

      bar.appendChild(label);
      bar.appendChild(copy);

      pre.parentNode.insertBefore(block, pre);
      block.appendChild(bar);
      block.appendChild(pre);
    });

    /* Internal links: .md files and in-page anchors */
    Array.prototype.forEach.call(content.querySelectorAll("a[href]"), function (link) {
      var href = link.getAttribute("href");

      if (/^https?:\/\//i.test(href)) {
        try {
          var url = new URL(href);
          if (url.origin !== window.location.origin) {
            link.target = "_blank";
            link.rel = "noopener";
          }
        } catch (error) {
          /* ignore malformed */
        }
        return;
      }

      var mdLink = /^(?:.*\/)?([^\/#]+)\.md(?:#[^#]*)?$/i.exec(href);
      if (mdLink) {
        link.classList.add("is-doc-link");
        link.setAttribute("href", "#/" + mdLink[1]);
        return;
      }

      if (href.charAt(0) === "#" && href.length > 1) {
        var target = href.slice(1);
        link.setAttribute("data-anchor", target);
        link.setAttribute("href", "#/" + item.slug);
      }
    });

    /* Anchor clicks inside the article scroll without changing the route. */
    content.addEventListener("click", function (event) {
      var link = event.target.closest("a[data-anchor]");
      if (!link || !content.contains(link)) return;
      var target = document.getElementById(link.getAttribute("data-anchor"));
      if (!target) return;
      event.preventDefault();
      target.scrollIntoView({
        behavior: reduce.matches ? "auto" : "smooth",
        block: "start"
      });
    });

    return headings;
  }

  function buildToc(headings, item) {
    if (!els.tocList || !els.toc) return;
    els.tocList.textContent = "";

    var tocHeadings = Array.prototype.filter.call(headings || [], function (heading) {
      return heading.tagName === "H2";
    });

    if (tocHeadings.length === 0) {
      els.toc.hidden = true;
      return;
    }

    els.toc.hidden = false;
    Array.prototype.forEach.call(tocHeadings, function (heading) {
      var li = document.createElement("li");
      li.className = "toc-h2";

      var link = document.createElement("a");
      link.href = "#/" + item.slug;
      link.setAttribute("data-anchor", heading.id);
      link.textContent = heading.textContent;

      link.addEventListener("click", function (event) {
        event.preventDefault();
        var target = document.getElementById(heading.id);
        if (!target) return;
        target.scrollIntoView({
          behavior: reduce.matches ? "auto" : "smooth",
          block: "start"
        });
      });

      li.appendChild(link);
      els.tocList.appendChild(li);
    });

    observeHeadings(tocHeadings);
  }

  function observeHeadings(headings) {
    if (app.tocObserver) app.tocObserver.disconnect();
    if (!("IntersectionObserver" in window) || !els.tocList) return;

    var links = els.tocList.querySelectorAll("a");
    var map = {};

    Array.prototype.forEach.call(links, function (link) {
      map[link.getAttribute("data-anchor")] = link;
    });

    app.tocObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          Array.prototype.forEach.call(links, function (link) {
            link.classList.remove("is-active");
          });
          var active = map[entry.target.id];
          if (active) active.classList.add("is-active");
        });
      },
      { rootMargin: "-18% 0px -68% 0px", threshold: 0 }
    );

    Array.prototype.forEach.call(headings, function (heading) {
      app.tocObserver.observe(heading);
    });
  }

  function renderPager(item) {
    if (!els.pager) return;
    els.pager.textContent = "";

    var index = app.flat.indexOf(item);
    var prev = index > 0 ? app.flat[index - 1] : null;
    var next = index < app.flat.length - 1 ? app.flat[index + 1] : null;

    if (!prev && !next) {
      els.pager.hidden = true;
      return;
    }

    function makeLink(target, direction) {
      var link = document.createElement("a");
      link.className =
        "doc-pager__link" + (direction === "next" ? " doc-pager__link--next" : "");
      link.href = "#/" + target.slug;

      var label = document.createElement("span");
      label.className = "doc-pager__label";
      label.textContent = direction === "next" ? "下一篇 →" : "← 上一篇";

      var title = document.createElement("span");
      title.className = "doc-pager__title";
      title.textContent = target.title;

      link.appendChild(label);
      link.appendChild(title);
      return link;
    }

    if (prev) els.pager.appendChild(makeLink(prev, "prev"));
    if (next) els.pager.appendChild(makeLink(next, "next"));
    els.pager.hidden = false;
  }

  function showError(item, error) {
    if (!els.view || !els.state) return;
    els.state.hidden = true;
    els.view.hidden = false;
    els.view.textContent = "";

    var box = document.createElement("div");
    box.className = "doc-error";

    var title = document.createElement("h2");
    title.textContent = "这篇文档没有载入";

    var reason = document.createElement("p");
    reason.textContent = "无法读取 " + (item ? item.file : "文档") + "。";

    var detail = document.createElement("p");
    detail.className = "mono mono--sm";
    detail.style.marginTop = "0.6rem";
    detail.textContent = String(error && error.message ? error.message : error);

    box.appendChild(title);
    box.appendChild(reason);
    box.appendChild(detail);

    if (window.location.protocol === "file:") {
      var hint = document.createElement("p");
      hint.textContent =
        "浏览器不允许 file:// 页面读取 Markdown。请在项目根目录运行：";
      var code = document.createElement("code");
      code.textContent = "python -m http.server 4173 --directory site";
      box.appendChild(hint);
      box.appendChild(code);
    } else {
      var hint = document.createElement("p");
      hint.textContent =
        "检查 manifest.json 中的 file 路径，或确认 content/*.md 已经放入。";
      box.appendChild(hint);
    }

    els.view.appendChild(box);
    if (els.pager) els.pager.hidden = true;
  }

  function renderDoc(item, meta, body) {
    if (!els.view || !els.state) return;

    var header = buildHeader(item, meta);
    var content = document.createElement("div");
    content.className = "doc-content";
    content.innerHTML = renderMarkdown(body);

    /* The article header already renders the title; drop the duplicate H1. */
    var firstH1 = content.querySelector("h1");
    if (firstH1) firstH1.remove();

    var headings = enhanceContent(content, item);

    els.view.textContent = "";
    els.view.appendChild(header);
    els.view.appendChild(content);
    els.view.hidden = false;
    els.state.hidden = true;

    document.title = (meta.title || item.title) + " — Levier 文档";
    buildToc(headings, item);
    renderPager(item);
    setActiveNav(item.slug);
  }

  function load(item) {
    if (!item || !els.state || !els.view) return;

    app.current = item;
    els.state.hidden = false;
    els.state.className = "doc-loading mono";
    els.state.textContent = "正在载入 " + item.title + " …";
    els.view.hidden = true;
    els.view.textContent = "";
    if (els.pager) els.pager.hidden = true;
    if (els.toc) els.toc.hidden = true;
    setActiveNav(item.slug);

    fetch(item.file, { cache: "no-store" })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("HTTP " + response.status + " · " + item.file);
        }
        return response.text();
      })
      .then(function (raw) {
        var parsed = parseFrontmatter(raw);
        renderDoc(item, parsed.meta, parsed.body);
      })
      .catch(function (error) {
        showError(item, error);
      });
  }

  function route() {
    var slug = currentSlug();

    if (!slug) {
      slug = app.flat.length ? app.flat[0].slug : "";
      if (slug) {
        history.replaceState(null, "", "#/" + slug);
      }
    }

    var item = find(slug);
    if (!item) {
      if (app.flat.length) {
        history.replaceState(null, "", "#/" + app.flat[0].slug);
        item = app.flat[0];
      } else {
        showError(null, new Error("manifest.json 为空"));
        return;
      }
    }

    load(item);
    window.scrollTo({ top: 0, behavior: "auto" });
    closeNav();
  }

  /* ------------------------------------------------------------------------
     Mobile drawer
     ------------------------------------------------------------------------ */

  function openNav() {
    if (!els.sidebar) return;
    els.sidebar.classList.add("is-open");
    if (els.backdrop) {
      els.backdrop.hidden = false;
      window.requestAnimationFrame(function () {
        els.backdrop.classList.add("is-open");
      });
    }
    document.body.classList.add("nav-open");
    if (els.menuBtn) els.menuBtn.setAttribute("aria-expanded", "true");
  }

  function closeNav() {
    if (!els.sidebar) return;
    els.sidebar.classList.remove("is-open");
    if (els.backdrop) {
      els.backdrop.classList.remove("is-open");
      window.setTimeout(function () {
        els.backdrop.hidden = true;
      }, 250);
    }
    document.body.classList.remove("nav-open");
    if (els.menuBtn) els.menuBtn.setAttribute("aria-expanded", "false");
  }

  /* ------------------------------------------------------------------------
     Boot
     ------------------------------------------------------------------------ */

  function init() {
    if (els.menuBtn) {
      els.menuBtn.addEventListener("click", function () {
        els.sidebar.classList.contains("is-open") ? closeNav() : openNav();
      });
    }

    if (els.backdrop) {
      els.backdrop.addEventListener("click", function () {
        closeNav();
        if (els.menuBtn) els.menuBtn.focus();
      });
    }

    if (els.closeBtn) {
      els.closeBtn.addEventListener("click", function () {
        closeNav();
        if (els.menuBtn) els.menuBtn.focus();
      });
    }

    if (els.nav) {
      els.nav.addEventListener("click", function (event) {
        if (event.target.closest("a")) closeNav();
      });
    }

    document.addEventListener("keydown", function (event) {
      if (event.key !== "Escape") return;
      var wasOpen = els.sidebar && els.sidebar.classList.contains("is-open");
      closeNav();
      if (wasOpen && els.menuBtn) els.menuBtn.focus();
    });

    window.addEventListener("resize", function () {
      if (window.innerWidth > 980) closeNav();
    });

    window.addEventListener("hashchange", route);

    fetch(MANIFEST_URL, { cache: "no-store" })
      .then(function (response) {
        if (!response.ok) throw new Error("HTTP " + response.status);
        return response.json();
      })
      .then(function (manifest) {
        app.manifest = manifest;
        app.flat = flatten(manifest);
        renderNav();
        route();
      })
      .catch(function (error) {
        if (els.nav) {
          els.nav.innerHTML =
            '<p class="mono mono--sm" style="color: var(--vermilion);">目录载入失败</p>';
        }
        showError(null, error);
      });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
