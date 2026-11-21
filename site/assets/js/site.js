/* Levier — small page behaviours. */
(function () {
  "use strict";

  /* Keep the masthead legible once the page has scrolled underneath. */
  var masthead = document.querySelector(".masthead");
  if (!masthead) return;

  var ticking = false;
  var update = function () {
    masthead.style.boxShadow =
      window.scrollY > 24 ? "0 1px 0 rgba(28,25,21,.1)" : "none";
    ticking = false;
  };

  window.addEventListener(
    "scroll",
    function () {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(update);
    },
    { passive: true }
  );

  update();
})();
