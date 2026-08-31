(() => {
  const year = document.getElementById("year");
  if (year) year.textContent = String(new Date().getFullYear());

  document.querySelectorAll("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const target = document.querySelector(btn.getAttribute("data-copy"));
      if (!target) return;
      const text = target.innerText.replace(/\n+$/, "") + "\n";
      try {
        await navigator.clipboard.writeText(text);
        btn.dataset.copied = "true";
        btn.textContent = "Copied";
        window.setTimeout(() => {
          delete btn.dataset.copied;
          btn.textContent = "Copy";
        }, 1600);
      } catch {
        btn.textContent = "Copy failed";
      }
    });
  });

  const navLinks = [...document.querySelectorAll('.nav nav a[href^="#"]')];
  const sections = navLinks
    .map((link) => document.querySelector(link.getAttribute("href")))
    .filter(Boolean);
  let scrollTarget = null;
  let scrollEndHandler = null;
  let scrollUnlockTimer = null;
  let activeLink = null;

  const setActiveSection = (section) => {
    navLinks.forEach((link) => {
      const isActive = section && link.getAttribute("href") === `#${section.id}`;
      link.classList.toggle("active", isActive);
      if (isActive) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");

      if (isActive && activeLink !== link) {
        activeLink = link;
        const mobileNav = window.matchMedia("(max-width: 640px)").matches;
        const nav = link.closest("nav");
        const navIsScrollable = nav && nav.scrollWidth > nav.clientWidth;

        if (mobileNav && navIsScrollable) {
          link.scrollIntoView({
            behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth",
            block: "nearest",
            inline: "center",
          });
        }
      }
    });

    if (!section) activeLink = null;
  };

  const updateActiveSection = () => {
    if (scrollTarget) {
      setActiveSection(document.getElementById(scrollTarget));
      return;
    }

    const marker = window.scrollY + document.querySelector(".nav").offsetHeight + 96;
    let current = null;

    sections.forEach((section) => {
      if (section.offsetTop <= marker) current = section;
    });

    setActiveSection(current);
  };

  const releaseScrollTarget = () => {
    if (scrollEndHandler) {
      window.removeEventListener("scrollend", scrollEndHandler);
      scrollEndHandler = null;
    }
    window.clearTimeout(scrollUnlockTimer);
    scrollTarget = null;
    updateActiveSection();
  };

  navLinks.forEach((link) => {
    link.addEventListener("click", (event) => {
      const target = document.querySelector(link.getAttribute("href"));
      if (!target) return;

      event.preventDefault();
      releaseScrollTarget();
      scrollTarget = target.id;
      setActiveSection(target);

      const navHeight = document.querySelector(".nav").offsetHeight;
      const targetTop = target.getBoundingClientRect().top + window.scrollY - navHeight - 24;
      const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

      window.scrollTo({
        top: Math.max(0, targetTop),
        behavior: reducedMotion ? "auto" : "smooth",
      });

      if (!reducedMotion && "onscrollend" in window) {
        scrollEndHandler = releaseScrollTarget;
        window.addEventListener("scrollend", scrollEndHandler, { once: true });
      }
      scrollUnlockTimer = window.setTimeout(releaseScrollTarget, reducedMotion ? 0 : 1200);
      history.replaceState(null, "", link.getAttribute("href"));
    });
  });

  let ticking = false;
  window.addEventListener(
    "scroll",
    () => {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(() => {
        updateActiveSection();
        ticking = false;
      });
    },
    { passive: true }
  );

  updateActiveSection();
})();
