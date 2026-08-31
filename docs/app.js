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
})();
