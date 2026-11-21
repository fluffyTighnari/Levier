/* Levier — integration trace player.
   Tabs switch between real commands; each screen reveals line by line once. */
(function () {
  "use strict";

  var tablist = document.querySelector(".trace__steps");
  if (!tablist) return;

  var tabs = Array.prototype.slice.call(
    tablist.querySelectorAll(".trace-step")
  );
  var screens = Array.prototype.slice.call(
    document.querySelectorAll(".trace-screen")
  );
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)");
  var played = Object.create(null);
  var timers = [];

  function clearTimers() {
    timers.forEach(window.clearTimeout);
    timers = [];
  }

  function removeCursor(screen) {
    var cursors = screen.querySelectorAll(".t-cursor");
    Array.prototype.forEach.call(cursors, function (cursor) {
      cursor.parentNode.removeChild(cursor);
    });
  }

  function addCursor(screen) {
    var pre = screen.querySelector(".terminal__pre");
    if (!pre) return;
    var cursor = document.createElement("span");
    cursor.className = "t-cursor";
    cursor.setAttribute("aria-hidden", "true");
    pre.appendChild(cursor);
  }

  function reveal(screen, animate) {
    var lines = Array.prototype.slice.call(
      screen.querySelectorAll(".t-line")
    );

    removeCursor(screen);
    lines.forEach(function (line) {
      line.classList.remove("is-revealed");
    });

    if (!animate || reduce.matches) {
      lines.forEach(function (line) {
        line.classList.add("is-revealed");
      });
      addCursor(screen);
      return;
    }

    lines.forEach(function (line, index) {
      timers.push(
        window.setTimeout(function () {
          line.classList.add("is-revealed");
          if (index === lines.length - 1) addCursor(screen);
        }, 90 + index * 105)
      );
    });
  }

  function select(id, animate) {
    var screen = null;

    tabs.forEach(function (tab) {
      var active = tab.dataset.trace === id;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", active ? "true" : "false");
      tab.tabIndex = active ? 0 : -1;
    });

    screens.forEach(function (candidate) {
      var active = candidate.dataset.screen === id;
      candidate.classList.toggle("is-active", active);
      if (active) screen = candidate;
    });

    if (!screen) return;

    clearTimers();
    var firstPlay = !played[id];
    played[id] = true;
    reveal(screen, animate !== false && firstPlay);
  }

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      select(tab.dataset.trace, true);
    });
  });

  tablist.addEventListener("keydown", function (event) {
    var keys = ["ArrowDown", "ArrowUp", "Home", "End"];
    if (keys.indexOf(event.key) === -1) return;
    event.preventDefault();

    var current = tabs.indexOf(document.activeElement);
    var next = current;

    if (event.key === "ArrowDown") next = (current + 1) % tabs.length;
    if (event.key === "ArrowUp") next = (current - 1 + tabs.length) % tabs.length;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = tabs.length - 1;

    tabs[next].focus();
    select(tabs[next].dataset.trace, true);
  });

  /* First screen is readable immediately; later tab switches replay once. */
  select("1", false);
})();
